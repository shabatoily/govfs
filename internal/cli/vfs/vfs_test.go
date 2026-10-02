package vfs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHandler(t *testing.T, handler http.HandlerFunc) (*Handler, *bytes.Buffer) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cmd := &cobra.Command{}
	output := new(bytes.Buffer)
	cmd.SetOut(output)
	return &Handler{cmd: cmd, client: client.New(server.URL)}, output
}

func TestBackupFailurePreservesExistingFile(t *testing.T) {
	h, _ := testHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	dest := filepath.Join(t.TempDir(), "backup")
	require.NoError(t, os.WriteFile(dest, []byte("previous backup"), 0o600))
	require.Error(t, h.Backup(dest))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "previous backup", string(data))
}

func TestRecursiveDownload(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			meta := types.MetaRes{Meta: vfs.Meta{ID: uuid.NewV4(), Name: "report.txt", Path: "/docs/report.txt", Size: 7}}
			tree := &types.TreeNodeRes{
				Meta:     types.MetaRes{Meta: vfs.Meta{Name: "docs", IsDir: true}},
				Children: []*types.TreeNodeRes{{Meta: meta}},
			}
			h, output := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/vfs":
					assert.NoError(t, json.NewEncoder(w).Encode(types.VfsRes[*types.TreeNodeRes]{Payload: tree}))
				case "/vfs/" + meta.ID.String() + "/stat":
					assert.NoError(t, json.NewEncoder(w).Encode(meta))
				default:
					if fail {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					_, err := w.Write([]byte("content"))
					assert.NoError(t, err)
				}
			})
			dest := t.TempDir()
			err := h.handleRecursiveDownload("/docs", dest)
			if fail {
				require.Error(t, err)
				require.NotContains(t, output.String(), "Summary:")
				return
			}
			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(dest, "docs", "report.txt"))
			require.NoError(t, err)
			require.Equal(t, "content", string(data))
			data, err = os.ReadFile(filepath.Join(dest, "docs", "report.txt.json"))
			require.NoError(t, err)
			var saved types.MetaRes
			require.NoError(t, json.Unmarshal(data, &saved))
			require.Equal(t, meta.ID, saved.ID)
			require.Contains(t, output.String(), "Downloaded 1 files")
		})
	}
}

func TestRemoveDirectoryRequiresRecursiveAndOneDelete(t *testing.T) {
	meta := types.MetaRes{Meta: vfs.Meta{ID: uuid.NewV4(), Name: "docs", IsDir: true}}
	deletes, trees := 0, 0
	h, _ := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes++
			assert.Equal(t, "/vfs/"+meta.ID.String(), r.URL.Path)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.URL.Query().Get("viewType") == "tree" {
			trees++
		}
		assert.NoError(t, json.NewEncoder(w).Encode(types.VfsRes[[]types.MetaRes]{Payload: []types.MetaRes{meta}}))
	})
	require.Error(t, h.Remove("/docs", false))
	require.Zero(t, deletes)
	require.NoError(t, h.Remove("/docs", true))
	require.Equal(t, 1, deletes)
	require.Zero(t, trees)
}

func TestFindMetaUsesCompleteName(t *testing.T) {
	meta := types.MetaRes{Meta: vfs.Meta{Name: "report.txt", Extension: "txt"}}
	h, _ := testHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		assert.NoError(t, json.NewEncoder(w).Encode(types.VfsRes[[]types.MetaRes]{Payload: []types.MetaRes{meta}}))
	})
	got, err := h.findMetaByPath("/report.txt")
	require.NoError(t, err)
	require.Equal(t, meta.Name, got.Name)
	_, err = h.findMetaByPath("/report.txttxt")
	require.Error(t, err)
}

func TestMkdirParentsDoesNotIgnoreFailedCreation(t *testing.T) {
	posts := 0
	h, _ := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusForbidden)
			return
		}
		assert.NoError(t, json.NewEncoder(w).Encode(types.VfsRes[[]types.MetaRes]{Payload: []types.MetaRes{}}))
	})
	require.Error(t, h.Mkdir("/parent/child", true))
	require.Equal(t, 1, posts)
}

func TestRecursiveUploadStopsWhenRootCreationFails(t *testing.T) {
	posts := 0
	h, _ := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusForbidden)
			return
		}
		assert.NoError(t, json.NewEncoder(w).Encode(types.VfsRes[[]types.MetaRes]{Payload: []types.MetaRes{}}))
	})
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("content"), 0o600))
	require.Error(t, h.Copy(src, "vfs:/target", true))
	require.Equal(t, 1, posts)
}
