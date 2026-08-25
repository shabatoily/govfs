package services

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers"
)

type DriveManager struct {
	config drivers.Config
	drives map[uuid.UUID]vfs.VFS
	mu     sync.Mutex
}

func NewDriveManager(config drivers.Config) *DriveManager {
	return &DriveManager{config: config, drives: make(map[uuid.UUID]vfs.VFS)}
}

func (m *DriveManager) Drive(userID uuid.UUID) (vfs.VFS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if drive := m.drives[userID]; drive != nil {
		return drive, nil
	}
	drive, err := m.open(userID)
	if err != nil {
		return nil, err
	}
	m.drives[userID] = drive
	return drive, nil
}

func (m *DriveManager) open(userID uuid.UUID) (vfs.VFS, error) {
	config := m.config
	switch config.Type {
	case drivers.DriverTypeBadger:
		config.Badger.Path = filepath.Join(config.Badger.Path, userID.String())
		config.Badger.EncryptKey = nil
	case drivers.DriverTypeLocalStorage:
		config.LocalStorage.Path = filepath.Join(config.LocalStorage.Path, userID.String())
	}
	return drivers.New(&config)
}

func (m *DriveManager) OpenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.drives)
}

func (m *DriveManager) Stats(userID uuid.UUID) (types.StorageStatRes, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	drive, wasOpen := m.drives[userID]
	if !wasOpen {
		var err error
		drive, err = m.open(userID)
		if err != nil {
			return types.StorageStatRes{}, false, err
		}
		defer drive.Close()
	}
	tree, err := drive.Tree(vfs.Root)
	if err != nil {
		return types.StorageStatRes{}, wasOpen, err
	}
	var total types.StorageStatRes
	for node := range tree.Walk() {
		if node.Meta.Path == vfs.Root {
			continue
		}
		total.Items++
		total.Size += node.Meta.Size
	}
	return total, wasOpen, nil
}

func (m *DriveManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	for id, drive := range m.drives {
		err = errors.Join(err, drive.Close())
		delete(m.drives, id)
	}
	return err
}
