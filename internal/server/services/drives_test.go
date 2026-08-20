package services

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
)

func TestDriveManagerSeparatesUsers(t *testing.T) {
	manager := NewDriveManager(badger.Config{Path: filepath.Join(t.TempDir(), "drives")})
	t.Cleanup(func() { _ = manager.Close() })
	first, err := manager.Drive(uuid.New())
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
}
