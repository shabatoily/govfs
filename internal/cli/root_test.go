package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/shabatoily/govfs/internal/config"
)

func TestSetUserConfigUsesPrivatePermissions(t *testing.T) {
	previous := configPath
	configPath = t.TempDir()
	t.Cleanup(func() { configPath = previous })

	path := filepath.Join(configPath, "config")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setUserConfig(&UserConfig{ServerURL: "http://localhost:3000"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("설정 파일 권한 = %o", info.Mode().Perm())
	}
}

func TestVersionWithoutConfig(t *testing.T) {
	root := NewRootCommand(config.AppInfo{Name: "govfs-cli", Version: "v1.2.3"})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--config", "/dev/null/missing", "--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "govfs-cli version v1.2.3\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
