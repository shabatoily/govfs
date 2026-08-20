package services

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
)

func TestDriveManagerSeparatesUsers(t *testing.T) {
	manager := NewDriveManager(badger.Config{Path: filepath.Join(t.TempDir(), "drives")})
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
}
