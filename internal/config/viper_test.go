package config

import (
	"path/filepath"
	"testing"

	"github.com/shabatoily/govfs/pkg/drivers"
)

func TestResolveConfigExpandsHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Config{}
	cfg.Server.Logger.Path = "~/.govfs/logs/server.log"
	cfg.Server.Logger.AccessLogPath = "~/.govfs/logs/access.log"
	cfg.VFS.Logger.Path = "~/.govfs/logs/vfs.log"
	cfg.VFS.Driver.Type = drivers.DriverTypeBadger
	cfg.VFS.Driver.Badger.Path = "~/.govfs/data"

	if err := resolveConfig(&cfg); err != nil {
		t.Fatal(err)
	}

	wantRoot := filepath.Join(home, ".govfs")
	paths := map[string]string{
		cfg.Server.Logger.Path:          filepath.Join(wantRoot, "logs", "server.log"),
		cfg.Server.Logger.AccessLogPath: filepath.Join(wantRoot, "logs", "access.log"),
		cfg.VFS.Logger.Path:             filepath.Join(wantRoot, "logs", "vfs.log"),
		cfg.VFS.Driver.Badger.Path:      filepath.Join(wantRoot, "data"),
	}
	for got, want := range paths {
		if got != want {
			t.Errorf("홈 경로 확장 결과 = %q, 기대값 = %q", got, want)
		}
	}
}
