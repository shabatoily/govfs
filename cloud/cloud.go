// Package cloud는 클라우드 저장소 연동을 위한 인터페이스 및 설정 구조를 제공합니다.
package cloud

import (
	"fmt"
	"io"

	"github.com/meteormin/govfs/cloud/googledrive"
)

// Storage는 클라우드 스토리지와 통신하기 위한 공통 인터페이스입니다.
type Storage interface {
	Upload(path string, r io.Reader) error
	Download(path string) (io.ReadCloser, error)
	Delete(path string) error
	List(path string) ([]string, error)
}

// Config는 클라우드 서비스 연동을 위한 설정 정보를 담고 있는 구조체입니다.
type Config struct {
	ClientType  string                   `json:"clientType"`
	GoogleDrive googledrive.ClientConfig `json:"googleDrive"`
}

// New는 설정된 클라이언트 타입에 맞는 클라우드 스토리지 어댑터를 생성하여 반환합니다.
func New(cfg *Config) (Storage, error) {
	switch cfg.ClientType {
	case "googleDrive":
		return googledrive.New(&cfg.GoogleDrive)
	default:
		return nil, fmt.Errorf("unknown client type: %s", cfg.ClientType)
	}
}
