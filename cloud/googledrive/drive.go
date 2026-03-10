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
	vfs "github.com/meteormin/govfs"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const tokenFileMode = 0o600

var ErrUnauthorized = errors.New("unauthorized")

type ClientConfig struct {
	Context      context.Context `json:"-"`
	TokenPath    string          `json:"tokenPath"`
	ParentFolder string          `json:"parentFolder"`
	ClientID     string          `json:"-"`
	ClientSecret string          `json:"-"`
	OAuth2Config oauth2.Config   `json:"-"`
}

type Adapter struct {
	cfg     *ClientConfig
	service *drive.Service
}

// New creates a new Google Drive Storage adapter.
// If a parentFolderName is provided, it will find that folder in the root of the drive or create it if it doesn't exist.
// All operations will then be scoped to that folder. If the name is empty, it will use the root of the drive.
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
		dir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		tokenPath := filepath.Join(dir, ".govfs")
		if _, err = os.Stat(tokenPath); errors.Is(err, os.ErrNotExist) {
			err = os.Mkdir(tokenPath, vfs.DefaultDirMode)
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

func (d *Adapter) AuthCodeURL(redirectURL string, state string) string {
	oauth2Config := d.cfg.OAuth2Config
	oauth2Config.RedirectURL = redirectURL
	return oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

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

// List returns a recursive listing of all files under the given prefix.
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
	f, err := os.OpenFile(d.cfg.TokenPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, tokenFileMode)
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
