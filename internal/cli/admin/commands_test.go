package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
)

func TestAdminCommands(t *testing.T) {
	const id = "018f0000-0000-7000-8000-000000000001"
	tests := []struct {
		args                      []string
		method, path, query, body string
		status                    int
	}{
		{[]string{"status"}, "GET", "/admin/status", "", "", 200},
		{[]string{"users", "list"}, "GET", "/admin/users", "", "", 200},
		{[]string{"users", "status", id}, "GET", "/admin/users/" + id + "/status", "", "", 200},
		{[]string{"users", "create", "alice", "--password", "secret", "--role", "admin"}, "POST", "/admin/users", "", `{"username":"alice","password":"secret","role":"admin"}`, 201},
		{[]string{"users", "update", id, "--disabled=false"}, "PATCH", "/admin/users/" + id, "", `{"password":"","role":null,"disabled":false}`, 200},
		{[]string{"users", "update", id, "--role", "user", "--password", "new-secret"}, "PATCH", "/admin/users/" + id, "", `{"password":"new-secret","role":"user","disabled":null}`, 200},
		{[]string{"users", "clear-events", id}, "DELETE", "/admin/users/" + id + "/events", "", "", 204},
		{[]string{"events", "--user-id", id, "--page", "2", "--page-size", "5"}, "GET", "/admin/events", "page=2&pageSize=5&userId=" + id, "", 200},
		{[]string{"system", "entries"}, "GET", "/admin/system/entries", "page=1&pageSize=20", "", 200},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing session token")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/auth/me" {
					_, _ = w.Write([]byte(`{"token":"test-token"}`))
					return
				}
				called = true
				if r.Method != tt.method || r.URL.Path != tt.path || r.URL.RawQuery != tt.query {
					t.Errorf("request = %s %s", r.Method, r.URL)
				}
				if tt.body != "" {
					var got, want any
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
						t.Fatal(err)
					}
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(want)
					if !bytes.Equal(gotJSON, wantJSON) {
						t.Errorf("body = %s, want %s", gotJSON, wantJSON)
					}
				}
				w.WriteHeader(tt.status)
				if tt.status != 204 {
					var response any
					user := types.UserRes{Username: "alice", Role: types.RoleAdmin}
					switch {
					case tt.path == "/admin/status":
						response = types.StatusRes{Users: 3, System: types.StorageStatRes{Items: 12, Size: 4096},
							BadgerDrives: []types.BadgerResourceRes{{LSMSize: 8192}}}
					case tt.path == "/admin/events":
						response = types.UserEventPageRes{Items: []types.UserEventRes{{Username: "alice", Action: "login", Status: 200}}, Page: 2, PageSize: 5, Total: 7}
					case tt.path == "/admin/system/entries":
						response = types.SystemEntryPageRes{Items: []types.SystemEntryRes{{Key: "user:alice", Kind: "user", Value: map[string]any{"username": "alice"}}}, Page: 1, PageSize: 20, Total: 1}
					case strings.HasSuffix(tt.path, "/status"):
						response = types.UserDriveStatusRes{Username: "alice", Online: true, Size: 4096}
					case tt.path == "/admin/users" && tt.method == "GET":
						response = []types.UserRes{user}
					default:
						response = user
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				}
			}))
			defer server.Close()
			root := cli.NewRootCommand(config.AppInfo{Name: "govfs-cli"})
			RegisterCommands(root)
			if err := root.PersistentFlags().Set("config", t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if err := setUserConfig(root, &cli.UserConfig{ServerURL: server.URL, TokenInfo: cli.TokenInfo{TokenRes: types.TokenRes{Token: "test-token", ExpiresAt: time.Now().Add(time.Hour)}}}); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs(append([]string{"admin"}, tt.args...))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("admin endpoint not called")
			}
			if tt.status == 204 && output.Len() != 0 {
				t.Fatalf("delete output = %q", output.String())
			}
			if tt.status != 204 && !strings.Contains(output.String(), "|") {
				t.Fatalf("invalid output: %q", output.String())
			}
			if tt.status != 204 {
				expected := "alice"
				switch tt.path {
				case "/admin/status":
					expected = "8192"
				case "/admin/events":
					expected = "Page: 2 | Page size: 5 | Total: 7"
				case "/admin/system/entries":
					expected = `{"username":"alice"}`
				}
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("output missing %q: %s", expected, output.String())
				}
			}
		})
	}
}

func TestAdminPermissionDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/me" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	root := cli.NewRootCommand(config.AppInfo{Name: "govfs-cli"})
	RegisterCommands(root)
	if err := root.PersistentFlags().Set("config", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := setUserConfig(root, &cli.UserConfig{ServerURL: server.URL, TokenInfo: cli.TokenInfo{TokenRes: types.TokenRes{Token: "test-token", ExpiresAt: time.Now().Add(time.Hour)}}}); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"admin", "status"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v", err)
	}
}

// setUserConfig는 명령 테스트용 로그인 세션을 저장합니다.
func setUserConfig(cmd *cobra.Command, session *cli.UserConfig) error {
	base, err := cmd.PersistentFlags().GetString("config")
	if err != nil {
		return err
	}
	dir := filepath.Join(base, ".govfs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(session)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config"), data, 0o600)
}
