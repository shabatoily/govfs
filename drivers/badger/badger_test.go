package badger

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logger, _ = vfs.NewLogger(vfs.LoggerConfig{})

func Test_NewBadgerVFS(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Initialize VFS",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()
			require.NotNil(t, v)
		})
	}
}

func Test_BadgerVFS_Mkdir(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS)
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
			name:  "Create Nested Directory",
			setup: nil,
			args:  args{path: "/test/a/b"},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.Equal(t, err, vfs.ErrNotFound)
			},
		},
		{
			name: "Create Existing Directory",
			setup: func(v *BadgerVFS) {
				_, _ = v.Mkdir("/test")
			},
			args: args{path: "/test"},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.Equal(t, err, vfs.ErrAlreadyExists)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			_, err = v.Mkdir(tt.args.path)
			tt.wantErr(t, err, fmt.Sprintf("Mkdir(%v)", tt.args.path))
		})
	}
}

func Test_BadgerVFS_Create(t *testing.T) {
	type args struct {
		path    string
		content *bytes.Buffer
	}
	tests := []struct {
		name    string
		setup   func(vfs *BadgerVFS)
		args    args
		want    *vfs.Meta
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Create File",
			setup: func(v *BadgerVFS) {
				_, _ = v.Mkdir("/test")
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
			name: "Create File in Non-existent Directory",
			setup: func(v *BadgerVFS) {
				_, err := v.List("/test")
				assert.Error(t, err)
			},
			args: args{
				path:    "/test/file.txt",
				content: bytes.NewBufferString("content"),
			},
			want:    nil,
			wantErr: assert.Error, // Assuming parent dir must exist
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			got, err := v.Create(tt.args.path, tt.args.content)
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

func Test_BadgerVFS_List(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name      string
		setup     func(v *BadgerVFS)
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
			setup: func(v *BadgerVFS) {
				_, _ = v.Mkdir("/test")
			},
			args:      args{path: "/"},
			wantCount: 1,
			wantErr:   assert.NoError,
		},
		{
			name: "List Subdirectory",
			setup: func(v *BadgerVFS) {
				_, _ = v.Mkdir("/test")
				_, _ = v.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			args:      args{path: "/test"},
			wantCount: 1,
			wantErr:   assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			got, err := v.List(tt.args.path)
			if !tt.wantErr(t, err, fmt.Sprintf("List(%v)", tt.args.path)) {
				return
			}
			if err == nil {
				assert.Len(t, got, tt.wantCount)
			}
		})
	}
}

func Test_BadgerVFS_Read(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS) (uuid.UUID, []byte)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Read Existing File",
			setup: func(v *BadgerVFS) (uuid.UUID, []byte) {
				_, _ = v.Mkdir("/test")
				content := bytes.NewBufferString("content")
				m, _ := v.Create("/test/file.txt", content)
				return m.ID, []byte("content")
			},
			wantErr: assert.NoError,
		},
		{
			name: "Read Non-existent File",
			setup: func(_ *BadgerVFS) (uuid.UUID, []byte) {
				return uuid.Nil, nil
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id, wantContent := tt.setup(v)

			got, err := v.Open(id)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				actual := make([]byte, got.Meta.Size)
				_, err := got.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, wantContent, actual)
			}
		})
	}
}

func Test_BadgerVFS_Write(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(v *BadgerVFS) uuid.UUID
		content  *bytes.Buffer
		expected []byte
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name: "Write Existing File",
			setup: func(v *BadgerVFS) uuid.UUID {
				_, _ = v.Mkdir("/test")
				m, _ := v.Create("/test/file.txt", bytes.NewBufferString("old"))
				return m.ID
			},
			content:  bytes.NewBufferString("new"),
			expected: []byte("new"),
			wantErr:  assert.NoError,
		},
		{
			name: "Write Non-existent File",
			setup: func(_ *BadgerVFS) uuid.UUID {
				return uuid.Nil
			},
			content:  bytes.NewBufferString("new"),
			expected: []byte("new"),
			wantErr:  assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id := tt.setup(v)

			_, err = v.Write(id, tt.content)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, _ := v.Open(id)
				actual := make([]byte, f.Meta.Size)
				_, err := f.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, actual)
			}
		})
	}
}

func Test_BadgerVFS_Delete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS) uuid.UUID
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Delete Existing File",
			setup: func(v *BadgerVFS) uuid.UUID {
				_, _ = v.Mkdir("/test")
				m, _ := v.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			wantErr: assert.NoError,
		},
		{
			name: "Delete Non-existent File",
			setup: func(_ *BadgerVFS) uuid.UUID {
				return uuid.Nil
			},
			wantErr: assert.Error, // Assuming delete returns error if not found
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id := tt.setup(v)

			err = v.Delete(id)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				_, err := v.Open(id)
				assert.Error(t, err)
			}
		})
	}
}

func Test_BadgerVFS_Copy(t *testing.T) {
	type args struct {
		dst string
	}
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS) uuid.UUID
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Copy File",
			setup: func(v *BadgerVFS) uuid.UUID {
				_, _ = v.Mkdir("/test")
				m, _ := v.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			args:    args{dst: "test/file.copy.txt"},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id := tt.setup(v)

			copied, err := v.Copy(id, tt.args.dst)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, err := v.Open(copied.ID)
				require.NoError(t, err)
				actual := make([]byte, f.Meta.Size)
				_, err = f.Read(actual)
				require.NoError(t, err)
				assert.Equal(t, "content", string(actual))
			}
		})
	}
}

func Test_BadgerVFS_Move(t *testing.T) {
	type args struct {
		dst string
	}
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS) uuid.UUID
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Move File",
			setup: func(v *BadgerVFS) uuid.UUID {
				_, _ = v.Mkdir("/test")
				m, _ := v.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			args:    args{dst: "test/file.mv.txt"},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id := tt.setup(v)

			moved, err := v.Move(id, tt.args.dst)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, err := v.Open(moved.ID)
				require.NoError(t, err)
				assert.Equal(t, "/test/file.mv.txt", f.Meta.Path)
			}
		})
	}
}

func Test_BadgerVFS_Tree(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(vfs *BadgerVFS)
		path    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Tree Root",
			setup: func(vfs *BadgerVFS) {
				_, _ = vfs.Mkdir("/test")
				_, _ = vfs.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			path:    "/",
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			tree, err := v.Tree(tt.path)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				jsonBytes, err := json.Marshal(tree)
				require.NoError(t, err)
				t.Log(string(jsonBytes))
			}
		})
	}
}

func Test_BadgerVFS_WriteComments(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(vfs *BadgerVFS) uuid.UUID
		comment string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Write Comment",
			setup: func(v *BadgerVFS) uuid.UUID {
				_, _ = v.Mkdir("/test")
				m, _ := v.Create("/test/file.txt", bytes.NewBufferString("content"))
				return m.ID
			},
			comment: "test comment",
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			id := tt.setup(v)

			_, err = v.WriteComments(id, tt.comment)
			if !tt.wantErr(t, err) {
				return
			}
			if err == nil {
				f, _ := v.Open(id)
				assert.Equal(t, tt.comment, f.Meta.Comments)
			}
		})
	}
}

func Test_BadgerVFS_Backup_And_Load(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(v *BadgerVFS)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Backup and Load",
			setup: func(v *BadgerVFS) {
				_, _ = v.Mkdir("/test")
				_, _ = v.Create("/test/file.txt", bytes.NewBufferString("content"))
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			var backupData []byte
			buf := bytes.NewBuffer(backupData)

			f, err := os.Create("data.bak")
			require.NoError(t, err)
			defer func(f *os.File) {
				_ = os.Remove(f.Name())
			}(f)

			_, err = v.Backup(buf, 0)
			if !tt.wantErr(t, err) {
				return
			}

			// Test Load
			cfg2 := Config{
				InMemory: true,
				Logger:   logger,
			}
			newVFS, err := New(cfg2)
			require.NoError(t, err)
			defer newVFS.Close()

			err = newVFS.Load(buf, 256)
			require.NoError(t, err)

			stat, err := newVFS.StatByPath("/test/file.txt")
			require.NoError(t, err)
			assert.Equal(t, "/test/file.txt", stat.Path)
		})
	}
}

func Test_BadgerVFS_StatByPath(t *testing.T) {
	type args struct {
		p string
	}
	tests := []struct {
		name     string
		setup    func(vfs *BadgerVFS)
		args     args
		wantPath string
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name:     "Root Path",
			setup:    nil,
			args:     args{p: "/"},
			wantPath: "/",
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.Equal(t, err, vfs.ErrInvalidPath)
			},
		},
		{
			name: "Existing Directory",
			setup: func(vfs *BadgerVFS) {
				_, err := vfs.Mkdir("/test")
				require.NoError(t, err)
			},
			args:     args{p: "/test"},
			wantPath: "/test/",
			wantErr:  assert.NoError,
		},
		{
			name: "Existing Directory Trailing Slash",
			setup: func(vfs *BadgerVFS) {
				_, err := vfs.Mkdir("/test")
				require.NoError(t, err)
			},
			args:     args{p: "/test/"},
			wantPath: "/test/",
			wantErr:  assert.NoError,
		},
		{
			name: "Existing File",
			setup: func(vfs *BadgerVFS) {
				_, err := vfs.Mkdir("/test")
				require.NoError(t, err)
				_, err = vfs.Create("/test/file.txt", bytes.NewBufferString("content"))
				require.NoError(t, err)
			},
			args:     args{p: "/test/file.txt"},
			wantPath: "/test/file.txt",
			wantErr:  assert.NoError,
		},
		{
			name:    "Non-existent Path",
			setup:   nil,
			args:    args{p: "/nonexistent"},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			if tt.setup != nil {
				tt.setup(v)
			}

			got, err := v.StatByPath(tt.args.p)
			if !tt.wantErr(t, err, fmt.Sprintf("StatByPath(%v)", tt.args.p)) {
				return
			}
			if err == nil {
				assert.Equal(t, tt.wantPath, got.Path)
			}
		})
	}
}

func Test_BadgerVFS_WriteLargeFile(t *testing.T) {
	tests := []struct {
		name string
		size int64
	}{
		{
			name: "Exact Multiple (2.5MB)",
			size: int64(2.5 * 1024 * 1024), // 256KB * 10
		},
		{
			name: "ChunkSize + 1 byte",
			size: ChunkSize + 1,
		},
		{
			name: "ChunkSize - 1 byte",
			size: ChunkSize - 1,
		},
		{
			name: "Irregular Size (2MB + 12345 bytes)",
			size: int64(2*1024*1024) + 12345,
		},
		{
			name: "Small File (100 bytes)",
			size: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup BadgerVFS
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			largeContent := make([]byte, tt.size)
			_, err = rand.Read(largeContent)
			require.NoError(t, err)

			fileName := fmt.Sprintf("/test/%s.bin", tt.name)

			// Create Dir (ignore if exists)
			_, _ = v.Mkdir("/test")

			// Create File
			start := time.Now()
			meta, err := v.Create(fileName, bytes.NewReader(largeContent))
			require.NoError(t, err)
			t.Logf("Created file %s in %v", tt.name, time.Since(start))

			assert.Equal(t, tt.size, meta.Size)

			// Read File back
			f, err := v.Open(meta.ID)
			require.NoError(t, err)

			readContent, err := io.ReadAll(f)
			require.NoError(t, err)

			assert.Equal(t, tt.size, int64(len(readContent)), "Read content size should match written size")

			// Compare content
			if !bytes.Equal(largeContent, readContent) {
				t.Error("Read content does not match written content")
				// Find first mismatch for debugging
				for i := 0; i < len(largeContent); i++ {
					if i >= len(readContent) {
						t.Logf("Mismatch at offset %d: expected byte but read buffer ended", i)
						break
					}
					if largeContent[i] != readContent[i] {
						t.Logf("Mismatch at offset %d: expected %x, got %x", i, largeContent[i], readContent[i])
						break
					}
				}
			}
		})
	}
}

func Test_BadgerVFS_Seek(t *testing.T) {
	cfg := Config{
		InMemory: true,
		Logger:   logger,
	}
	v, err := New(cfg)
	require.NoError(t, err)
	defer v.Close()

	_, _ = v.Mkdir("/test")
	content := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	m, err := v.Create("/test/seek.txt", bytes.NewReader(content))
	require.NoError(t, err)

	f, err := v.Open(m.ID)
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

func Test_BadgerVFS_AtomicWrite_Rotation(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Internal ID Rotation on Write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				InMemory: true,
				Logger:   logger,
			}
			v, err := New(cfg)
			require.NoError(t, err)
			defer v.Close()

			// 1. Create File (Version 1)
			path := "/test/atomic.txt"
			_, _ = v.Mkdir("/test")
			content1 := []byte("version 1")
			meta, err := v.Create(path, bytes.NewReader(content1))
			require.NoError(t, err)

			// Get Internal ID 1
			var im internalMeta
			err = v.db.View(func(txn *badger.Txn) error {
				metaItem, findErr := findMetaItemByID(txn, meta.ID)
				if findErr != nil {
					return findErr
				}
				im, err = getMeta(metaItem)
				return err
			})
			require.NoError(t, err)
			require.NotEqual(t, uuid.Nil, im.InternalID)
			// Generally InternalID != PublicID for new files (though implementation creates new IDs for both)

			// 2. Write File (Version 2)
			content2 := []byte("version 2")
			_, err = v.Write(meta.ID, bytes.NewReader(content2))
			require.NoError(t, err)

			// Get Internal ID 2
			var im2 internalMeta
			err = v.db.View(func(txn *badger.Txn) error {
				metaItem, findErr := findMetaItemByID(txn, meta.ID)
				if findErr != nil {
					return findErr
				}
				im2, err = getMeta(metaItem)
				return err
			})
			require.NoError(t, err)

			// Verify Rotation
			assert.NotEqual(t, im.InternalID, im2.InternalID, "Internal ID must rotate on write")

			// Check Content
			f, err := v.Open(meta.ID)
			require.NoError(t, err)
			readContent, _ := io.ReadAll(f)
			assert.Equal(t, content2, readContent)
		})
	}
}

func Test_BadgerVFS_Move_Recursive(t *testing.T) {
	cfg := Config{
		InMemory: true,
		Logger:   logger,
	}
	v, err := New(cfg)
	require.NoError(t, err)
	defer v.Close()

	// Setup:
	// /parent
	// /parent/child1.txt
	// /parent/sub
	// /parent/sub/child2.txt
	_, err = v.Mkdir("/parent")
	require.NoError(t, err)

	_, err = v.Create("/parent/child1.txt", bytes.NewBufferString("child1 content"))
	require.NoError(t, err)

	_, err = v.Mkdir("/parent/sub")
	require.NoError(t, err)

	_, err = v.Create("/parent/sub/child2.txt", bytes.NewBufferString("child2 content"))
	require.NoError(t, err)

	// Action: Move /parent to /parent_moved
	parentMeta, err := v.StatByPath("/parent")
	require.NoError(t, err)

	_, err = v.Move(parentMeta.ID, "/parent_moved")
	require.NoError(t, err)

	// Assertion
	// 1. Old paths should not exist
	_, err = v.StatByPath("/parent")
	require.Error(t, err)
	_, err = v.StatByPath("/parent/child1.txt")
	require.Error(t, err)

	// 2. New paths should exist
	mov, err := v.StatByPath("/parent_moved")
	require.NoError(t, err)
	assert.Equal(t, "/parent_moved/", mov.Path)

	c1, err := v.StatByPath("/parent_moved/child1.txt")
	require.NoError(t, err)
	assert.Equal(t, "/parent_moved/child1.txt", c1.Path)

	s, err := v.StatByPath("/parent_moved/sub")
	require.NoError(t, err)
	assert.Equal(t, "/parent_moved/sub/", s.Path)

	c2, err := v.StatByPath("/parent_moved/sub/child2.txt")
	require.NoError(t, err)
	assert.Equal(t, "/parent_moved/sub/child2.txt", c2.Path)
}
