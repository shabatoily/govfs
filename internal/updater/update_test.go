package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdate(t *testing.T) {
	for _, scenario := range []string{"success", "checksum mismatch", "missing checksum", "check", "current CLI with old server", "CLI only", "already current"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			cli := filepath.Join(dir, "govfs-cli")
			server := filepath.Join(dir, "govfs")
			for _, path := range []string{cli, server} {
				if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH
			binary := "new"
			sum := fmt.Sprintf("%x", sha256.Sum256([]byte(binary)))
			requests := []string{}
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/latest":
					fmt.Fprintf(w, `{"tag_name":"v2.0.0","assets":[{"name":"checksums.txt"},{"name":"govfs-cli%s"},{"name":"govfs%s"}]}`, suffix, suffix)
				case "/v2.0.0/checksums.txt":
					if scenario == "missing checksum" {
						http.NotFound(w, r)
						return
					}
					fmt.Fprintf(w, "%s  govfs-cli%s\n%s  govfs%s\n", sum, suffix, sum, suffix)
				case "/v2.0.0/govfs-cli" + suffix, "/v2.0.0/govfs" + suffix:
					if scenario == "checksum mismatch" && strings.HasPrefix(r.URL.Path, "/v2.0.0/govfs-") {
						fmt.Fprint(w, "corrupted")
						return
					}
					fmt.Fprint(w, binary)
				default:
					http.NotFound(w, r)
				}
			}))
			defer remote.Close()
			u := updater{client: remote.Client(), apiURL: remote.URL + "/latest", downloadURL: remote.URL + "/"}
			version := "v1.0.0"
			if scenario == "current CLI with old server" || scenario == "already current" {
				version = "v2.0.0"
			}
			var output bytes.Buffer
			err := u.run(context.Background(), Options{Version: version, Executable: cli, Server: scenario != "CLI only" && scenario != "already current", Check: scenario == "check"}, &output)
			wantError := scenario == "checksum mismatch" || scenario == "missing checksum"
			if (err != nil) != wantError {
				t.Fatalf("err = %v", err)
			}
			for _, path := range []string{cli, server} {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				want := "new"
				if wantError || scenario == "check" || scenario == "already current" || (scenario == "CLI only" && path == server) {
					want = "old"
				}
				if string(data) != want {
					t.Fatalf("%s = %q, want %q", path, data, want)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o755 {
					t.Fatalf("mode = %v", info.Mode())
				}
			}
			if scenario == "check" && len(requests) != 1 {
				t.Fatalf("check downloaded assets: %v", requests)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 {
				t.Fatalf("temporary files retained: %v", entries)
			}
		})
	}
}

func TestReplaceRollsBack(t *testing.T) {
	dir := t.TempDir()
	items := []replacement{}
	for _, name := range []string{"cli", "server"} {
		path := filepath.Join(dir, name)
		temp := path + ".new"
		if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if name == "cli" {
			if err := os.WriteFile(temp, []byte("new"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		items = append(items, replacement{path: path, temp: temp})
	}
	if err := replace(items); err == nil {
		t.Fatal("expected replacement failure")
	}
	for _, item := range items {
		data, err := os.ReadFile(item.path)
		if err != nil || string(data) != "old" {
			t.Fatalf("rollback = %q, %v", data, err)
		}
	}
}

func TestParseChecksumsRejectsInvalidManifest(t *testing.T) {
	hash := fmt.Sprintf("%x", sha256.Sum256(nil))
	for _, text := range []string{"bad  binary", hash + "  binary\n" + hash + "  binary", hash} {
		if _, err := parseChecksums(strings.NewReader(text)); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
