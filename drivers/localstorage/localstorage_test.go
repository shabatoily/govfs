package localstorage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupVFS helper to create a fresh LocalStorage in a temp dir.
// Returns the ls instance and a cleanup function.
func setupVFS(t *testing.T) (*LocalStorage, func()) {
	dir, err := os.MkdirTemp("", "vfs-local-test-*")
	require.NoError(t, err)

	ls, err := New(&Config{Path: dir, Logger: vfs.DefaultLogger})
	require.NoError(t, err)

	return ls, func() {
		_ = ls.Close()
		_ = os.RemoveAll(dir)
	}
}

func Test_NewLocalStorage(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()
	assert.NotNil(t, ls)
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
			name:  "Create Single Directory",
			setup: nil,
			args:  args{path: "/test"},
			wantErr: func(t assert.TestingT, err error, i ...any) bool {
				return assert.NoError(t, err, i...)
			},
		},
		{
			name:  "Create Nested Directory",
			setup: nil,
			args:  args{path: "/test/a/b"},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				// LocalStorage implementation uses MkdirAll, so nested creation implies parents created automatically?
				// The implementation logic for Create/Mkdir uses os.MkdirAll.
				// However, VFS spec often requires strict parent existence or recursive flag.
				// Let's check implementation: `os.MkdirAll(localPath)` -> Success.
				return assert.NoError(t, err)
			},
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
				return uuid.Nil, nil
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
				return uuid.Nil
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
				return uuid.Nil
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

			err = ls2.Load(&buf, 0)
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
			expected: 20, // 10(prev) + 5(read) + 5(seek) = 20
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
	assert.Error(t, err)
	_, err = ls.StatByPath("/a/")
	assert.Error(t, err)

	// Verify /a/b is gone (recursive metadata cleanup)
	_, err = ls.Stat(metaB.ID)
	assert.Error(t, err, "Child directory metadata should be deleted")
	_, err = ls.StatByPath("/a/b/")
	assert.Error(t, err)

	// Verify /a/b/c.txt is gone
	_, err = ls.Stat(metaC.ID)
	assert.Error(t, err, "Grandchild file metadata should be deleted")
	_, err = ls.StatByPath("/a/b/c.txt")
	assert.Error(t, err)
}

func Test_RecursiveMove(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	// Create /a/b/c.txt
	ls.Mkdir("/a")
	ls.Mkdir("/a/b")
	ls.Create("/a/b/c.txt", bytes.NewBufferString("content"))

	metaA, err := ls.StatByPath("/a/")
	require.NoError(t, err)

	// Move /a -> /x
	_, err = ls.Move(metaA.ID, "/x")
	require.NoError(t, err)

	// Verify /a gone
	_, err = ls.StatByPath("/a/")
	assert.Error(t, err)

	// Verify /x exists
	_, err = ls.StatByPath("/x/")
	assert.NoError(t, err)

	// Verify /x/b exists (recursive update)
	metaB, err := ls.StatByPath("/x/b/")
	assert.NoError(t, err, "Child path should be updated")
	assert.Equal(t, "/x/b/", metaB.Path)

	// Verify /x/b/c.txt exists
	metaC, err := ls.StatByPath("/x/b/c.txt")
	assert.NoError(t, err, "Grandchild path should be updated")
	assert.Equal(t, "/x/b/c.txt", metaC.Path)

	// Content check
	f, err := ls.Open(metaC.ID)
	require.NoError(t, err)
	f.Close()
}

func Test_TreeStructure(t *testing.T) {
	ls, cleanup := setupVFS(t)
	defer cleanup()

	ls.Mkdir("/a")
	ls.Mkdir("/a/b")
	ls.Create("/a/b/c.txt", bytes.NewBuffer([]byte("c")))
	ls.Create("/a/d.txt", bytes.NewBuffer([]byte("d")))

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
		if name == "b" {
			nodeB = child
		} else if name == "d.txt" {
			nodeD = child
		}
	}
	require.NotNil(t, nodeB, "Should find node b")
	require.NotNil(t, nodeD, "Should find node d.txt")

	// "b" should have 1 child: "c.txt"
	require.Len(t, nodeB.Children, 1)
	require.Equal(t, "/a/b/c.txt", nodeB.Children[0].Meta.Path)
}
