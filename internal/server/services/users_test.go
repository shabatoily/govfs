package services

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	badgerdb "github.com/dgraph-io/badger/v4"
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
	for _, lookup := range []func() (User, error){
		func() (User, error) { return store.ByID(admin.ID) },
		func() (User, error) { return store.ByUsername(" ADMIN ") },
	} {
		user, err := lookup()
		if err != nil || user.ID != admin.ID || user.Username != admin.Username || user.Role != admin.Role {
			t.Fatalf("사용자 조회 = %#v, %v", user, err)
		}
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
	for _, tc := range []struct {
		page   int
		userID *uuid.UUID
		total  int
		action string
	}{
		{page: 2, total: 3, action: "vfs.create"},
		{page: 3, total: 3, action: "auth.login"},
		{page: 4, total: 3},
		{page: 2, userID: &admin.ID, total: 2, action: "auth.login"},
		{page: 3, userID: &admin.ID, total: 2},
	} {
		items, count, err := store.ListEvents(tc.page, 1, tc.userID)
		if err != nil || count != tc.total || items == nil {
			t.Fatalf("이벤트 페이지 %d = %#v, 전체 = %d, %v", tc.page, items, count, err)
		}
		if tc.action == "" {
			if len(items) != 0 {
				t.Fatalf("마지막 이후 이벤트 = %#v", items)
			}
		} else if len(items) != 1 || items[0].Action != tc.action || (tc.userID != nil && items[0].UserID != *tc.userID) {
			t.Fatalf("이벤트 페이지 순서·필터 = %#v", items)
		}
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
	stats, users, err := store.Stats()
	if err != nil || stats.Items != 5 || stats.Size == 0 || users != 2 {
		t.Fatalf("시스템 DB 통계 = %#v, 사용자 = %d, %v", stats, users, err)
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

func TestStoreUserLookupErrors(t *testing.T) {
	store, err := OpenUserStore(filepath.Join(t.TempDir(), "users"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id := uuid.NewV4()
	if _, err := store.ByID(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("없는 ID 조회 = %v", err)
	}
	if _, err := store.ByUsername("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("없는 사용자명 조회 = %v", err)
	}
	for _, tc := range []struct {
		name  string
		value []byte
	}{
		{name: "잘못된 인덱스", value: []byte("invalid")},
		{name: "레코드 없는 인덱스", value: id[:]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.db.Update(func(txn *badgerdb.Txn) error {
				return txn.Set(usernameKey("member"), tc.value)
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ByUsername("member"); err == nil {
				t.Fatal("손상된 인덱스 조회가 성공했습니다")
			} else if len(tc.value) == len(id) && !errors.Is(err, ErrNotFound) {
				t.Fatalf("누락된 사용자 레코드 오류 = %v", err)
			}
		})
	}
	if err := store.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(userKey(id), []byte("invalid JSON"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ByID(id); err == nil {
		t.Fatal("손상된 사용자 ID 조회가 성공했습니다")
	}
	if _, err := store.ByUsername("member"); err == nil {
		t.Fatal("손상된 사용자명 조회가 성공했습니다")
	}
}

func TestStoreSystemStatsAndPages(t *testing.T) {
	store, err := OpenUserStore(filepath.Join(t.TempDir(), "users"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	values := map[string]string{
		"user:one": `{"username":"one"}`,
		"user:two": `{"username":"two","disabled":true}`,
		"other:a":  strings.Repeat("a", 64*1024),
		"other:b":  "b",
	}
	var size int64
	if err := store.db.Update(func(txn *badgerdb.Txn) error {
		for key, value := range values {
			size += int64(len(key) + len(value))
			if err := txn.Set([]byte(key), []byte(value)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stats, users, err := store.Stats()
	if err != nil || stats.Items != len(values) || stats.Size != size || users != 2 {
		t.Fatalf("시스템 DB 통계 = %#v, 사용자 = %d, %v", stats, users, err)
	}
	for page := 1; page <= 3; page++ {
		entries, total, err := store.ListSystemEntries(page, 2)
		if err != nil || total != len(values) || entries == nil {
			t.Fatalf("페이지 %d = %#v, 전체 = %d, %v", page, entries, total, err)
		}
		switch page {
		case 1:
			if len(entries) != 2 || entries[0].Kind != "unknown" || entries[1].Kind != "unknown" || entries[0].Value != "<redacted>" {
				t.Fatalf("시스템 값 마스킹 = %#v", entries)
			}
		case 2:
			if len(entries) != 2 || entries[0].Value.(types.UserRes).Username != "one" || entries[1].Value.(types.UserRes).Username != "two" {
				t.Fatalf("사용자 페이지 = %#v", entries)
			}
		case 3:
			if len(entries) != 0 {
				t.Fatalf("마지막 이후 페이지 = %#v", entries)
			}
		}
	}
}
