package services

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers"
	driverbadger "github.com/shabatoily/govfs/pkg/drivers/badger"
)

// DriveManagerConfig는 사용자 드라이브의 생성과 수명주기를 설정합니다.
type DriveManagerConfig struct {
	Driver      drivers.Config
	IdleTimeout time.Duration
}

type driveEntry struct {
	drive    vfs.VFS
	lastUsed time.Time
}

type DriveManager struct {
	config      drivers.Config
	drives      map[uuid.UUID]driveEntry
	idleTimeout time.Duration
	stop        chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	mu          sync.Mutex
}

func NewDriveManager(config DriveManagerConfig) *DriveManager {
	m := &DriveManager{
		config:      config.Driver,
		drives:      make(map[uuid.UUID]driveEntry),
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
	if entry, ok := m.drives[userID]; ok {
		entry.lastUsed = time.Now()
		m.drives[userID] = entry
		return entry.drive, nil
	}
	drive, err := m.open(userID)
	if err != nil {
		return nil, err
	}
	m.drives[userID] = driveEntry{drive: drive, lastUsed: time.Now()}
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

// BadgerResources는 열려 있는 Badger 드라이브의 리소스 현황을 반환합니다.
func (m *DriveManager) BadgerResources() ([]types.BadgerResourceRes, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	resources := make([]types.BadgerResourceRes, 0, len(m.drives))
	for userID, entry := range m.drives {
		drive, ok := entry.drive.(*driverbadger.BadgerVFS)
		if !ok {
			continue
		}
		db := drive.DB()
		lsm, vlog := db.Size()
		blockCache, err := db.CacheMaxCost(badgerdb.BlockCache, -1)
		if err != nil {
			return nil, err
		}
		indexCache, err := db.CacheMaxCost(badgerdb.IndexCache, -1)
		if err != nil {
			return nil, err
		}
		resources = append(resources, types.BadgerResourceRes{
			UserID: userID, LSMSize: lsm, VlogSize: vlog,
			BlockCacheMaxCost: blockCache, IndexCacheMaxCost: indexCache,
		})
	}
	return resources, nil
}

func (m *DriveManager) Stats(userID uuid.UUID) (types.StorageStatRes, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, wasOpen := m.drives[userID]
	drive := entry.drive
	if !wasOpen {
		var err error
		drive, err = m.open(userID)
		if err != nil {
			return types.StorageStatRes{}, false, err
		}
		defer drive.Close()
	} else {
		entry.lastUsed = time.Now()
		m.drives[userID] = entry
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
	entry, ok := m.drives[userID]
	if !ok {
		return nil
	}
	delete(m.drives, userID)
	return entry.drive.Close()
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
	for id, entry := range m.drives {
		if now.Sub(entry.lastUsed) < m.idleTimeout {
			continue
		}
		if err := entry.drive.Close(); err != nil {
			log.Errorf("failed to close idle drive: %v", err)
		}
		delete(m.drives, id)
	}
}

func (m *DriveManager) Close() error {
	m.stopOnce.Do(func() { close(m.stop) })
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	for id, entry := range m.drives {
		err = errors.Join(err, entry.drive.Close())
		delete(m.drives, id)
	}
	return err
}
