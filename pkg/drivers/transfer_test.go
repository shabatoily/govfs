package drivers_test

import (
	"io"
	"strings"
	"testing"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
	"github.com/shabatoily/govfs/pkg/drivers/localstorage"
	"github.com/stretchr/testify/require"
)

func TestTransfers(t *testing.T) {
	for _, driver := range []string{"badger", "localstorage"} {
		t.Run(driver, func(t *testing.T) {
			var fs vfs.VFS
			var err error
			if driver == "badger" {
				fs, err = badger.New(&badger.Config{InMemory: true})
			} else {
				fs, err = localstorage.New(&localstorage.Config{Path: t.TempDir()})
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, fs.Close()) })
			source, err := fs.Mkdir("/source")
			require.NoError(t, err)
			_, err = fs.Mkdir("/source/nested")
			require.NoError(t, err)
			file, err := fs.Create("/source/nested/file.txt", strings.NewReader("source contents"))
			require.NoError(t, err)
			_, err = fs.WriteComments(file.ID, "preserve me")
			require.NoError(t, err)

			copied, err := fs.Copy(source.ID, "/copied")
			require.NoError(t, err)
			require.NotEqual(t, source.ID, copied.ID)
			child, err := fs.StatByPath("/copied/nested/file.txt")
			require.NoError(t, err)
			require.NotEqual(t, file.ID, child.ID)
			require.Equal(t, "preserve me", child.Comments)
			readContents(t, fs, child.ID)

			_, err = fs.Copy(source.ID, "/copied")
			require.ErrorIs(t, err, vfs.ErrAlreadyExists)
			_, err = fs.Move(source.ID, "/copied", uuid.NewV4())
			require.ErrorIs(t, err, vfs.ErrAlreadyExists)
			_, err = fs.Copy(source.ID, "/missing", copied.ID)
			require.ErrorIs(t, err, vfs.ErrAlreadyExists)
			readContents(t, fs, child.ID)

			for _, dst := range []string{"/source", "/source/child", "/", "/source/../source"} {
				_, err = fs.Move(source.ID, dst)
				require.ErrorIs(t, err, vfs.ErrInvalidPath)
			}

			moved, err := fs.Move(source.ID, "/copied")
			require.NoError(t, err)
			require.Equal(t, source.ID, moved.ID)
			_, err = fs.Stat(child.ID)
			require.ErrorIs(t, err, vfs.ErrNotFound)
			_, err = fs.StatByPath("/source")
			require.ErrorIs(t, err, vfs.ErrNotFound)
			readContents(t, fs, file.ID)

			target, err := fs.Create("/target.txt", strings.NewReader("old"))
			require.NoError(t, err)
			newFile, err := fs.Copy(file.ID, "/target.txt", target.ID)
			require.NoError(t, err)
			require.NotEqual(t, file.ID, newFile.ID)
			_, err = fs.Stat(target.ID)
			require.ErrorIs(t, err, vfs.ErrNotFound)
			readContents(t, fs, newFile.ID)

			// 파일과 디렉터리의 끝 슬래시가 달라도 충돌을 검사합니다.
			_, err = fs.Copy(moved.ID, "/target.txt")
			require.ErrorIs(t, err, vfs.ErrAlreadyExists)
			replaced, err := fs.Copy(moved.ID, "/target.txt", newFile.ID)
			require.NoError(t, err)
			require.True(t, replaced.IsDir)
			_, err = fs.Stat(newFile.ID)
			require.ErrorIs(t, err, vfs.ErrNotFound)
		})
	}
}

func readContents(t *testing.T, fs vfs.VFS, id uuid.UUID) {
	t.Helper()
	file, err := fs.Open(id)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, "source contents", string(data))
}
