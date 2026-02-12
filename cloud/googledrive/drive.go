package googledrive

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
)

type DriveStorage struct {
	service        *drive.Service
	parentFolderID string
}

// New creates a new Google Drive Storage adapter.
// If a parentFolderName is provided, it will find that folder in the root of the drive or create it if it doesn't exist.
// All operations will then be scoped to that folder. If the name is empty, it will use the root of the drive.
func New(service *drive.Service, parentFolderName string) (*DriveStorage, error) {
	d := &DriveStorage{
		service: service,
	}

	if parentFolderName != "" {
		folderID, err := d.findOrCreateFolder(parentFolderName)
		if err != nil {
			return nil, fmt.Errorf("could not find or create parent folder '%s': %w", parentFolderName, err)
		}
		d.parentFolderID = folderID
	} else {
		d.parentFolderID = "root"
	}

	return d, nil
}

// findOrCreateFolder finds a folder by name in the root of the drive, or creates it if it doesn't exist.
// It returns the ID of the folder.
func (d *DriveStorage) findOrCreateFolder(name string) (string, error) {
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
func (d *DriveStorage) findOrCreatePath(filePath string) (string, error) {
	dir := path.Dir(filePath)
	if dir == "." || dir == "" {
		return d.parentFolderID, nil
	}

	parts := strings.Split(dir, "/")
	currentParentID := d.parentFolderID

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

func (d *DriveStorage) Upload(p string, r io.Reader) error {
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

func (d *DriveStorage) Download(p string) (io.ReadCloser, error) {
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

func (d *DriveStorage) Delete(p string) error {
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
func (d *DriveStorage) List(prefix string) ([]string, error) {
	startFolderID := d.parentFolderID
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

func (d *DriveStorage) recursiveList(folderID, currentPath string, allFiles *[]string) error {
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

func (d *DriveStorage) findFolderIDByPath(p string) (string, error) {
	p = strings.Trim(p, "/")
	if p == "" {
		return d.parentFolderID, nil
	}

	parts := strings.Split(p, "/")
	currentParentID := d.parentFolderID

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

func (d *DriveStorage) findFileIDByPath(p string) (string, error) {
	dir, name := path.Split(p)
	parentFolderID := d.parentFolderID
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

func GetClient(tokenPath string, cfg *oauth2.Config) (*http.Client, error) {
	tokFile := filepath.Join(tokenPath, "token.json")
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(cfg)
		err = saveToken(tokFile, tok)
		if err != nil {
			return nil, err
		}
	}
	return cfg.Client(context.Background(), tok), nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

func saveToken(p string, token *oauth2.Token) error {
	fmt.Printf("Saving credential file to: %s\n", p)
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
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
