package drivers

import (
	"fmt"

	"github.com/meteormin/go-vfs"
	driverBadger "github.com/meteormin/go-vfs/drivers/badger"
	driverLocalStorage "github.com/meteormin/go-vfs/drivers/localstorage"
)

type DriverType string

const (
	DriverTypeBadger       DriverType = "badger"
	DriverTypeLocalStorage DriverType = "localstorage"
)

type Config struct {
	Type         DriverType                `json:"type"`
	Badger       driverBadger.Config       `json:"badger"`
	LocalStorage driverLocalStorage.Config `json:"localstorage"`
}

func New(cfg Config) (vfs.VFS, error) {
	switch cfg.Type {
	case DriverTypeBadger:
		return driverBadger.New(cfg.Badger)
	case DriverTypeLocalStorage:
		return driverLocalStorage.New(cfg.LocalStorage)
	default:
		return nil, fmt.Errorf("unknown driver type: %s", cfg.Type)
	}
}
