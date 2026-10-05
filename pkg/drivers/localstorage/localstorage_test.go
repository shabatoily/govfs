package localstorage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupVFS helper to create a fresh LocalStorage in a temp dir.
// Returns the ls instance and a cleanup function.
func setupVFS(t *testing.T) (*LocalStorage, func()) {
	dir, err := os.MkdirTemp("", "vfs-local-test-*")
	require.NoError(t, err)

	ls, err := New(&Config{Path: dir, Logger: log.Default()})
	require.NoError(t, err)

	return ls, func() {
		_ = ls.Close()
		_ = os.RemoveAll(dir)
	}
}

func TestDirectoryQueriesKeepSubtreeBoundary(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()
	for _, path := range []string{"/a", "/a/nested", "/ab"} {
		_, err := ls.Mkdir(path)
		require.NoError(t, err)
	}
	for _, path := range []string{"/a/first.txt", "/a/nested/second.txt", "/ab/other.txt"} {
		_, err := ls.Create(path, bytes.NewBufferString("content"))
		require.NoError(t, err)
	}
	for _, path := range []string{"/a", "/a/"} {
		list, err := ls.List(path)
		require.NoError(t, err)
		require.Len(t, list, 2)
		for _, meta := range list {
			assert.Contains(t, []string{"/a/first.txt", "/a/nested/"}, meta.Path)
		}
		tree, err := ls.Tree(path)
		require.NoError(t, err)
		require.Len(t, tree.Children, 2)
		for _, child := range tree.Children {
			if child.Meta.IsDir {
				require.Len(t, child.Children, 1)
				assert.Equal(t, "/a/nested/second.txt", child.Children[0].Meta.Path)
			} else {
				assert.Equal(t, "/a/first.txt", child.Meta.Path)
			}
		}
	}
}

func Test_NewLocalStorage(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()
	assert.NotNil(t, ls)
}

func TestNewRejectsUnreadableIndex(t *testing.T) {
	dir := t.TempDir()
	indexFile := filepath.Join(dir, IndexFileName)
	require.NoError(t, os.Symlink(IndexFileName, indexFile))
	_, readErr := os.ReadFile(indexFile)
	require.Error(t, readErr)
	ls, err := New(&Config{Path: dir})
	require.Error(t, err)
	assert.Nil(t, ls)
	var pathErr *os.PathError
	require.ErrorAs(t, readErr, &pathErr)
	assert.ErrorIs(t, err, pathErr.Err)
}

func TestNewRestoresSavedIndex(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(&Config{Path: dir})
	require.NoError(t, err)
	meta, err := ls.Create("/file.txt", bytes.NewBufferString("content"))
	require.NoError(t, err)
	require.NoError(t, ls.Close())
	reopened, err := New(&Config{Path: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.StatByPath("/file.txt")
	require.NoError(t, err)
	assert.Equal(t, meta.ID, restored.ID)
}

func TestCloseReplacesIndexWithoutFollowingSymlink(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(&Config{Path: dir})
	require.NoError(t, err)
	meta, err := ls.Mkdir("/dir")
	require.NoError(t, err)
	previous := filepath.Join(dir, "previous.json")
	require.NoError(t, os.WriteFile(previous, []byte("[]"), vfs.DefaultFileMode))
	require.NoError(t, os.Symlink(previous, filepath.Join(dir, IndexFileName)))
	require.NoError(t, ls.Close())
	data, err := os.ReadFile(previous)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data))
	reopened, err := New(&Config{Path: dir})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.StatByPath("/dir/")
	require.NoError(t, err)
	assert.Equal(t, meta.ID, restored.ID)
}

func TestCloseCleansTemporaryIndexOnRenameError(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(&Config{Path: dir})
	require.NoError(t, err)
	indexDir := filepath.Join(dir, IndexFileName)
	require.NoError(t, os.Mkdir(indexDir, vfs.DefaultDirMode))
	previous := filepath.Join(indexDir, "previous")
	require.NoError(t, os.WriteFile(previous, []byte("keep"), vfs.DefaultFileMode))
	require.Error(t, ls.Close())
	data, err := os.ReadFile(previous)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data))
	temporary, err := filepath.Glob(filepath.Join(dir, ".vfs_index-*"))
	require.NoError(t, err)
	assert.Empty(t, temporary)
}

func Test_LocalStorage_Mkdir(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage)
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "Create Single Directory",
			setup:   nil,
			args:    args{path: "/test"},
			wantErr: assert.NoError,
		},
		{
			name:    "Create Nested Directory",
			setup:   nil,
			args:    args{path: "/test/a/b"},
			wantErr: assert.NoError,
		},
		{
			name: "Create Existing Directory",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
			},
			args: args{path: "/test"},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.Equal(t, err, vfs.ErrAlreadyExists)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(ls)
			}

			_, err := ls.Mkdir(tt.args.path)
			tt.wantErr(t, err, fmt.Sprintf("Mkdir(%v)", tt.args.path))
		})
	}
}

func Test_LocalStorage_Create(t *testing.T) {
	type args struct {
		path    string
		content *bytes.Buffer
	}
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage)
		args    args
		want    *vfs.Meta
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Create File",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
			},
			args: args{
				path:    "/test/file.txt",
				content: bytes.NewBufferString("content"),
			},
			want: &vfs.Meta{
				Path: "/test/file.txt",
				Size: 7,
			},
			wantErr: assert.NoError,
		},
		{
			name:  "Create File (Auto-parent)",
			setup: nil, // Parent dir doesn't exist, but LocalStorage uses MkdirAll
			args: args{
				path:    "/test/file.txt",
				content: bytes.NewBufferString("content"),
			},
			want: &vfs.Meta{
				Path: "/test/file.txt",
				Size: 7,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(ls)
			}

			got, err := ls.Create(tt.args.path, tt.args.content)
			if !tt.wantErr(t, err, fmt.Sprintf("Create(%v)", tt.args.path)) {
				return
			}
			if err == nil {
				assert.Equal(t, tt.want.Path, got.Path)
				assert.Equal(t, tt.want.Size, got.Size)
			}
		})
	}
}

func Test_LocalStorage_List(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name      string
		setup     func(ls *LocalStorage)
		args      args
		wantCount int
		wantErr   assert.ErrorAssertionFunc
	}{
		{
			name:      "List Root Empty",
			setup:     nil,
			args:      args{path: "/"},
			wantCount: 0,
			wantErr:   assert.NoError,
		},
		{
			name: "List Root With Items",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
			},
			args:      args{path: "/"},
			wantCount: 1,
			wantErr:   assert.NoError,
		},
		{
			name: "List Subdirectory",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
				_, _ = ls.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			args:      args{path: "/test"},
			wantCount: 1,
			wantErr:   assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(ls)
			}

			got, err := ls.List(tt.args.path)
			if !tt.wantErr(t, err, fmt.Sprintf("List(%v)", tt.args.path)) {
				return
			}
			if err == nil {
				assert.Len(t, got, tt.wantCount)
			}
		})
	}
}

func Test_LocalStorage_Read(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage) (uuid.UUID, []byte)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Read Existing File",
			setup: func(ls *LocalStorage) (uuid.UUID, []byte) {
				_, _ = ls.Mkdir("/test")
				content := bytes.NewBufferString("content")
				m, _ := ls.Create("/test/file.txt", content)
				return m.ID, []byte("content")
			},
			wantErr: assert.NoError,
		},
		{
			name: "Read Non-existent File",
			setup: func(_ *LocalStorage) (uuid.UUID, []byte) {
				return uuid.Nil(), nil
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			id, wantContent := tt.setup(ls)

			got, err := ls.Open(id)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				actual := make([]byte, got.Meta.Size)
				_, err := got.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, wantContent, actual)
				got.Close()
			}
		})
	}
}

func Test_LocalStorage_Write(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(ls *LocalStorage) uuid.UUID
		content  *bytes.Buffer
		expected []byte
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name: "Write Existing File",
			setup: func(ls *LocalStorage) uuid.UUID {
				_, _ = ls.Mkdir("/test")
				m, _ := ls.Create("/test/file.txt", bytes.NewBufferString("old"))
				return m.ID
			},
			content:  bytes.NewBufferString("new"),
			expected: []byte("new"),
			wantErr:  assert.NoError,
		},
		{
			name: "Write Non-existent File",
			setup: func(_ *LocalStorage) uuid.UUID {
				return uuid.Nil()
			},
			content:  bytes.NewBufferString("new"),
			expected: []byte("new"),
			wantErr:  assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			id := tt.setup(ls)

			_, err := ls.Write(id, tt.content)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, _ := ls.Open(id)
				defer f.Close()
				actual := make([]byte, f.Meta.Size)
				_, err := f.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, actual)
			}
		})
	}
}

func Test_LocalStorage_Delete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage) uuid.UUID
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Delete Existing File",
			setup: func(ls *LocalStorage) uuid.UUID {
				_, _ = ls.Mkdir("/test")
				m, _ := ls.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			wantErr: assert.NoError,
		},
		{
			name: "Delete Non-existent File",
			setup: func(_ *LocalStorage) uuid.UUID {
				return uuid.Nil()
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			id := tt.setup(ls)

			err := ls.Delete(id)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				_, err := ls.Open(id)
				assert.Error(t, err)
			}
		})
	}
}

func Test_LocalStorage_Copy(t *testing.T) {
	type args struct {
		dst string
	}
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage) uuid.UUID
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Copy File",
			setup: func(ls *LocalStorage) uuid.UUID {
				_, _ = ls.Mkdir("/test")
				m, _ := ls.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			args:    args{dst: "test/file.copy.txt"},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			id := tt.setup(ls)

			copied, err := ls.Copy(id, tt.args.dst)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, err := ls.Open(copied.ID)
				require.NoError(t, err)
				defer f.Close()
				actual := make([]byte, f.Meta.Size)
				_, err = f.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, "content", string(actual))
			}
		})
	}
}

func Test_LocalStorage_Move(t *testing.T) {
	type args struct {
		dst string
	}
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage) uuid.UUID
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Move File",
			setup: func(ls *LocalStorage) uuid.UUID {
				_, _ = ls.Mkdir("/test")
				m, _ := ls.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			args:    args{dst: "test/file.mv.txt"},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			id := tt.setup(ls)

			moved, err := ls.Move(id, tt.args.dst)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, err := ls.Open(moved.ID)
				require.NoError(t, err)
				defer f.Close()
				assert.Equal(t, "/test/file.mv.txt", f.Meta.Path)
			}
		})
	}
}

func Test_LocalStorage_Tree(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage)
		path    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Tree Root",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
				_, _ = ls.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			path:    "/",
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(ls)
			}

			_, err := ls.Tree(tt.path)
			tt.wantErr(t, err)
		})
	}
}

type failAfterHeaderWriter struct{ bytes.Buffer }

func (w *failAfterHeaderWriter) Write(p []byte) (int, error) {
	if w.Len() != 0 {
		return 0, io.ErrClosedPipe
	}
	return w.Buffer.Write(p)
}

func TestBackupIncludesFinalWrites(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()
	var buf bytes.Buffer
	size, err := ls.Backup(&buf, 0)
	require.NoError(t, err)
	assert.Equal(t, uint64(buf.Len()), size)
	w := &failAfterHeaderWriter{}
	size, err = ls.Backup(w, 0)
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Equal(t, uint64(w.Len()), size)
}

func TestBackupRejectsMissingIndexedFile(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()
	meta, err := ls.Create("/missing.txt", bytes.NewBufferString("content"))
	require.NoError(t, err)
	require.NoError(t, os.Remove(ls.toLocalPath(meta.Path)))
	var buf bytes.Buffer
	_, err = ls.Backup(&buf, 0)
	require.ErrorIs(t, err, os.ErrNotExist)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, ls.toLocalPath(meta.Path), pathErr.Path)
	_, err = ls.StatByPath(meta.Path)
	require.NoError(t, err)
}

func TestLoadPreservesDataOnInvalidArchive(t *testing.T) {
	parent := t.TempDir()
	ls, err := New(&Config{Path: filepath.Join(parent, "storage")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ls.Close() })
	meta, err := ls.Create("/original.txt", bytes.NewBufferString("keep"))
	require.NoError(t, err)
	require.NoError(t, ls.Close())
	indexPath := filepath.Join(ls.basePath, IndexFileName)
	index, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	var valid bytes.Buffer
	_, err = ls.Backup(&valid, 0)
	require.NoError(t, err)
	corrupt := bytes.Clone(valid.Bytes())
	corrupt[len(corrupt)-8] ^= 1
	var invalidIndex bytes.Buffer
	gw := gzip.NewWriter(&invalidIndex)
	tw := tar.NewWriter(gw)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: IndexFileName, Mode: 0o644, Size: 1}))
	_, err = tw.Write([]byte("{"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	archives := [][]byte{[]byte("invalid"), valid.Bytes()[:valid.Len()-4], corrupt, invalidIndex.Bytes()}
	for _, includeFile := range []bool{false, true} {
		var incomplete bytes.Buffer
		gw = gzip.NewWriter(&incomplete)
		tw = tar.NewWriter(gw)
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: IndexFileName, Mode: 0o644, Size: int64(len(index))}))
		_, err = tw.Write(index)
		require.NoError(t, err)
		if includeFile {
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: "original.txt", Mode: 0o644, Size: 1}))
			_, err = tw.Write([]byte("k"))
			require.NoError(t, err)
		}
		require.NoError(t, tw.Close())
		require.NoError(t, gw.Close())
		archives = append(archives, incomplete.Bytes())
	}
	for _, archive := range archives {
		require.Error(t, ls.Load(bytes.NewReader(archive), 0))
		opened, err := ls.Open(meta.ID)
		require.NoError(t, err)
		content, err := io.ReadAll(opened)
		require.NoError(t, errors.Join(err, opened.Close()))
		assert.Equal(t, "keep", string(content))
		data, err := os.ReadFile(indexPath)
		require.NoError(t, err)
		assert.Equal(t, index, data)
	}
	staged, err := filepath.Glob(filepath.Join(parent, ".vfs-restore-*"))
	require.NoError(t, err)
	assert.Empty(t, staged)
}

func Test_LocalStorage_Backup_And_Load(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(ls *LocalStorage)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Backup and Load",
			setup: func(ls *LocalStorage) {
				_, _ = ls.Mkdir("/test")
				_, _ = ls.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(ls)
			}

			// Capture Backup (tar.gz)
			var buf bytes.Buffer
			_, err := ls.Backup(&buf, 0)
			require.NoError(t, err)

			// Create NEW LocalStorage instance (clean)
			ls2, cleanup2 := setupVFS(t)
			defer cleanup2()
			old, err := ls2.Create("/old.txt", bytes.NewBufferString("old"))
			require.NoError(t, err)
			require.NoError(t, ls2.Close())

			err = ls2.Load(&buf, 0)
			require.NoError(t, err)
			_, err = ls2.Open(old.ID)
			require.ErrorIs(t, err, vfs.ErrNotFound)
			_, err = os.Stat(filepath.Join(ls2.basePath, "old.txt"))
			require.ErrorIs(t, err, os.ErrNotExist)
			reopened, err := New(&Config{Path: ls2.basePath})
			require.NoError(t, err)
			_, err = reopened.StatByPath("/test/file.txt")
			require.NoError(t, err)

			// Verify
			stat, err := ls2.StatByPath("/test/file.txt")
			require.NoError(t, err)
			assert.Equal(t, "/test/file.txt", stat.Path)

			// Verify content
			f, err := ls2.Open(stat.ID)
			require.NoError(t, err)
			defer f.Close()

			content, err := io.ReadAll(f)
			require.NoError(t, err)
			assert.Equal(t, "content", string(content))
		})
	}
}

func Test_LocalStorage_Seek(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	_, _ = ls.Mkdir("/test")
	content := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	m, err := ls.Create("/test/seek.txt", bytes.NewReader(content))
	require.NoError(t, err)

	f, err := ls.Open(m.ID)
	require.NoError(t, err)
	defer f.Close()

	tests := []struct {
		name     string
		offset   int64
		whence   int
		expected int64
		readLen  int
		want     string
		wantErr  bool
	}{
		{
			name:     "Seek Start",
			offset:   10,
			whence:   io.SeekStart,
			expected: 10,
			readLen:  5,
			want:     "ABCDE",
			wantErr:  false,
		},
		{
			name:     "Seek Current",
			offset:   5,
			whence:   io.SeekCurrent,
			expected: 20,
			readLen:  1,
			want:     "K",
			wantErr:  false,
		},
		{
			name:     "Seek End",
			offset:   -1,
			whence:   io.SeekEnd,
			expected: int64(len(content) - 1),
			readLen:  1,
			want:     "Z",
			wantErr:  false,
		},
		{
			name:     "Seek Past End",
			offset:   10,
			whence:   io.SeekEnd,
			expected: int64(len(content)) + 10,
			readLen:  0,
			want:     "",
			wantErr:  false,
		},
		{
			// os.File typically returns error on negative seek, but behavior can vary by impl.
			// Let's verify standard os.File behavior.
			name:     "Seek Negative",
			offset:   -1,
			whence:   io.SeekStart,
			expected: 0,
			readLen:  0,
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, err := f.Seek(tt.offset, tt.whence)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, pos)

			if tt.readLen > 0 {
				buf := make([]byte, tt.readLen)
				n, err := f.Read(buf)
				require.NoError(t, err)
				assert.Equal(t, tt.readLen, n)
				assert.Equal(t, tt.want, string(buf))
			} else if tt.readLen == 0 && tt.expected > int64(len(content)) {
				// Checking EOF if read attempt is made
				buf := make([]byte, 1)
				_, err := f.Read(buf)
				assert.Equal(t, io.EOF, err)
			}
		})
	}
}

func Test_RecursiveDelete(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	// Create dir structure: /a/b/c.txt
	_, err := ls.Mkdir("/a")
	require.NoError(t, err)
	_, err = ls.Mkdir("/a/b")
	require.NoError(t, err)
	_, err = ls.Create("/a/b/c.txt", bytes.NewBufferString("content"))
	require.NoError(t, err)

	// Get IDs
	metaA, err := ls.StatByPath("/a/")
	require.NoError(t, err)
	metaB, err := ls.StatByPath("/a/b/")
	require.NoError(t, err)
	metaC, err := ls.StatByPath("/a/b/c.txt")
	require.NoError(t, err)

	// Delete /a
	err = ls.Delete(metaA.ID)
	require.NoError(t, err)

	// Verify /a is gone
	_, err = ls.Stat(metaA.ID)
	require.Error(t, err)
	_, err = ls.StatByPath("/a/")
	require.Error(t, err)

	// Verify /a/b is gone (recursive metadata cleanup)
	_, err = ls.Stat(metaB.ID)
	require.Error(t, err, "Child directory metadata should be deleted")
	_, err = ls.StatByPath("/a/b/")
	require.Error(t, err)

	// Verify /a/b/c.txt is gone
	_, err = ls.Stat(metaC.ID)
	require.Error(t, err, "Grandchild file metadata should be deleted")
	_, err = ls.StatByPath("/a/b/c.txt")
	require.Error(t, err)
}

func Test_RecursiveMove(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	// Create /a/b/c.txt
	_, err := ls.Mkdir("/a")
	require.NoError(t, err)

	_, err = ls.Mkdir("/a/b")
	require.NoError(t, err)

	_, err = ls.Create("/a/b/c.txt", bytes.NewBufferString("content"))
	require.NoError(t, err)

	metaA, err := ls.StatByPath("/a/")
	require.NoError(t, err)

	// Move /a -> /x
	_, err = ls.Move(metaA.ID, "/x")
	require.NoError(t, err)

	// Verify /a gone
	_, err = ls.StatByPath("/a/")
	require.Error(t, err)

	// Verify /x exists
	_, err = ls.StatByPath("/x/")
	require.NoError(t, err)

	// Verify /x/b exists (recursive update)
	metaB, err := ls.StatByPath("/x/b/")
	require.NoError(t, err, "Child path should be updated")
	require.Equal(t, "/x/b/", metaB.Path)

	// Verify /x/b/c.txt exists
	metaC, err := ls.StatByPath("/x/b/c.txt")
	require.NoError(t, err, "Grandchild path should be updated")
	require.Equal(t, "/x/b/c.txt", metaC.Path)

	// Content check
	f, err := ls.Open(metaC.ID)
	require.NoError(t, err)
	f.Close()
}

func Test_TreeStructure(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	_, err := ls.Mkdir("/a")
	require.NoError(t, err)

	_, err = ls.Mkdir("/a/b")
	require.NoError(t, err)

	_, err = ls.Create("/a/b/c.txt", bytes.NewBuffer([]byte("c")))
	require.NoError(t, err)

	_, err = ls.Create("/a/d.txt", bytes.NewBuffer([]byte("d")))
	require.NoError(t, err)

	// Tree("/")
	root, err := ls.Tree("/")
	require.NoError(t, err)

	require.NotNil(t, root)
	require.Equal(t, "/", root.Meta.Path)

	// Root should have 1 child: "a"
	require.Len(t, root.Children, 1)
	nodeA := root.Children[0]
	// Directory path in metadata has trailing slash
	require.Equal(t, "/a/", nodeA.Meta.Path)

	// "a" should have 2 children: "b", "d.txt"
	require.Len(t, nodeA.Children, 2)

	// Find b and d
	var nodeB, nodeD *vfs.TreeNode
	for _, child := range nodeA.Children {
		name := filepath.Base(child.Meta.Path)
		switch name {
		case "b":
			nodeB = child
		case "d.txt":
			nodeD = child
		}
	}
	require.NotNil(t, nodeB, "Should find node b")
	require.NotNil(t, nodeD, "Should find node d.txt")

	// "b" should have 1 child: "c.txt"
	require.Len(t, nodeB.Children, 1)
	require.Equal(t, "/a/b/c.txt", nodeB.Children[0].Meta.Path)
}

func Test_LocalStoragePersistsUpdatedMetadata(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	meta, err := ls.Create("file.txt", bytes.NewBufferString("old"))
	require.NoError(t, err)
	_, err = ls.Write(meta.ID, bytes.NewBufferString("updated"))
	require.NoError(t, err)
	_, err = ls.WriteComments(meta.ID, "comment")
	require.NoError(t, err)

	got, err := ls.Stat(meta.ID)
	require.NoError(t, err)
	assert.Equal(t, "/file.txt", got.Path)
	assert.Equal(t, int64(len("updated")), got.Size)
	assert.Equal(t, "comment", got.Comments)
}

func TestTransferFailurePreservesReplacement(t *testing.T) {
	for _, copyItem := range []bool{false, true} {
		name := "move"
		if copyItem {
			name = "copy"
		}
		t.Run(name, func(t *testing.T) {
			ls, cleanup := setupVFS(t)
			defer cleanup()
			src, err := ls.Create("/source", bytes.NewBufferString("source"))
			require.NoError(t, err)
			dst, err := ls.Create("/target", bytes.NewBufferString("target"))
			require.NoError(t, err)
			// 원본 파일을 제거해 복사 실패와 이동 실패 시 대상 복구를 확인합니다.
			require.NoError(t, os.Remove(ls.toLocalPath(src.Path)))
			if copyItem {
				_, err = ls.Copy(src.ID, dst.Path, dst.ID)
			} else {
				_, err = ls.Move(src.ID, dst.Path, dst.ID)
			}
			require.Error(t, err)
			file, err := ls.Open(dst.ID)
			require.NoError(t, err)
			defer file.Close()
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			require.Equal(t, "target", string(data))
		})
	}
}
