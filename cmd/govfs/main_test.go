package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/kardianos/service"
	"github.com/shabatoily/govfs/internal/config"
)

func TestServiceCommands(t *testing.T) {
	root := newRootCommand(config.AppInfo{Name: "govfs"})
	for _, action := range []string{"install", "start", "stop", "restart", "uninstall", "status"} {
		command, _, err := root.Find([]string{"service", action})
		if err != nil {
			t.Errorf("service %s 명령 조회: %v", action, err)
			continue
		}
		if command.Name() != action {
			t.Errorf("service %s 명령 대신 %s 조회", action, command.Name())
		}
	}
}

func TestServiceStatusText(t *testing.T) {
	tests := map[service.Status]string{
		service.StatusUnknown: "unknown",
		service.StatusRunning: "running",
		service.StatusStopped: "stopped",
	}
	for status, want := range tests {
		if got := serviceStatusText(status); got != want {
			t.Errorf("serviceStatusText(%d) = %q, 기대값 %q", status, got, want)
		}
	}
}

func TestVersionWithoutConfig(t *testing.T) {
	for _, arg := range []string{"--version", "version"} {
		t.Run(arg, func(t *testing.T) {
			root := newRootCommand(config.AppInfo{Name: "govfs", Version: "v1.2.3"})
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs([]string{"--config", "/dev/null/missing", arg})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			want := "govfs version v1.2.3\n"
			if arg == "version" {
				want = "govfs v1.2.3\n"
			}
			if got := output.String(); got != want {
				t.Fatalf("version output = %q, want %q", got, want)
			}
		})
	}
}

func TestRootConfigFlagsAreIndependent(t *testing.T) {
	first := newRootCommand(config.AppInfo{Name: "first"})
	if err := first.PersistentFlags().Set("config", "first.toml"); err != nil {
		t.Fatal(err)
	}
	second := newRootCommand(config.AppInfo{Name: "second"})
	if err := second.PersistentFlags().Set("config", "second.toml"); err != nil {
		t.Fatal(err)
	}
	got, err := first.PersistentFlags().GetString("config")
	if err != nil {
		t.Fatal(err)
	}
	if got != "first.toml" {
		t.Fatalf("first config = %q", got)
	}
}

func TestLoadAppReturnsConfigError(t *testing.T) {
	p := &program{configPath: filepath.Join(t.TempDir(), "missing.toml")}
	app, address, err := p.loadApp(context.Background())
	if err == nil || app != nil || address != "" {
		t.Fatalf("loadApp = %v, %q, %v", app, address, err)
	}
}

func TestForegroundWaitsForShutdownHooks(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	listening := make(chan string, 1)
	app.Hooks().OnListen(func(data fiber.ListenData) error {
		listening <- net.JoinHostPort(data.Host, data.Port)
		return nil
	})
	cleanupStarted, finishCleanup := make(chan struct{}), make(chan struct{})
	var cleanupOnce, releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(finishCleanup) }) })
	app.Hooks().OnPostShutdown(func(_ error) error {
		cleanupOnce.Do(func() { close(cleanupStarted); <-finishCleanup })
		return nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- listenForeground(ctx, app, "127.0.0.1:0") }()
	var address string
	select {
	case address = <-listening:
	case <-time.After(time.Second):
		t.Fatal("서버 시작 대기 시간 초과")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	cancel()
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("리소스 정리 시작 대기 시간 초과")
	}
	select {
	case err := <-result:
		t.Fatalf("리소스 정리 전 직접 실행이 반환되었습니다: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(finishCleanup) })
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("리소스 정리 후 직접 실행 종료 대기 시간 초과")
	}
}
