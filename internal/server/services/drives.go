package services

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers"
)

// DriveManagerConfig는 사용자 드라이브의 생성과 수명주기를 설정합니다.
type DriveManagerConfig struct {
	Driver      drivers.Config
	IdleTimeout time.Duration
}

type DriveManager struct {
	config      drivers.Config
	drives      map[uuid.UUID]vfs.VFS
	lastUsed    map[uuid.UUID]time.Time
	idleTimeout time.Duration
	stop        chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	mu          sync.Mutex
}

func NewDriveManager(config DriveManagerConfig) *DriveManager {
	m := &DriveManager{
		config:      config.Driver,
		drives:      make(map[uuid.UUID]vfs.VFS),
		lastUsed:    make(map[uuid.UUID]time.Time),
		idleTimeout: config.IdleTimeout,
		stop:        make(chan struct{}),
	}
	if config.IdleTimeout > 0 {
		m.wg.Add(1)
		go m.gc()
	}
	return m
}

func (m *DriveManager) Drive(userID uuid.UUID) (vfs.VFS, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if drive := m.drives[userID]; drive != nil {
		m.lastUsed[userID] = time.Now()
		return drive, nil
	}
	drive, err := m.open(userID)
	if err != nil {
		return nil, err
	}
	m.drives[userID] = drive
	m.lastUsed[userID] = time.Now()
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
	} else {
		m.lastUsed[userID] = time.Now()
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

// CloseDrive는 지정한 사용자의 드라이브가 열려 있으면 닫습니다.
func (m *DriveManager) CloseDrive(userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	drive := m.drives[userID]
	if drive == nil {
		return nil
	}
	delete(m.drives, userID)
	delete(m.lastUsed, userID)
	return drive.Close()
}

func (m *DriveManager) gc() {
	defer m.wg.Done()
	interval := min(m.idleTimeout/2, time.Minute)
	if interval <= 0 {
		interval = m.idleTimeout
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			m.closeIdle(now)
		case <-m.stop:
			return
		}
	}
}

func (m *DriveManager) closeIdle(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, lastUsed := range m.lastUsed {
		if now.Sub(lastUsed) < m.idleTimeout {
			continue
		}
		if err := m.drives[id].Close(); err != nil {
			log.Errorf("failed to close idle drive: %v", err)
		}
		delete(m.drives, id)
		delete(m.lastUsed, id)
	}
}

func (m *DriveManager) Close() error {
	m.stopOnce.Do(func() { close(m.stop) })
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	for id, drive := range m.drives {
		err = errors.Join(err, drive.Close())
		delete(m.drives, id)
		delete(m.lastUsed, id)
	}
	return err
}
