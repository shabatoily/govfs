package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/spf13/viper"
)

func TestVFSIdleTimeoutDefaultAndDisable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetDefault("vfs.idleTimeout", DefaultConfig.VFS.IdleTimeout)

	cfg := Config{}
	if err := setConfigFromViper(&cfg, AppInfo{}); err != nil {
		t.Fatal(err)
	}
	if cfg.VFS.IdleTimeout != 30*time.Minute {
		t.Fatalf("기본 idle timeout = %s", cfg.VFS.IdleTimeout)
	}

	viper.Set("vfs.idleTimeout", 0)
	if err := setConfigFromViper(&cfg, AppInfo{}); err != nil {
		t.Fatal(err)
	}
	if cfg.VFS.IdleTimeout != 0 {
		t.Fatalf("비활성 idle timeout = %s", cfg.VFS.IdleTimeout)
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
