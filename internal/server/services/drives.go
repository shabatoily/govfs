package services

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
	"uuid"

	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/gofiber/fiber/v3/log"
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
	drive          vfs.VFS
	lastUsed       time.Time
	users          int
	closeRequested bool
}

type DriveManager struct {
	config      drivers.Config
	drives      map[uuid.UUID]driveEntry
	idleTimeout time.Duration
	stop        chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	mu          sync.Mutex
	closed      bool
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
	return m.driveLocked(userID)
}

func (m *DriveManager) driveLocked(userID uuid.UUID) (vfs.VFS, error) {
	if m.closed {
		return nil, os.ErrClosed
	}
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

// Acquire는 사용 중인 드라이브가 유휴 정리나 로그아웃으로 닫히지 않도록 유지합니다.
// 반환된 release는 요청, 비동기 작업 또는 스트림이 끝날 때 호출해야 합니다.
func (m *DriveManager) Acquire(userID uuid.UUID) (vfs.VFS, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	drive, err := m.driveLocked(userID)
	if err != nil {
		return nil, nil, err
	}
	entry := m.drives[userID]
	entry.users++
	m.drives[userID] = entry
	var once sync.Once
	return drive, func() { once.Do(func() { m.release(userID) }) }, nil
}

func (m *DriveManager) release(userID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.drives[userID]
	if !ok {
		return
	}
	entry.users--
	entry.lastUsed = time.Now()
	m.drives[userID] = entry
	if entry.users == 0 && entry.closeRequested {
		delete(m.drives, userID)
		if err := entry.drive.Close(); err != nil {
			log.Errorf("failed to close released drive: %v", err)
		}
	}
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
	if m.closed {
		return types.StorageStatRes{}, false, os.ErrClosed
	}
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

// CloseDrive는 사용자 드라이브를 닫고, 사용 중이면 마지막 release까지 종료를 미룹니다.
func (m *DriveManager) CloseDrive(userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.drives[userID]
	if !ok {
		return nil
	}
	if entry.users > 0 {
		entry.closeRequested = true
		m.drives[userID] = entry
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
		if entry.users > 0 || now.Sub(entry.lastUsed) < m.idleTimeout {
			continue
		}
		if err := entry.drive.Close(); err != nil {
			log.Errorf("failed to close idle drive: %v", err)
		}
		delete(m.drives, id)
	}
}

func (m *DriveManager) Close() error {
	// 정리 작업 전에 신규 드라이브 개방을 차단합니다.
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
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
