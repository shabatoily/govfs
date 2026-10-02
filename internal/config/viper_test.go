package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shabatoily/govfs/pkg/drivers"
)

func TestVFSIdleTimeoutDefaultAndDisable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    time.Duration
	}{
		{"default", "", 30 * time.Minute},
		{"disabled", "[vfs]\nidleTimeout = 0\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadWithViper(path, AppInfo{})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.VFS.IdleTimeout != tc.want {
				t.Fatalf("idle timeout = %s, want %s", cfg.VFS.IdleTimeout, tc.want)
			}
		})
	}
}

func TestResolveConfigExpandsHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Config{}
	cfg.Server.Logger.Path = "~/.govfs/logs/server.log"
	cfg.Server.Logger.AccessLogPath = "~/.govfs/logs/access.log"
	cfg.VFS.Logger.Path = "~/.govfs/logs/vfs.log"
	cfg.VFS.Driver.Type = drivers.DriverTypeBadger
	cfg.VFS.Driver.Badger.Path = "~/.govfs/data"
	cfg.VFS.Driver.LocalStorage.Path = "~/.govfs/local"

	if err := resolveConfig(&cfg); err != nil {
		t.Fatal(err)
	}

	wantRoot := filepath.Join(home, ".govfs")
	paths := map[string]string{
		cfg.Server.Logger.Path:           filepath.Join(wantRoot, "logs", "server.log"),
		cfg.Server.Logger.AccessLogPath:  filepath.Join(wantRoot, "logs", "access.log"),
		cfg.VFS.Logger.Path:              filepath.Join(wantRoot, "logs", "vfs.log"),
		cfg.VFS.Driver.Badger.Path:       filepath.Join(wantRoot, "data"),
		cfg.VFS.Driver.LocalStorage.Path: filepath.Join(wantRoot, "local"),
	}
	for got, want := range paths {
		if got != want {
			t.Errorf("홈 경로 확장 결과 = %q, 기대값 = %q", got, want)
		}
	}
}

func TestLoadConfigWithoutExtensionInDottedDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "settings.d")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	if _, err := LoadWithViper(path, AppInfo{}); err == nil {
		t.Fatal("missing configuration should return an error")
	}
	if err := os.WriteFile(path+".toml", []byte("[server]\nport = 4321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithViper(path, AppInfo{Name: "govfs", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4321 || cfg.App.Version != "test" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestConcurrentConfigLoadsAreIndependent(t *testing.T) {
	for _, port := range []int{4321, 5432} {
		t.Run(fmt.Sprint(port), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.toml")
			content := fmt.Sprintf("[server]\nport = %d\n", port)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadWithViper(path, AppInfo{})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Port != port {
				t.Fatalf("port = %d, want %d", cfg.Server.Port, port)
			}
		})
	}
}

func TestResolveConfigRejectsInvalidLogDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{}
	cfg.Server.Logger.Path = filepath.Join(parent, "server.log")
	if err := resolveConfig(&cfg); err == nil {
		t.Fatal("log parent is a file, want error")
	}
}
