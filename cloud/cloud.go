package cloud

import (
	"fmt"
	"io"

	"github.com/meteormin/govfs/cloud/googledrive"
)

type Storage interface {
	Upload(path string, r io.Reader) error
	Download(path string) (io.ReadCloser, error)
	Delete(path string) error
	List(path string) ([]string, error)
}

type Config struct {
	ClientType  string                   `json:"clientType"`
	GoogleDrive googledrive.ClientConfig `json:"googleDrive"`
}

func New(cfg *Config) (Storage, error) {
	switch cfg.ClientType {
	case "googleDrive":
		return googledrive.New(&cfg.GoogleDrive)
	default:
		return nil, fmt.Errorf("unknown client type: %s", cfg.ClientType)
	}
}
