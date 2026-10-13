package badger

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
)

const (
	rotationDirectory     = ".key-rotation"
	rotationDirectoryMode = 0o700
)

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func atomicSecret(name string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(name), ".key-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(secretFileMode); err != nil {
		return errors.Join(err, file.Close())
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), name); err != nil {
		return err
	}
	return syncDir(filepath.Dir(name))
}

// recoverRotation은 커밋 기록이 없는 키 회전을 이전 키·registry 쌍으로 되돌립니다.
func recoverRotation(dir string) error {
	backup := filepath.Join(dir, rotationDirectory)
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(backup, "committed")); err == nil {
		return os.RemoveAll(backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Join(backup, "ready")); errors.Is(err, os.ErrNotExist) {
		return os.RemoveAll(backup)
	} else if err != nil {
		return err
	}
	for _, name := range []string{DefaultSecretFilename, badger.KeyRegistryFileName} {
		data, err := os.ReadFile(filepath.Join(backup, name))
		if err != nil {
			return err
		}
		if err := atomicSecret(filepath.Join(dir, name), data); err != nil {
			return fmt.Errorf("rotation rollback failed; originals retained at %s: %w", backup, err)
		}
	}
	return os.RemoveAll(backup)
}

func (bvfs *BadgerVFS) rotate(newKey []byte) (err error) {
	if err := bvfs.db.Close(); err != nil {
		return err
	}
	oldOptions := bvfs.options
	bvfs.failed = os.ErrClosed
	committed := false
	defer func() {
		if !committed {
			markerErr := os.Remove(filepath.Join(bvfs.path, rotationDirectory, "committed"))
			if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
				err = errors.Join(err, markerErr)
				bvfs.failed = markerErr
				return
			}
			rollbackErr := recoverRotation(bvfs.path)
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
				bvfs.failed = rollbackErr
				return
			}
			db, openErr := badger.Open(oldOptions)
			err = errors.Join(err, openErr)
			if openErr == nil {
				bvfs.db = db
				bvfs.failed = nil
			}
		}
	}()
	backup, err := bvfs.prepareRotation()
	if err != nil {
		return err
	}
	opt := badger.KeyRegistryOptions{
		Dir: bvfs.path, ReadOnly: true, EncryptionKey: bvfs.key,
		EncryptionKeyRotationDuration: bvfs.keyRotationDuration,
	}
	registry, err := badger.OpenKeyRegistry(opt)
	if err != nil {
		return err
	}
	opt.EncryptionKey = newKey
	writeErr := badger.WriteKeyRegistry(registry, opt)
	if err := errors.Join(writeErr, registry.Close()); err != nil {
		return err
	}
	if err := atomicSecret(filepath.Join(bvfs.path, DefaultSecretFilename), newKey); err != nil {
		return err
	}
	next := oldOptions.WithEncryptionKey(bytes.Clone(newKey))
	db, err := badger.Open(next)
	if err != nil {
		return err
	}
	if err := atomicSecret(filepath.Join(backup, "committed"), nil); err != nil {
		return errors.Join(err, db.Close())
	}
	bvfs.db = db
	bvfs.options = next
	bvfs.key = bytes.Clone(newKey)
	bvfs.failed = nil
	committed = true
	return os.RemoveAll(backup)
}

func (bvfs *BadgerVFS) prepareRotation() (string, error) {
	backup := filepath.Join(bvfs.path, rotationDirectory)
	if err := os.Mkdir(backup, rotationDirectoryMode); err != nil {
		return "", err
	}
	for _, name := range []string{DefaultSecretFilename, badger.KeyRegistryFileName} {
		data, err := os.ReadFile(filepath.Join(bvfs.path, name))
		if err != nil {
			return "", err
		}
		if err := atomicSecret(filepath.Join(backup, name), data); err != nil {
			return "", err
		}
	}
	if err := atomicSecret(filepath.Join(backup, "ready"), nil); err != nil {
		return "", err
	}
	if err := syncDir(bvfs.path); err != nil {
		return "", err
	}
	return backup, nil
}

// Resources는 키 회전과 충돌하지 않도록 리소스 조회 중 DB를 유지합니다.
func (bvfs *BadgerVFS) Resources() (lsm, vlog, block, index int64, err error) {
	bvfs.mu.RLock()
	defer bvfs.mu.RUnlock()
	if bvfs.failed != nil {
		return 0, 0, 0, 0, bvfs.failed
	}
	lsm, vlog = bvfs.db.Size()
	block, err = bvfs.db.CacheMaxCost(badger.BlockCache, -1)
	if err != nil {
		return
	}
	index, err = bvfs.db.CacheMaxCost(badger.IndexCache, -1)
	return
}
