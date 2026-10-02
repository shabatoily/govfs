package server

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

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
