package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/shabatoily/govfs/pkg/drivers/localstorage"
	vfsLog "github.com/shabatoily/govfs/pkg/log"
)

func TestInitFailureClosesUserStore(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{VFS: config.VfsConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(root, "drives")},
	}}}
	if _, err := Init(cfg); err == nil {
		t.Fatal("관리자 정보가 없으면 초기화에 실패해야 합니다")
	}
	users, err := services.OpenUserStore(filepath.Join(root, "system", "users"))
	if err != nil {
		t.Fatalf("초기화 실패 후 저장소 재열기: %v", err)
	}
	if err := users.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownKeepsUserStoreOpenUntilRequestCompletes(t *testing.T) {
	users, err := services.OpenUserStore(filepath.Join(t.TempDir(), "users"))
	if err != nil {
		t.Fatal(err)
	}
	defer users.Close()
	logger, err := vfsLog.NewLogger(vfsLog.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.SetContext(context.Background())
	drives := services.NewDriveManager(services.DriveManagerConfig{})
	app := initServer(serverContext{Config: cfg, Users: users, Drives: drives, VFSLogger: logger})
	shutdownDone := false
	t.Cleanup(func() {
		if !shutdownDone {
			_ = app.ShutdownWithTimeout(time.Second)
		}
	})
	started, shuttingDown := make(chan struct{}), make(chan struct{})
	app.Hooks().OnPreShutdown(func() error { close(shuttingDown); return nil })
	app.Get("/inflight", func(c fiber.Ctx) error {
		close(started)
		select {
		case <-shuttingDown:
		case <-time.After(5 * time.Second):
			return c.SendStatus(http.StatusGatewayTimeout)
		}
		if _, err := users.List(); err != nil {
			return c.SendStatus(http.StatusInternalServerError)
		}
		return c.SendStatus(http.StatusOK)
	})
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listening := make(chan error, 1)
	go func() { listening <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true}) }()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+listener.Addr().String()+"/inflight", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				err = fiber.NewError(res.StatusCode)
			}
		}
		response <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("요청 시작 대기 시간 초과")
	}
	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	shutdownDone = true
	if err := <-response; err != nil {
		t.Fatalf("종료 중 요청 처리: %v", err)
	}
	if err := <-listening; err != nil {
		t.Fatal(err)
	}
	if _, err := users.List(); err == nil {
		t.Fatal("종료 후 저장소가 닫혀야 합니다")
	}
}

func TestShutdownKeepsResourcesOpenUntilAsyncDriveRelease(t *testing.T) {
	root := t.TempDir()
	users, err := services.OpenUserStore(filepath.Join(root, "users"))
	if err != nil {
		t.Fatal(err)
	}
	defer users.Close()
	logger, err := vfsLog.NewLogger(vfsLog.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.SetContext(t.Context())
	drives := services.NewDriveManager(services.DriveManagerConfig{Driver: drivers.Config{
		Type:         drivers.DriverTypeLocalStorage,
		LocalStorage: localstorage.Config{Path: filepath.Join(root, "drives")},
	}})
	app := initServer(serverContext{Config: cfg, Users: users, Drives: drives, VFSLogger: logger})
	userID := uuid.NewV4()
	_, release, err := drives.Acquire(userID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	closed := make(chan error, 1)
	go func() { closed <- app.Shutdown() }()
	deadline := time.Now().Add(time.Second)
	for {
		_, done, err := drives.Acquire(userID)
		if errors.Is(err, os.ErrClosed) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		done()
		if time.Now().After(deadline) {
			t.Fatal("종료 시작 대기 시간 초과")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := users.List(); err != nil {
		t.Fatalf("비동기 작업 완료 전 사용자 저장소가 닫혔습니다: %v", err)
	}
	release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("비동기 작업 완료 후 서버 정리 대기 시간 초과")
	}
	if _, err := users.List(); err == nil {
		t.Fatal("종료 후 사용자 저장소가 닫히지 않았습니다")
	}
}
