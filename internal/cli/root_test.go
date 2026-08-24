package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetUserConfigUsesPrivatePermissions(t *testing.T) {
	previous := configPath
	configPath = t.TempDir()
	t.Cleanup(func() { configPath = previous })

	path := filepath.Join(configPath, "config")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetUserConfig(&UserConfig{ServerURL: "http://localhost:3000"}); err != nil {
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
