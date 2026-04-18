// Package googledrive는 Google Drive API를 사용한 클라우드 저장소 연동 기능(어댑터)을 제공합니다.
package googledrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	tokenDirMode         = 0600
	tokenFileMode        = 0600
	defaultTokenFilename = "token.json"
)

// ErrUnauthorized는 인증 정보가 없거나 유효하지 않을 때 발생하는 에러입니다.
var ErrUnauthorized = errors.New("unauthorized")

// ClientConfig는 Google Drive API 연동에 필요한 클라이언트 설정 정보를 포함하는 구조체입니다.
type ClientConfig struct {
	Context      context.Context `json:"-"`
	TokenPath    string          `json:"tokenPath"`
	ParentFolder string          `json:"parentFolder"`
	ClientID     string          `json:"-"`
	ClientSecret string          `json:"-"`
	OAuth2Config oauth2.Config   `json:"-"`
}

// Adapter는 Google Drive와 연동하여 데이터를 읽고 쓰는 클라우드 스토리지 구현체입니다.
type Adapter struct {
	cfg     *ClientConfig
	service *drive.Service
}

// New는 새로운 Google Drive 스토리지 어댑터를 생성합니다.
// ParentFolder가 제공되면 드라이브 루트에서 해당 폴더를 찾거나 없으면 생성하며,
// 이후 모든 작업은 해당 폴더 범위(스코프) 내에서만 수행됩니다.
func New(cfg *ClientConfig) (*Adapter, error) {
	d := &Adapter{cfg: cfg}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("need to set env GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
	}

	cfg.OAuth2Config = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       []string{drive.DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  google.Endpoint.AuthURL,
			TokenURL: google.Endpoint.TokenURL,
		},
	}

	if cfg.TokenPath == "" {
		tokenPath := ".googledrive"
		if _, err := os.Stat(tokenPath); errors.Is(err, os.ErrNotExist) {
			err = os.Mkdir(tokenPath, tokenDirMode)
			if err != nil {
				return nil, err
			}
		}
		cfg.TokenPath = tokenPath
	}

	t, err := d.getTokenFromFile()
	if err != nil {
		return d, ErrUnauthorized
	}

	if err := d.Init(t); err != nil {
		return nil, err
	}

	return d, nil
}

// Init은 지정된 OAuth2 토큰을 사용하여 내부 Google Drive 서비스 클라이언트를 초기화합니다.
func (d *Adapter) Init(token *oauth2.Token) error {
	client := option.WithHTTPClient(d.cfg.OAuth2Config.Client(d.cfg.Context, token))
	service, err := drive.NewService(d.cfg.Context, client)
	if err != nil {
		return err
	}

	d.service = service
	if d.cfg.ParentFolder != "" {
		folderID, err := d.findOrCreateFolder(d.cfg.ParentFolder)
		if err != nil {
			return fmt.Errorf("could not find or create parent folder '%s': %w", d.cfg.ParentFolder, err)
		}
		d.cfg.ParentFolder = folderID
	} else {
		d.cfg.ParentFolder = "root"
	}

	return d.saveToken(token)
}

// AuthCodeURL은 사용자 인증을 수행할 수 있는 초기 구글 로그인 페이지 URL을 생성합니다.
func (d *Adapter) AuthCodeURL(redirectURL, state string) string {
	oauth2Config := d.cfg.OAuth2Config
	oauth2Config.RedirectURL = redirectURL
	return oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// IssueToken은 권한 요청을 통해 받은 코드를 이용해 Google OAuth2 토큰을 발급받습니다.
func (d *Adapter) IssueToken(code string) (*oauth2.Token, error) {
	oauth2Config := d.cfg.OAuth2Config
	token, err := oauth2Config.Exchange(context.TODO(), code)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// findOrCreateFolder finds a folder by name in the root of the drive, or creates it if it doesn't exist.
// It returns the ID of the folder.
func (d *Adapter) findOrCreateFolder(name string) (string, error) {
	query := fmt.Sprintf("name = '%s' and 'root' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
		escape(name))
	resp, err := d.service.Files.List().Q(query).Fields("files(id)").PageSize(1).Do()
	if err != nil {
		return "", err
	}

	if len(resp.Files) > 0 {
		return resp.Files[0].Id, nil
	}

	folder := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{"root"},
	}
	createdFolder, err := d.service.Files.Create(folder).Fields("id").Do()
	if err != nil {
		return "", err
	}
	return createdFolder.Id, nil
}

// findOrCreatePath ensures the folder structure for a given path exists, creating it if necessary.
// It returns the ID of the immediate parent folder for the given path.
func (d *Adapter) findOrCreatePath(filePath string) (string, error) {
	dir := path.Dir(filePath)
	if dir == "." || dir == "" {
		return d.cfg.ParentFolder, nil
	}

	parts := strings.Split(dir, "/")
	currentParentID := d.cfg.ParentFolder

	for _, part := range parts {
		if part == "" {
			continue
		}
		q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
			escape(part), currentParentID)
		resp, err := d.service.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
		if err != nil {
			return "", fmt.Errorf("failed to search for folder '%s': %w", part, err)
		}

		if len(resp.Files) > 0 {
			currentParentID = resp.Files[0].Id
		} else {
			folder := &drive.File{
				Name:     part,
				MimeType: "application/vnd.google-apps.folder",
				Parents:  []string{currentParentID},
			}
			createdFolder, err := d.service.Files.Create(folder).Fields("id").Do()
			if err != nil {
				return "", fmt.Errorf("failed to create folder '%s': %w", part, err)
			}
			currentParentID = createdFolder.Id
		}
	}
	return currentParentID, nil
}

// Upload는 지정된 경로에 파일을 업로드(또는 갱신)합니다. 필요한 부모 디렉토리는 자동으로 생성합니다.
func (d *Adapter) Upload(p string, r io.Reader) error {
	parentID, err := d.findOrCreatePath(p)
	if err != nil {
		return fmt.Errorf("could not establish path for '%s': %w", p, err)
	}

	fileName := path.Base(p)

	// Check if file exists to update it, otherwise create a new one.
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false and mimeType != 'application/vnd.google-apps.folder'",
		escape(fileName), parentID)
	resp, err := d.service.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
	if err != nil {
		return err
	}

	file := &drive.File{Name: fileName}
	if len(resp.Files) > 0 {
		_, err = d.service.Files.Update(resp.Files[0].Id, file).Media(r).Do()
	} else {
		file.Parents = []string{parentID}
		_, err = d.service.Files.Create(file).Media(r).Do()
	}
	return err
}

// Download는 지정된 경로의 파일을 다운로드할 수 있는 데이터 스트림(ReadCloser)을 반환합니다.
func (d *Adapter) Download(p string) (io.ReadCloser, error) {
	fileID, err := d.findFileIDByPath(p)
	if err != nil {
		return nil, err
	}

	resp, err := d.service.Files.Get(fileID).Download()
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Delete는 지정된 경로의 파일이나 폴더를 제거합니다. 파일이 없으면 정상 처리로 간주합니다.
func (d *Adapter) Delete(p string) error {
	fileID, err := d.findFileIDByPath(p)
	if err != nil {
		// If the file doesn't exist, it's not an error for a delete operation.
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	return d.service.Files.Delete(fileID).Do()
}

// List는 주어진 접두사(prefix) 하위에 위치한 모든 파일들의 경로를 재귀적으로 탐색하여 목록을 반환합니다.
func (d *Adapter) List(prefix string) ([]string, error) {
	startFolderID := d.cfg.ParentFolder
	if prefix != "" {
		folderID, err := d.findFolderIDByPath(prefix)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return []string{}, nil
			}
			return nil, err
		}
		startFolderID = folderID
	}

	var allFiles []string
	err := d.recursiveList(startFolderID, prefix, &allFiles)
	if err != nil {
		return nil, err
	}
	return allFiles, nil
}

func (d *Adapter) recursiveList(folderID, currentPath string, allFiles *[]string) error {
	var pageToken string
	var pageSize int64 = 1000

	q := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	for {
		resp, err := d.service.Files.List().Q(q).Fields("nextPageToken, files(id, name, mimeType)").PageSize(pageSize).PageToken(pageToken).Do()
		if err != nil {
			return err
		}

		for _, f := range resp.Files {
			newPath := path.Join(currentPath, f.Name)
			if f.MimeType == "application/vnd.google-apps.folder" {
				if err := d.recursiveList(f.Id, newPath, allFiles); err != nil {
					return err
				}
			} else {
				*allFiles = append(*allFiles, newPath)
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return nil
}

func (d *Adapter) findFolderIDByPath(p string) (string, error) {
	p = strings.Trim(p, "/")
	if p == "" {
		return d.cfg.ParentFolder, nil
	}

	parts := strings.Split(p, "/")
	currentParentID := d.cfg.ParentFolder

	for _, part := range parts {
		q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
			escape(part), currentParentID)
		resp, err := d.service.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
		if err != nil {
			return "", err
		}
		if len(resp.Files) == 0 {
			return "", fmt.Errorf("folder not found: %s", p)
		}
		currentParentID = resp.Files[0].Id
	}
	return currentParentID, nil
}

func (d *Adapter) findFileIDByPath(p string) (string, error) {
	dir, name := path.Split(p)
	parentFolderID := d.cfg.ParentFolder
	if dir != "" {
		var err error
		parentFolderID, err = d.findFolderIDByPath(dir)
		if err != nil {
			return "", err
		}
	}

	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType != 'application/vnd.google-apps.folder' and trashed = false",
		escape(name), parentFolderID)
	resp, err := d.service.Files.List().Q(q).Fields("files(id)").PageSize(1).Do()
	if err != nil {
		return "", err
	}
	if len(resp.Files) == 0 {
		return "", fmt.Errorf("file not found: %s", p)
	}
	return resp.Files[0].Id, nil
}

func escape(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}

// GetClient는 설정과 토큰을 사용하여 인가된 HTTP 클라이언트를 생성합니다.
func GetClient(token *oauth2.Token, cfg *oauth2.Config) (*http.Client, error) {
	return cfg.Client(context.Background(), token), nil
}

func (d *Adapter) getTokenFromFile() (*oauth2.Token, error) {
	if _, err := os.Stat(d.cfg.TokenPath); os.IsNotExist(err) {
		return nil, err
	}

	f, err := os.Open(d.cfg.TokenPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func (d *Adapter) saveToken(token *oauth2.Token) error {
	tokenFileName := filepath.Join(d.cfg.TokenPath, defaultTokenFilename)
	f, err := os.OpenFile(tokenFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, tokenFileMode)
	if err != nil {
		return err
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(token)
	if err != nil {
		return err
	}
	return nil
}
