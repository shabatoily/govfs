package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/shabatoily/govfs/internal/config"
)

func TestSetUserConfigUsesPrivatePermissions(t *testing.T) {
	root := NewRootCommand(config.AppInfo{Name: "govfs-cli"})
	base := t.TempDir()
	if err := root.PersistentFlags().Set("config", base); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, ".govfs")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setUserConfig(root, &UserConfig{ServerURL: "http://localhost:3000"}); err != nil {
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

// TestInfoWithoutConfig는 정보 출력이 세션과 파일 시스템에 의존하지 않는지 확인합니다.
func TestInfoWithoutConfig(t *testing.T) {
	for _, args := range [][]string{{"info"}, {"info", "-v"}, {"info", "--verbose=false"}} {
		root := NewRootCommand(config.AppInfo{Name: "govfs-cli", Version: "v1.2.3"})
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		root.SetArgs(append([]string{"--config", "/dev/null/missing"}, args...))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if output.Len() == 0 {
			t.Fatal("empty info output")
		}
	}
}

func TestConfigPathsAreIndependent(t *testing.T) {
	roots := []*cobra.Command{
		NewRootCommand(config.AppInfo{Name: "first"}),
		NewRootCommand(config.AppInfo{Name: "second"}),
	}
	for i, root := range roots {
		base := t.TempDir()
		if err := root.PersistentFlags().Set("config", base); err != nil {
			t.Fatal(err)
		}
		u := UserConfig{Username: fmt.Sprint(i)}
		for range 2 {
			if err := setUserConfig(root, &u); err != nil {
				t.Fatal(err)
			}
			got, err := GetUserConfig(root)
			if err != nil || got.Username != u.Username {
				t.Fatalf("config = %+v, err = %v", got, err)
			}
		}
	}
	for i, root := range roots {
		got, err := GetUserConfig(root)
		if err != nil || got.Username != fmt.Sprint(i) {
			t.Fatalf("config = %+v, err = %v", got, err)
		}
	}
}

func TestReadPasswordRestoresAfterCancellation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	t.Setenv("STTY_TEST_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$STTY_TEST_LOG\"\nif [ \"$1\" = '-g' ]; then printf 'saved-state\\n'; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "stty"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := bufio.NewReader(cancelingReader{cancel: cancel})
	if _, err := readPassword(ctx, reader); !errors.Is(err, io.EOF) {
		t.Fatalf("read error = %v, want EOF", err)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(calls)), "-g\n-echo\nsaved-state"; got != want {
		t.Fatalf("stty calls = %q, want %q", got, want)
	}
}

type cancelingReader struct {
	cancel context.CancelFunc
}

func (r cancelingReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, io.EOF
}
