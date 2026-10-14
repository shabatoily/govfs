package badger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestCopyCompletesDuringRotation(t *testing.T) {
	b, err := New(&Config{Path: t.TempDir()})
	require.NoError(t, err)
	content := strings.Repeat("keep", 8<<20)
	meta, err := b.Create("/source", strings.NewReader(content))
	require.NoError(t, err)
	done := make(chan error, 2)
	go func() {
		_, err := b.Copy(meta.ID, "/copy")
		done <- err
	}()
	go func() {
		for i := range 8 {
			if err := b.Rotate([]byte(strings.Repeat(fmt.Sprint(i), 32))); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for range 2 {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-timer.C:
			t.Fatal("copy and rotation did not complete")
		}
	}
	defer b.Close()
	copied, err := b.StatByPath("/copy")
	require.NoError(t, err)
	file, err := b.Open(copied.ID)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
}

func TestRotationPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	b, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer b.Close()
	meta, err := b.Create("/file", strings.NewReader("keep"))
	require.NoError(t, err)
	require.NoError(t, b.Rotate([]byte(strings.Repeat("x", 32))))
	require.NoError(t, b.Close())
	reopened, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer reopened.Close()
	file, err := reopened.Open(meta.ID)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	_, err = reopened.Create("/new", strings.NewReader("new"))
	require.NoError(t, err)
}

func TestRejectedCreatesDoNotRetainChunks(t *testing.T) {
	b, err := New(&Config{InMemory: true})
	require.NoError(t, err)
	defer b.Close()
	_, err = b.Create("/file", strings.NewReader("keep"))
	require.NoError(t, err)
	before, err := b.Stats()
	require.NoError(t, err)
	for _, name := range []string{"/file", "/missing/file"} {
		_, err = b.Create(name, strings.NewReader("discard"))
		require.Error(t, err)
	}
	after, err := b.Stats()
	require.NoError(t, err)
	require.Equal(t, before["blob"], after["blob"])
}

func TestInterruptedRotationRestoresOriginalPair(t *testing.T) {
	dir := t.TempDir()
	b, err := New(&Config{Path: dir})
	require.NoError(t, err)
	meta, err := b.Create("/file", strings.NewReader("original"))
	require.NoError(t, err)
	require.NoError(t, b.Close())
	oldKey, err := os.ReadFile(filepath.Join(dir, DefaultSecretFilename))
	require.NoError(t, err)
	registry, err := os.ReadFile(filepath.Join(dir, badger.KeyRegistryFileName))
	require.NoError(t, err)
	backup := filepath.Join(dir, rotationDirectory)
	require.NoError(t, os.Mkdir(backup, rotationDirectoryMode))
	require.NoError(t, atomicSecret(filepath.Join(backup, DefaultSecretFilename), oldKey))
	require.NoError(t, atomicSecret(filepath.Join(backup, badger.KeyRegistryFileName), registry))
	require.NoError(t, atomicSecret(filepath.Join(backup, "ready"), nil))
	// registry만 갱신된 중단 상태에서 재개방이 이전 쌍을 복구해야 합니다.
	reg, err := badger.OpenKeyRegistry(badger.KeyRegistryOptions{Dir: dir, ReadOnly: true, EncryptionKey: oldKey})
	require.NoError(t, err)
	require.NoError(t, badger.WriteKeyRegistry(reg, badger.KeyRegistryOptions{Dir: dir, EncryptionKey: []byte(strings.Repeat("y", 32))}))
	require.NoError(t, reg.Close())
	reopened, err := New(&Config{Path: dir})
	require.NoError(t, err)
	defer reopened.Close()
	file, err := reopened.Open(meta.ID)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	restoredKey, err := os.ReadFile(filepath.Join(dir, DefaultSecretFilename))
	require.NoError(t, err)
	require.Equal(t, oldKey, restoredKey)
}

func TestOpenReaderContinuesAcrossRotation(t *testing.T) {
	b, err := New(&Config{Path: t.TempDir()})
	require.NoError(t, err)
	defer b.Close()
	content := strings.Repeat("keep", int(ChunkSize))
	meta, err := b.Create("/file", strings.NewReader(content))
	require.NoError(t, err)
	file, err := b.Open(meta.ID)
	require.NoError(t, err)
	defer file.Close()
	prefix := make([]byte, 7)
	_, err = io.ReadFull(file, prefix)
	require.NoError(t, err)
	require.NoError(t, b.Rotate([]byte(strings.Repeat("z", 32))))
	remainder, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, content, string(prefix)+string(remainder))
}

func TestKeyGenerationDoesNotOverwriteExistingSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), DefaultSecretFilename)
	key, err := GenerateEncryptionKey(file, 32)
	require.NoError(t, err)
	_, err = GenerateEncryptionKey(file, 32)
	require.ErrorIs(t, err, os.ErrExist)
	stored, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, key, stored)
}

func TestReadReusesChunkBuffer(t *testing.T) {
	b, err := New(&Config{InMemory: true})
	require.NoError(t, err)
	defer b.Close()
	content := strings.Repeat("a", int(ChunkSize)) + strings.Repeat("b", int(ChunkSize)) + "end"
	meta, err := b.Create("/chunks", strings.NewReader(content))
	require.NoError(t, err)
	file, err := b.Open(meta.ID)
	require.NoError(t, err)
	defer file.Close()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
	_, err = file.Seek(ChunkSize-2, io.SeekStart)
	require.NoError(t, err)
	buf := make([]byte, 4)
	_, err = io.ReadFull(file, buf)
	require.NoError(t, err)
	require.Equal(t, "aabb", string(buf))
}
