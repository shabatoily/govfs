package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
)

// 검증은 같은 서버에서 순차 실행하며 사례 이름을 보고서 식별자로 사용합니다.
func (e *environment) runScenarios(t *testing.T, driver string) {
	root := "/e2e"
	var directory vfs.Meta
	e.callTool(t, "vfs_mkdir", map[string]string{"path": root}, &directory, false)
	ids := map[string]string{}
	// tree와 검색의 기대 목록이 바뀌지 않도록 샘플 검증을 먼저 실행합니다.
	e.testFixtures(t, root, ids)
	e.testMCP(t, root)
	e.testHTTP(t, root, ids)
	e.testCLI(t, root, directory)
	e.testAuth(t, ids)
	e.testRecovery(t, root, driver)
	t.Run("cleanup", func(t *testing.T) {
		e.callTool(t, "vfs_delete", map[string]string{"id": directory.ID.String()}, nil, false)
		e.requestJSON(t, "GET", "/vfs/"+directory.ID.String()+"/stat", nil, 404, nil)
	})
	t.Run("browser", func(t *testing.T) { t.Skip("requires agent browser execution; see docs/testing/AGENT_E2E.md") })
}

func (e *environment) testFixtures(t *testing.T, root string, ids map[string]string) {
	t.Run("MCP-01/tools", func(t *testing.T) {
		list, err := e.session.ListTools(t.Context(), nil)
		must(t, err)
		var names []string
		for _, tool := range list.Tools {
			names = append(names, tool.Name)
		}
		slices.Sort(names)
		want := []string{"vfs_delete", "vfs_mkdir", "vfs_stat", "vfs_tree", "vfs_upload"}
		if !slices.Equal(names, want) {
			t.Fatalf("tools = %v", names)
		}
		content, err := json.MarshalIndent(list, "", "  ")
		must(t, err)
		write(t, filepath.Join(e.dir, "tools.json"), content)
	})
	for _, sample := range e.fixtures {
		t.Run("fixture/"+sample.Path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(e.fixtureRoot, sample.Path))
			must(t, err)
			meta := e.upload(t, root+"/"+sample.Path, data)
			ids[sample.Path] = meta.ID.String()
			if meta.Size != int64(sample.Size) || meta.Path != root+"/"+sample.Path {
				t.Fatalf("upload metadata: %+v", meta)
			}
			var stat vfs.Meta
			e.callTool(t, "vfs_stat", map[string]string{"id": meta.ID.String()}, &stat, false)
			if stat.ID != meta.ID || stat.Size != meta.Size {
				t.Fatal("stat differs from upload metadata")
			}
			t.Run("download", func(t *testing.T) {
				status, headers, content := e.request(t, "GET", "/vfs/"+meta.ID.String(), nil, nil)
				if status != 200 || fmt.Sprintf("%x", sha256.Sum256(content)) != sample.SHA256 {
					detail := content
					if len(detail) > 300 {
						detail = detail[:300]
					}
					t.Fatalf("download status=%d, bytes=%d, want %d; hash mismatch; body prefix=%q", status, len(content), sample.Size, detail)
				}
				if strings.Split(headers.Get("Content-Type"), ";")[0] != sample.MIME {
					t.Errorf("MIME = %q, want %q", headers.Get("Content-Type"), sample.MIME)
				}
			})
		})
	}
	content, err := json.MarshalIndent(ids, "", "  ")
	must(t, err)
	write(t, filepath.Join(e.dir, "ids.json"), content)
}

func (e *environment) testMCP(t *testing.T, root string) {
	t.Run("MCP-02/tree", func(t *testing.T) {
		var tree vfs.TreeNode
		e.callTool(t, "vfs_tree", map[string]string{"path": root}, &tree, false)
		if len(tree.Children) != len(e.fixtures) {
			t.Fatalf("children=%d want %d", len(tree.Children), len(e.fixtures))
		}
	})
	for _, input := range []struct {
		name string
		args map[string]string
	}{
		{"vfs_tree", map[string]string{"path": ""}},
		{"vfs_mkdir", map[string]string{"path": "relative"}},
		{"vfs_stat", map[string]string{"id": "invalid"}},
		{"vfs_upload", map[string]string{"path": root + "/bad", "content_base64": "!bad!"}},
		{"vfs_mkdir", map[string]string{"path": root}},
	} {
		t.Run("MCP-04/invalid/"+input.name, func(t *testing.T) { e.callTool(t, input.name, input.args, nil, true) })
	}
	for _, size := range []int{0, (10 << 20) - 2, (10 << 20) - 1, 10 << 20, (10 << 20) + 1} {
		t.Run(fmt.Sprintf("MCP-04/size-%d", size), func(t *testing.T) {
			var meta vfs.Meta
			e.callTool(t, "vfs_upload", map[string]string{"path": fmt.Sprintf("%s/boundary-%d.bin", root, size), "content_base64": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, size))}, &meta, size > 10<<20)
			if size <= 10<<20 {
				e.callTool(t, "vfs_delete", map[string]string{"id": meta.ID.String()}, nil, false)
			}
		})
	}
}

func (e *environment) testHTTP(t *testing.T, root string, ids map[string]string) {
	for _, query := range []string{"NOTE", "한글", "space", "no-such-match", ""} {
		t.Run("API-04/search/"+query, func(t *testing.T) {
			if query == "" {
				e.requestJSON(t, "GET", "/vfs/search?q=", nil, 400, nil)
				return
			}
			var found []vfs.Meta
			e.requestJSON(t, "GET", "/vfs/search?q="+url.QueryEscape(query), nil, 200, &found)
			var got, want []string
			for _, meta := range found {
				got = append(got, meta.Path)
			}
			for _, sample := range e.fixtures {
				if strings.Contains(strings.ToLower(sample.Path), strings.ToLower(query)) {
					want = append(want, root+"/"+sample.Path)
				}
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
	t.Run("API-02/write-comment-cache", func(t *testing.T) {
		meta := e.upload(t, root+"/edit.go", []byte("original"))
		id := meta.ID.String()
		e.requestJSON(t, "PUT", "/vfs/"+id, map[string]string{"content": "edited 한글\n"}, 202, nil)
		poll(t, func() bool {
			_, _, content := e.request(t, "GET", "/vfs/"+id, nil, nil)
			return string(content) == "edited 한글\n"
		})
		_, headers, _ := e.request(t, "GET", "/vfs/"+id, nil, nil)
		if headers.Get("Cache-Control") != "no-cache" {
			t.Error("editor content is cached")
		}
		e.requestJSON(t, "PATCH", "/vfs/"+id+"/comments", map[string]string{"comment": "e2e comment"}, 202, nil)
		poll(t, func() bool {
			var meta vfs.Meta
			e.requestJSON(t, "GET", "/vfs/"+id+"/stat", nil, 200, &meta)
			return meta.Comments == "e2e comment"
		})
	})
	for _, rangeValue := range []string{"bytes=0-9", "bytes=999999999-"} {
		t.Run("API-05/"+rangeValue, func(t *testing.T) {
			id := ids["image_0_320x240.png"]
			if id == "" {
				t.Skip("PNG upload failed")
			}
			status, headers, content := e.request(t, "GET", "/vfs/"+id, nil, http.Header{"Range": {rangeValue}})
			if rangeValue == "bytes=0-9" {
				data, err := os.ReadFile(filepath.Join(e.fixtureRoot, "image_0_320x240.png"))
				must(t, err)
				if status != 206 || !bytes.Equal(content, data[:10]) || headers.Get("Content-Range") != fmt.Sprintf("bytes 0-9/%d", len(data)) {
					t.Fatalf("status=%d Content-Range=%q bytes=%d", status, headers.Get("Content-Range"), len(content))
				}
			} else if status != 416 {
				t.Fatalf("status=%d want 416", status)
			}
		})
	}
	t.Run("API-03/transfer", func(t *testing.T) {
		meta := e.upload(t, root+"/source.txt", []byte("copy me"))
		id := meta.ID.String()
		var copied vfs.Meta
		dst := map[string]any{"name": root + "/copy.txt", "checkConflict": true}
		e.requestJSON(t, "POST", "/vfs/"+id+"/copy?wait=true", dst, 200, &copied)
		e.requestJSON(t, "POST", "/vfs/"+id+"/copy?wait=true", dst, 409, nil)
		e.requestJSON(t, "PATCH", "/vfs/"+copied.ID.String()+"?wait=true", map[string]any{"name": root + "/moved.txt", "checkConflict": true}, 200, nil)
		e.requestJSON(t, "POST", "/vfs/"+id+"/copy?wait=true", map[string]any{"name": root + "/moved.txt", "checkConflict": true, "replaceId": id}, 409, nil)
		var replaced vfs.Meta
		e.requestJSON(t, "POST", "/vfs/"+id+"/copy?wait=true", map[string]any{"name": root + "/moved.txt", "checkConflict": true, "replaceId": copied.ID.String()}, 200, &replaced)
		_, _, content := e.request(t, "GET", "/vfs/"+replaced.ID.String(), nil, nil)
		if string(content) != "copy me" {
			t.Fatalf("copied file HTTP content=%q, want %q", content, "copy me")
		}
		e.requestJSON(t, "GET", "/vfs/"+id+"/stat", nil, 200, nil)
	})
}

func (e *environment) testCLI(t *testing.T, root string, directory vfs.Meta) {
	t.Run("CLI-01/commands", func(t *testing.T) {
		e.cli(t, "ls", root)
		e.cli(t, "tree", root)
		e.cli(t, "search", "NOTE")
		e.cli(t, "stat", directory.ID.String())
		e.cli(t, "mkdir", root+"/cli")
		e.cli(t, "cp", filepath.Join(e.fixtureRoot, "note.txt"), "vfs:"+root+"/cli.txt")
		poll(t, func() bool {
			var rows []vfs.Meta
			e.requestJSON(t, "GET", "/vfs/search?q=cli.txt", nil, 200, &rows)
			return len(rows) == 1
		})
		target := filepath.Join(e.dir, "cli-download.txt")
		e.cli(t, "cp", "vfs:"+root+"/cli.txt", target)
		got, err := os.ReadFile(target)
		must(t, err)
		want, err := os.ReadFile(filepath.Join(e.fixtureRoot, "note.txt"))
		must(t, err)
		if !bytes.Equal(got, want) {
			t.Error("CLI download differs")
		}
		e.cli(t, "rm", root+"/cli.txt")
		poll(t, func() bool {
			var rows []vfs.Meta
			e.requestJSON(t, "GET", "/vfs/search?q=cli.txt", nil, 200, &rows)
			return len(rows) == 0
		})
	})
}

func (e *environment) testAuth(t *testing.T, ids map[string]string) {
	t.Run("AUTH-01/missing-token", func(t *testing.T) {
		saved := e.token
		e.token = ""
		defer func() { e.token = saved }()
		status, _, _ := e.request(t, "GET", "/vfs?q=/", nil, nil)
		if status != 400 && status != 401 {
			t.Fatalf("unauthenticated access status=%d", status)
		}
	})
	t.Run("AUTH-01/bad-password", func(t *testing.T) {
		e.requestJSON(t, "POST", "/auth/login", map[string]string{"username": "e2e-admin", "password": "incorrect"}, 401, nil)
	})
	for _, name := range []string{"alice", "bob"} {
		t.Run("AUTH-01/"+name, func(t *testing.T) {
			var user types.UserRes
			e.requestJSON(t, "POST", "/admin/users", map[string]string{"username": name, "password": e.password, "role": "user"}, 201, &user)
			var login types.TokenRes
			e.requestJSON(t, "POST", "/auth/login", map[string]string{"username": name, "password": e.password}, 200, &login)
			admin := e.token
			e.token = login.Token
			defer func() { e.token = admin }()
			e.requestJSON(t, "GET", "/admin/users", nil, 403, nil)
			if id := ids["note.txt"]; id != "" {
				e.requestJSON(t, "GET", "/vfs/"+id+"/stat", nil, 404, nil)
			}
			var found []vfs.Meta
			e.requestJSON(t, "GET", "/vfs/search?q=note", nil, 200, &found)
			if len(found) != 0 {
				t.Error("other user's files leaked")
			}
			e.token = admin
			e.requestJSON(t, "PATCH", "/admin/users/"+user.ID.String(), map[string]bool{"disabled": true}, 200, nil)
			e.token = login.Token
			e.requestJSON(t, "GET", "/auth/me", nil, 401, nil)
			e.token = admin
			e.requestJSON(t, "PATCH", "/admin/users/"+user.ID.String(), map[string]bool{"disabled": false}, 200, nil)
			e.requestJSON(t, "GET", "/admin/users/"+user.ID.String()+"/status", nil, 200, nil)
			e.requestJSON(t, "DELETE", "/admin/users/"+user.ID.String()+"/events", nil, 204, nil)
		})
	}
	for _, route := range []string{"/admin/users", "/admin/status", "/admin/system/entries?page=1&pageSize=2", "/admin/events?page=1&pageSize=2", "/sse/clients"} {
		t.Run("ADMIN-01"+route, func(t *testing.T) {
			status, _, content := e.request(t, "GET", route, nil, nil)
			if status != 200 || bytes.Contains(content, []byte("passwordHash")) {
				t.Fatalf("status=%d, passwordHash exposed=%v", status, bytes.Contains(content, []byte("passwordHash")))
			}
		})
	}
}

func (e *environment) testRecovery(t *testing.T, root, driver string) {
	t.Run("MCP-05/delete", func(t *testing.T) {
		meta := e.upload(t, root+"/delete.txt", []byte("delete"))
		args := map[string]string{"id": meta.ID.String()}
		e.callTool(t, "vfs_delete", args, nil, false)
		e.requestJSON(t, "GET", "/vfs/"+meta.ID.String()+"/stat", nil, 404, nil)
		e.callTool(t, "vfs_delete", args, nil, true)
	})
	t.Run("CLI-01/backup-filename", func(t *testing.T) {
		name := filepath.Join(e.dir, "exact.bak")
		e.cli(t, "backup", "-f", name)
		_, err := os.Stat(name)
		must(t, err)
	})
	t.Run("API-06/backup-restore", func(t *testing.T) {
		meta := e.upload(t, root+"/restore.txt", []byte("before backup"))
		id := meta.ID.String()
		e.cli(t, "backup", "-f", filepath.Join(e.dir, "backup-%s.bak"))
		matches, err := filepath.Glob(filepath.Join(e.dir, "backup-*.bak"))
		must(t, err)
		if len(matches) != 1 {
			t.Fatalf("backups=%v", matches)
		}
		e.requestJSON(t, "PUT", "/vfs/"+id, map[string]string{"content": "after backup"}, 202, nil)
		poll(t, func() bool {
			_, _, content := e.request(t, "GET", "/vfs/"+id, nil, nil)
			return string(content) == "after backup"
		})
		e.cli(t, "restore", "-f", matches[0])
		_, _, content := e.request(t, "GET", "/vfs/"+id, nil, nil)
		want := "before backup"
		if driver == "badger" {
			// Badger Load는 백업 버전을 적재하며 기존의 더 최신 버전을 보존합니다.
			want = "after backup"
		}
		if string(content) != want {
			t.Fatalf("restored content=%q, want %q", content, want)
		}
	})
	for _, route := range []string{"/badger/keys", "/badger/stats"} {
		t.Run("DRIVER-01"+route, func(t *testing.T) {
			status, _, _ := e.request(t, "GET", route, nil, nil)
			if driver == "badger" && status != 200 || driver == "localstorage" && status < 400 {
				t.Fatalf("status=%d", status)
			}
		})
	}
	t.Run("RECOVERY-01/restart", func(t *testing.T) {
		meta := e.upload(t, root+"/persist.txt", []byte("persistent"))
		e.stop(t)
		e.launch(t)
		e.connect(t)
		_, _, content := e.request(t, "GET", "/vfs/"+meta.ID.String(), nil, nil)
		if string(content) != "persistent" {
			t.Fatalf("HTTP content after restart=%q, want %q", content, "persistent")
		}
		e.upload(t, root+"/after-restart.txt", []byte("new"))
	})
}
