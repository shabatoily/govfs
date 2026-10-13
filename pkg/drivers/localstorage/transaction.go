package localstorage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"strings"
	"uuid"

	vfs "github.com/shabatoily/govfs"
)

const (
	transactionDir       = ".vfs-transaction"
	privateDirectoryMode = 0o700
)

type diskMove struct{ From, To string }

func storagePath(value string, directory bool) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\\\x00") {
		return "", vfs.ErrInvalidPath
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." || strings.HasPrefix(part, ".vfs") {
			return "", vfs.ErrInvalidPath
		}
	}
	value = path.Clean("/" + value)
	if directory && value != "/" {
		value += "/"
	}
	return value, nil
}

func relative(value string) string { return strings.Trim(strings.TrimPrefix(value, "/"), "/") }

func (ls *LocalStorage) checkParent(value string) error {
	parent := getParentPath(value)
	if parent == "/" {
		return nil
	}
	meta, err := ls.statPath(parent)
	if err != nil {
		return err
	}
	if !meta.IsDir {
		return vfs.ErrNotDir
	}
	return nil
}

func (ls *LocalStorage) readIndex(data []byte) error {
	var metas []vfs.Meta
	if err := json.Unmarshal(data, &metas); err != nil {
		return err
	}
	ids := make(map[uuid.UUID]vfs.Meta)
	paths := make(map[string]vfs.Meta)
	for _, meta := range metas {
		clean, err := storagePath(meta.Path, meta.IsDir)
		if err != nil || clean != meta.Path || clean == "/" || meta.ID == uuid.Nil() || meta.Size < 0 ||
			meta.Name != path.Base(strings.TrimSuffix(clean, "/")) {
			return fmt.Errorf("invalid index entry: %s", meta.Path)
		}
		if _, ok := ids[meta.ID]; ok {
			return fmt.Errorf("duplicate index ID: %s", meta.ID)
		}
		key := strings.TrimSuffix(clean, "/")
		if _, ok := paths[key]; ok {
			return fmt.Errorf("duplicate index path: %s", clean)
		}
		ids[meta.ID] = meta
		paths[key] = meta
	}
	ls.idMap = ids
	ls.pathMap = make(map[string]vfs.Meta, len(paths))
	for _, meta := range ids {
		ls.pathMap[meta.Path] = meta
	}
	for _, meta := range ids {
		if err := ls.checkParent(meta.Path); err != nil {
			return fmt.Errorf("invalid parent for %s: %w", meta.Path, err)
		}
	}
	return nil
}

func (ls *LocalStorage) checkIndexedFiles(root *os.Root) error {
	for _, meta := range ls.idMap {
		info, err := root.Stat(relative(meta.Path))
		if err != nil {
			return err
		}
		if info.IsDir() != meta.IsDir || (!meta.IsDir && (!info.Mode().IsRegular() || info.Size() != meta.Size)) {
			return fmt.Errorf("stored file does not match index: %s", meta.Path)
		}
	}
	return nil
}

func syncDirectory(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func writeSynced(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, vfs.DefaultFileMode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Sync(), f.Close())
}

// recoverTransaction은 인덱스 rename을 커밋 기준으로 삼아 미완료 파일 이동을 역순으로 복구합니다.
func recoverTransaction(root *os.Root) error {
	data, err := root.ReadFile(transactionDir + "/journal")
	if errors.Is(err, os.ErrNotExist) {
		return root.RemoveAll(transactionDir)
	}
	if err != nil {
		return err
	}
	var moves []diskMove
	if err := json.Unmarshal(data, &moves); err != nil {
		return err
	}
	if _, err := root.Lstat(transactionDir + "/index"); err == nil {
		for i := len(moves) - 1; i >= 0; i-- {
			move := moves[i]
			if !isInternalPath(move.From) || !isInternalPath(move.To) {
				return errors.New("invalid recovery path")
			}
			_, fromErr := root.Lstat(move.From)
			_, toErr := root.Lstat(move.To)
			if fromErr == nil {
				continue
			}
			if !errors.Is(fromErr, os.ErrNotExist) {
				return fromErr
			}
			if errors.Is(toErr, os.ErrNotExist) {
				continue
			}
			if toErr != nil {
				return toErr
			}
			if err := root.Rename(move.To, move.From); err != nil {
				return fmt.Errorf("rollback failed; data retained in %s: %w", transactionDir, err)
			}
			for _, dir := range []string{path.Dir(move.From), path.Dir(move.To)} {
				if err := syncDirectory(root, dir); err != nil {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syncDirectory(root, "."); err != nil {
		return err
	}
	return root.RemoveAll(transactionDir)
}

func isInternalPath(value string) bool {
	return value != "" && value != "." && !path.IsAbs(value) && path.Clean(value) == value &&
		!strings.ContainsAny(value, "\\\x00") && !strings.HasPrefix(value, "../")
}

// transaction은 파일 변경 전에 복구 기록을 저장하고 파일과 인덱스를 함께 커밋합니다.
func (ls *LocalStorage) transaction(stage func(*os.Root) ([]diskMove, error), update func()) (err error) {
	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Mkdir(transactionDir, privateDirectoryMode); err != nil {
		return fmt.Errorf("unfinished storage transaction: %w", err)
	}
	beforeIDs, beforePaths := maps.Clone(ls.idMap), maps.Clone(ls.pathMap)
	committed := false
	defer func() {
		if !committed {
			ls.idMap, ls.pathMap = beforeIDs, beforePaths
		}
		recoveryErr := recoverTransaction(root)
		if recoveryErr != nil {
			ls.failed = recoveryErr
		}
		err = errors.Join(err, recoveryErr)
	}()
	moves, err := stage(root)
	if err != nil {
		return err
	}
	update()
	metas := make([]vfs.Meta, 0, len(ls.idMap))
	for _, meta := range ls.idMap {
		metas = append(metas, meta)
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return err
	}
	if err := writeSynced(root, transactionDir+"/index", data); err != nil {
		return err
	}
	journal, err := json.Marshal(moves)
	if err != nil {
		return err
	}
	if err := writeSynced(root, transactionDir+"/journal.new", journal); err != nil {
		return err
	}
	if err := root.Rename(transactionDir+"/journal.new", transactionDir+"/journal"); err != nil {
		return err
	}
	if err := syncStaging(root); err != nil {
		return err
	}
	if err := syncDirectory(root, "."); err != nil {
		return err
	}
	for _, move := range moves {
		if err := root.Rename(move.From, move.To); err != nil {
			return err
		}
		for _, dir := range []string{path.Dir(move.From), path.Dir(move.To)} {
			if err := syncDirectory(root, dir); err != nil {
				return err
			}
		}
	}
	if err := root.Rename(transactionDir+"/index", IndexFileName); err != nil {
		return err
	}
	committed = true
	return syncDirectory(root, ".")
}

// syncStaging은 복원·복사한 하위 파일과 디렉터리도 커밋 전에 동기화합니다.
func syncStaging(root *os.Root) error {
	return fs.WalkDir(root.FS(), transactionDir, func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		return errors.Join(file.Sync(), file.Close())
	})
}

func stageContent(root *os.Root, r io.Reader) (int64, error) {
	f, err := root.OpenFile(transactionDir+"/new", os.O_CREATE|os.O_EXCL|os.O_WRONLY, vfs.DefaultFileMode)
	if err != nil {
		return 0, err
	}
	size, copyErr := io.Copy(f, r)
	return size, errors.Join(copyErr, f.Sync(), f.Close())
}

func emptyStage(_ *os.Root) ([]diskMove, error) { return nil, nil }
