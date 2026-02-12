package cloud

import (
	"context"
	"io"
	"net/http"

	"github.com/meteormin/govfs/cloud/googledrive"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type Storage interface {
	Upload(path string, r io.Reader) error
	Download(path string) (io.ReadCloser, error)
	Delete(path string) error
	List(path string) ([]string, error)
}

func NewGoogleDriveStorage(ctx context.Context, client *http.Client, parentFolderID string) (*googledrive.DriveStorage, error) {
	svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	return googledrive.New(svc, parentFolderID)
}
