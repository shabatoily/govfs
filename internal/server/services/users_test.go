package services

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/shabatoily/govfs/internal/types"
)

func TestStoreUserLifecycle(t *testing.T) {
	store, err := OpenUserStore(filepath.Join(t.TempDir(), "users"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	admin, err := store.Create("Admin", "password", types.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("admin", "password", types.RoleUser); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("중복 사용자 오류 = %v", err)
	}
	if _, err := store.Authenticate("ADMIN", "password"); err != nil {
		t.Fatal(err)
	}
	disabled := true
	if _, err := store.Update(admin.ID, UserUpdate{Disabled: &disabled}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("마지막 관리자 오류 = %v", err)
	}
}
