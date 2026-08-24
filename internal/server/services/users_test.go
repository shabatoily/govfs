package services

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	if err := store.RecordEvent(admin, "auth.login", 200); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Nanosecond)
	if err := store.RecordEvent(admin, "vfs.create", 202); err != nil {
		t.Fatal(err)
	}
	events, total, err := store.ListEvents(1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || total != 2 || events[0].Action != "vfs.create" {
		t.Fatalf("최근 이벤트 = %#v", events)
	}
	member, err := store.Create("member", "password", types.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordEvent(member, "auth.login", 200); err != nil {
		t.Fatal(err)
	}
	events, _, err = store.ListEvents(1, 10, &admin.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("사용자 이벤트 = %#v, %v", events, err)
	}
	if deleted, err := store.ClearEvents(admin.ID); err != nil || deleted != 2 {
		t.Fatalf("이벤트 삭제 = %d, %v", deleted, err)
	}
	events, total, err = store.ListEvents(1, 10, &admin.ID)
	if err != nil || total != 0 || len(events) != 0 {
		t.Fatalf("삭제 후 이벤트 = %#v, %d, %v", events, total, err)
	}
	stats, err := store.Stats()
	if err != nil || stats.Items == 0 || stats.Size == 0 {
		t.Fatalf("시스템 DB 통계 = %#v, %v", stats, err)
	}
	entries, total, err := store.ListSystemEntries(1, 10)
	if err != nil || total == 0 || len(entries) == 0 {
		t.Fatalf("시스템 DB 상세 = %#v, %d, %v", entries, total, err)
	}
	for _, entry := range entries {
		if value, ok := entry.Value.(types.UserRes); ok && value.Username == "" {
			t.Fatal("사용자 상세 변환 실패")
		}
	}
}
