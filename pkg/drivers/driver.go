// Package drivers는 VFS의 저장소 드라이버 인터페이스 및 관리 기능을 제공합니다.
package drivers

import (
	"fmt"

	vfs "github.com/shabatoily/govfs"
	driverBadger "github.com/shabatoily/govfs/pkg/drivers/badger"
	driverLocalStorage "github.com/shabatoily/govfs/pkg/drivers/localstorage"
)

// DriverType은 지원되는 드라이버 유형을 정의합니다.
type DriverType string

const (
	// DriverTypeBadger는 BadgerDB 기반 드라이버입니다.
	DriverTypeBadger DriverType = "badger"
	// DriverTypeLocalStorage는 로컬 파일 시스템 기반 드라이버입니다.
	DriverTypeLocalStorage DriverType = "localstorage"
)

// Config는 드라이버 초기화를 위한 설정 구조체입니다.
type Config struct {
	Type         DriverType                `json:"type"`
	Badger       driverBadger.Config       `json:"badger"`
	LocalStorage driverLocalStorage.Config `json:"localstorage"`
}

// New는 설정된 유형에 따라 적절한 VFS 드라이버 인스턴스를 생성하여 반환합니다.
func New(cfg *Config) (vfs.VFS, error) {
	switch cfg.Type {
	case DriverTypeBadger:
		return driverBadger.New(&cfg.Badger)
	case DriverTypeLocalStorage:
		return driverLocalStorage.New(&cfg.LocalStorage)
	default:
		return nil, fmt.Errorf("unknown driver type: %s", cfg.Type)
	}
}
