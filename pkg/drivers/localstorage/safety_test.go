package localstorage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/stretchr/testify/require"
)

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { return copy(p, "partial"), errors.New("read failed") }

func TestStorageBoundaryAndAtomicWrite(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "drive")
	ls, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer ls.Close()
	_, err = ls.Create("/../outside", strings.NewReader("escape"))
	require.Error(t, err)
	_, err = ls.Mkdir("/../outside")
	require.Error(t, err)
	_, err = ls.Create("/"+IndexFileName, strings.NewReader("[]"))
	require.Error(t, err)
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link")))
	_, err = ls.Create("/link/file", strings.NewReader("escape"))
	require.Error(t, err)
	meta, err := ls.Create("/a.txt", strings.NewReader("original"))
	require.NoError(t, err)
	_, err = ls.Create("/./a.txt", strings.NewReader("overwrite"))
	require.ErrorIs(t, err, vfs.ErrAlreadyExists)
	_, err = ls.Write(meta.ID, brokenReader{})
	require.Error(t, err)
	file, err := ls.Open(meta.ID)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}

func TestMutationSurvivesReopenWithoutClose(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer ls.Close()
	_, err = ls.Create("/missing/file", bytes.NewBufferString("no"))
	require.Error(t, err)
	meta, err := ls.Create("/file", bytes.NewBufferString("yes"))
	require.NoError(t, err)
	reopened, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer reopened.Close()
	got, err := reopened.Stat(meta.ID)
	require.NoError(t, err)
	require.Equal(t, meta.ID, got.ID)
	require.Equal(t, meta.Size, got.Size)
	require.True(t, meta.Modified.Equal(got.Modified))
}

func TestRecoveryBeforeAndAfterIndexCommit(t *testing.T) {
	for _, steps := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(steps), func(t *testing.T) {
			dir := t.TempDir()
			ls, err := New(&Config{Path: dir})
			require.NoError(t, err)
			meta, err := ls.Create("/file", strings.NewReader("old"))
			require.NoError(t, err)
			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Mkdir(transactionDir, 0o700))
			require.NoError(t, root.WriteFile(transactionDir+"/new", []byte("new data"), 0o600))
			next := meta
			next.Size = 8
			index, err := json.Marshal([]vfs.Meta{next})
			require.NoError(t, err)
			require.NoError(t, writeSynced(root, transactionDir+"/index", index))
			moves := []diskMove{{"file", transactionDir + "/old"}, {transactionDir + "/new", "file"}}
			journal, err := json.Marshal(moves)
			require.NoError(t, err)
			require.NoError(t, writeSynced(root, transactionDir+"/journal", journal))
			for i := range min(steps, 2) {
				require.NoError(t, root.Rename(moves[i].From, moves[i].To))
			}
			if steps == 3 {
				require.NoError(t, root.Rename(transactionDir+"/index", IndexFileName))
			}
			reopened, err := New(&Config{Path: dir})
			require.NoError(t, err)
			defer reopened.Close()
			file, err := reopened.Open(meta.ID)
			require.NoError(t, err)
			defer file.Close()
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			want := "old"
			if steps == 3 {
				want = "new data"
			}
			require.Equal(t, want, string(data))
			require.Equal(t, int64(len(want)), file.Meta.Size)
		})
	}
}

func TestRestoreRejectsUnsafeDirectoryAndDuplicateMetadata(t *testing.T) {
	ls, err := New(&Config{Path: t.TempDir()})
	require.NoError(t, err)
	defer ls.Close()
	meta, err := ls.Create("/keep", strings.NewReader("keep"))
	require.NoError(t, err)
	for _, metas := range [][]vfs.Meta{
		{{ID: uuid.NewV4(), Path: "/../victim/", Name: "victim", IsDir: true}},
		{meta, meta},
	} {
		var data bytes.Buffer
		gz := gzip.NewWriter(&data)
		tw := tar.NewWriter(gz)
		index, err := json.Marshal(metas)
		require.NoError(t, err)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: IndexFileName, Mode: 0o600, Size: int64(len(index))}))
		_, err = tw.Write(index)
		require.NoError(t, err)
		require.NoError(t, tw.Close())
		require.NoError(t, gz.Close())
		require.Error(t, ls.Load(&data, 0))
		_, err = ls.Stat(meta.ID)
		require.NoError(t, err)
	}
}

func TestRecoveryProcessExit(t *testing.T) {
	if dir := os.Getenv("GOVFS_RECOVERY_TEST_DIRECTORY"); dir != "" {
		root, err := os.OpenRoot(dir)
		require.NoError(t, err)
		data, err := root.ReadFile(IndexFileName)
		require.NoError(t, err)
		var metas []vfs.Meta
		require.NoError(t, json.Unmarshal(data, &metas))
		metas[0].Size = 8
		next, err := json.Marshal(metas)
		require.NoError(t, err)
		require.NoError(t, root.Mkdir(transactionDir, privateDirectoryMode))
		require.NoError(t, writeSynced(root, transactionDir+"/new", []byte("new data")))
		require.NoError(t, writeSynced(root, transactionDir+"/index", next))
		moves := []diskMove{{"file", transactionDir + "/old"}, {transactionDir + "/new", "file"}}
		journal, err := json.Marshal(moves)
		require.NoError(t, err)
		require.NoError(t, writeSynced(root, transactionDir+"/journal", journal))
		for _, move := range moves {
			require.NoError(t, root.Rename(move.From, move.To))
		}
		if os.Getenv("GOVFS_RECOVERY_TEST_COMMITTED") == "true" {
			require.NoError(t, root.Rename(transactionDir+"/index", IndexFileName))
		}
		os.Exit(0)
	}
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			dir := t.TempDir()
			ls, err := New(&Config{Path: dir})
			require.NoError(t, err)
			meta, err := ls.Create("/file", strings.NewReader("old"))
			require.NoError(t, err)
			executable, err := os.Executable()
			require.NoError(t, err)
			child := exec.CommandContext(context.Background(), executable, "-test.run=^TestRecoveryProcessExit$")
			child.Env = append(os.Environ(), "GOVFS_RECOVERY_TEST_DIRECTORY="+dir, fmt.Sprintf("GOVFS_RECOVERY_TEST_COMMITTED=%t", committed))
			output, err := child.CombinedOutput()
			require.NoError(t, err, string(output))
			reopened, err := New(&Config{Path: dir})
			require.NoError(t, err)
			defer reopened.Close()
			file, err := reopened.Open(meta.ID)
			require.NoError(t, err)
			defer file.Close()
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			want := "old"
			if committed {
				want = "new data"
			}
			require.Equal(t, want, string(data))
		})
	}
}
