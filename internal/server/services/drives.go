package services

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
)

type DriveManager struct {
	config badger.Config
	drives map[uuid.UUID]*badger.BadgerVFS
	mu     sync.Mutex
}

func NewDriveManager(config badger.Config) *DriveManager {
	return &DriveManager{config: config, drives: make(map[uuid.UUID]*badger.BadgerVFS)}
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

func (m *DriveManager) open(userID uuid.UUID) (*badger.BadgerVFS, error) {
	config := m.config
	config.Path = filepath.Join(m.config.Path, userID.String())
	config.EncryptKey = nil
	return badger.New(&config)
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
	stats, err := drive.Stats()
	if err != nil {
		return types.StorageStatRes{}, wasOpen, err
	}
	var total types.StorageStatRes
	for _, stat := range stats {
		total.Items += stat.Count
		total.Size += stat.Size
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
