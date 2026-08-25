package services

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
	"github.com/shabatoily/govfs/pkg/drivers/localstorage"
)

func TestDriveManagerSeparatesUsers(t *testing.T) {
	for _, driverType := range []drivers.DriverType{drivers.DriverTypeBadger, drivers.DriverTypeLocalStorage} {
		t.Run(string(driverType), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "drives")
			manager := NewDriveManager(drivers.Config{
				Type:         driverType,
				Badger:       badger.Config{Path: root},
				LocalStorage: localstorage.Config{Path: root},
			})
			t.Cleanup(func() { _ = manager.Close() })
			firstID := uuid.New()
			first, err := manager.Drive(firstID)
			if err != nil {
				t.Fatal(err)
			}
			second, err := manager.Drive(uuid.New())
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
			stats, open, err = manager.Stats(uuid.New())
			if err != nil || open || stats.Items != 0 || manager.OpenCount() != 2 {
				t.Fatalf("미개방 드라이브 통계 = %#v, open=%v, count=%d, err=%v", stats, open, manager.OpenCount(), err)
			}
		})
	}
}
