package services

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
	"github.com/shabatoily/govfs/pkg/drivers/localstorage"
)

func TestDriveManagerSeparatesUsers(t *testing.T) {
	for _, driverType := range []drivers.DriverType{drivers.DriverTypeBadger, drivers.DriverTypeLocalStorage} {
		t.Run(string(driverType), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "drives")
			manager := NewDriveManager(DriveManagerConfig{
				Driver: drivers.Config{
					Type:         driverType,
					Badger:       badger.Config{Path: root},
					LocalStorage: localstorage.Config{Path: root},
				},
				IdleTimeout: time.Hour,
			})
			t.Cleanup(func() { _ = manager.Close() })
			firstID := uuid.NewV4()
			first, err := manager.Drive(firstID)
			if err != nil {
				t.Fatal(err)
			}
			second, err := manager.Drive(uuid.NewV4())
			if err != nil {
				t.Fatal(err)
			}
			if first == second || manager.OpenCount() != 2 {
				t.Fatal("사용자 드라이브가 분리되지 않았습니다")
			}
			if same, err := manager.Drive(firstID); err != nil || same != first {
				t.Fatalf("동일 사용자 드라이브 재사용 실패: %v", err)
			}
			if _, err := first.Create("private.txt", bytes.NewBufferString("secret")); err != nil {
				t.Fatal(err)
			}
			files, err := second.List("/")
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 {
				t.Fatal("다른 사용자의 파일이 노출되었습니다")
			}
			stats, open, err := manager.Stats(firstID)
			if err != nil || !open || stats.Items != 1 || stats.Size != 6 {
				t.Fatalf("드라이브 통계 = %#v, open=%v, err=%v", stats, open, err)
			}
			stats, open, err = manager.Stats(uuid.NewV4())
			if err != nil || open || stats.Items != 0 || manager.OpenCount() != 2 {
				t.Fatalf("미개방 드라이브 통계 = %#v, open=%v, count=%d, err=%v", stats, open, manager.OpenCount(), err)
			}
		})
	}
}

func TestDriveManagerClosesIdleDrive(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{
		Driver: drivers.Config{
			Type:         drivers.DriverTypeLocalStorage,
			LocalStorage: localstorage.Config{Path: filepath.Join(t.TempDir(), "drives")},
		},
		IdleTimeout: 10 * time.Millisecond,
	})
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Drive(uuid.NewV4()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.OpenCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.OpenCount() != 0 {
		t.Fatal("유휴 드라이브가 닫히지 않았습니다")
	}
}

func TestDriveManagerBadgerResources(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{
		Driver: drivers.Config{
			Type:   drivers.DriverTypeBadger,
			Badger: badger.Config{Path: filepath.Join(t.TempDir(), "drives")},
		},
	})
	t.Cleanup(func() { _ = manager.Close() })
	userID := uuid.NewV4()
	if _, err := manager.Drive(userID); err != nil {
		t.Fatal(err)
	}

	resources, err := manager.BadgerResources()
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].UserID != userID || resources[0].BlockCacheMaxCost <= 0 {
		t.Fatalf("Badger 리소스 현황 = %#v", resources)
	}
}

func TestDriveManagerDoesNotReopenAfterClose(t *testing.T) {
	for _, driverType := range []drivers.DriverType{drivers.DriverTypeBadger, drivers.DriverTypeLocalStorage} {
		t.Run(string(driverType), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "drives")
			manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
				Type:         driverType,
				Badger:       badger.Config{Path: root},
				LocalStorage: localstorage.Config{Path: root},
			}})
			t.Cleanup(func() { _ = manager.Close() })
			userID := uuid.NewV4()
			if _, err := manager.Drive(userID); err != nil {
				t.Fatal(err)
			}
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uuid.UUID{userID, uuid.NewV4()} {
				if drive, err := manager.Drive(id); drive != nil || !errors.Is(err, os.ErrClosed) {
					t.Fatalf("종료 후 드라이브 개방: drive=%v, err=%v", drive, err)
				}
				if _, open, err := manager.Stats(id); open || !errors.Is(err, os.ErrClosed) {
					t.Fatalf("종료 후 드라이브 통계: open=%v, err=%v", open, err)
				}
			}
			if manager.OpenCount() != 0 {
				t.Fatal("종료 후 드라이브가 다시 열렸습니다")
			}
		})
	}
}

func TestDriveManagerConcurrentCloseAndOpen(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(t.TempDir(), "drives")},
	}, IdleTimeout: time.Millisecond})
	t.Cleanup(func() { _ = manager.Close() })
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := manager.Drive(uuid.NewV4()); err != nil && !errors.Is(err, os.ErrClosed) {
				t.Errorf("종료와 동시 개방: %v", err)
			}
		}()
	}
	close(start)
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if manager.OpenCount() != 0 {
		t.Fatal("종료와 경합한 드라이브가 남아 있습니다")
	}
}

func TestDriveManagerKeepsAcquiredDriveOpen(t *testing.T) {
	for _, driverType := range []drivers.DriverType{drivers.DriverTypeBadger, drivers.DriverTypeLocalStorage} {
		t.Run(string(driverType), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "drives")
			manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
				Type: driverType, Badger: badger.Config{Path: root}, LocalStorage: localstorage.Config{Path: root},
			}, IdleTimeout: time.Hour})
			t.Cleanup(func() { _ = manager.Close() })
			userID := uuid.NewV4()
			drive, release, err := manager.Acquire(userID)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)
			_, secondRelease, err := manager.Acquire(userID)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(secondRelease)
			manager.closeIdle(time.Now().Add(2 * time.Hour))
			if manager.OpenCount() != 1 {
				t.Fatal("사용 중인 드라이브가 유휴 정리로 닫혔습니다")
			}
			if err := manager.CloseDrive(userID); err != nil {
				t.Fatal(err)
			}
			release()
			release()
			if manager.OpenCount() != 1 {
				t.Fatal("다른 사용이 끝나기 전에 드라이브가 닫혔습니다")
			}
			if _, err := drive.List("/"); err != nil {
				t.Fatal(err)
			}
			secondRelease()
			if manager.OpenCount() != 0 {
				t.Fatal("마지막 사용 종료 후 드라이브가 닫히지 않았습니다")
			}
		})
	}
}

func TestDriveManagerIdleTimeoutStartsAfterRelease(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(t.TempDir(), "drives")},
	}, IdleTimeout: time.Hour})
	t.Cleanup(func() { _ = manager.Close() })
	_, release, err := manager.Acquire(uuid.NewV4())
	if err != nil {
		t.Fatal(err)
	}
	release()
	manager.closeIdle(time.Now())
	if manager.OpenCount() != 1 {
		t.Fatal("사용 종료 직후 드라이브가 닫혔습니다")
	}
	manager.closeIdle(time.Now().Add(2 * time.Hour))
	if manager.OpenCount() != 0 {
		t.Fatal("유휴 시간이 지난 드라이브가 닫히지 않았습니다")
	}
}

func TestDriveManagerShutdownWaitsForRelease(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(t.TempDir(), "drives")},
	}})
	t.Cleanup(func() { _ = manager.Close() })
	drive, release, err := manager.Acquire(uuid.NewV4())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	closed := make(chan error, 1)
	go func() { closed <- manager.CloseWithContext(t.Context()) }()
	select {
	case <-manager.stop:
	case <-time.After(time.Second):
		t.Fatal("종료 시작 대기 시간 초과")
	}
	select {
	case err := <-closed:
		t.Fatalf("사용 종료 전 저장소가 닫혔습니다: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := drive.List("/"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Acquire(uuid.NewV4()); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("종료 중 신규 개방: %v", err)
	}
	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("사용 종료 후 저장소 정리 대기 시간 초과")
	}
}

func TestDriveManagerShutdownDeadline(t *testing.T) {
	manager := NewDriveManager(DriveManagerConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(t.TempDir(), "drives")},
	}})
	t.Cleanup(func() { _ = manager.Close() })
	_, release, err := manager.Acquire(uuid.NewV4())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- manager.CloseWithContext(ctx) }()
	select {
	case err := <-closed:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("종료 대기 제한 오류: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("제한 시간 후에도 종료가 완료되지 않았습니다")
	}
	if manager.OpenCount() != 0 {
		t.Fatal("제한 시간 후 드라이브가 남았습니다")
	}
	release()
	release()
}
