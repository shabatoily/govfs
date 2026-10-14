// Package localstorage는 로컬 파일 시스템을 백엔드 저장소로 사용하는 VFS 드라이버 구현을 제공합니다.
package localstorage

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/pkg/log"
)

// IndexFileName은 메타데이터 인덱스를 저장하는 파일명입니다.
const IndexFileName = ".vfs_index.json"

// Config는 LocalStorage 드라이버의 설정을 정의합니다.
type Config struct {
	// Path는 실제 파일들이 저장될 루트 경로입니다.
	Path string `json:"path"`
	// Logger는 VFS 로거 인스턴스입니다.
	Logger *log.Logger `json:"-"`
}

// LocalStorage는 로컬 파일 시스템을 시스템의 VFS 인터페이스로 매핑하는 구현체입니다.
type LocalStorage struct {
	basePath string
	mu       sync.RWMutex
	idMap    map[uuid.UUID]vfs.Meta // ID -> Meta 매핑
	pathMap  map[string]vfs.Meta    // Path -> Meta 매핑 (빠른 조회를 위함)
	logger   *log.Logger
	failed   error
}

// New는 주어진 설정을 기반으로 새로운 LocalStorage 드라이버 인스턴스를 생성합니다.
func New(cfg *Config) (*LocalStorage, error) {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	// 1. basePath를 미리 깔끔하게 정리하고 구분자를 붙여 경계를 명확히 합니다.
	basePath := filepath.Clean(cfg.Path)
	// 유닉스/윈도우 공통으로 안전하게 접두사를 확인하기 위해 구분자를 추가합니다.
	basePrefix := basePath + string(os.PathSeparator)
	if err := os.MkdirAll(basePrefix, vfs.DefaultDirMode); err != nil {
		return nil, err
	}

	ls := &LocalStorage{
		basePath: basePrefix,
		idMap:    make(map[uuid.UUID]vfs.Meta),
		pathMap:  make(map[string]vfs.Meta),
		logger:   cfg.Logger,
	}

	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := recoverTransaction(root); err != nil {
		return nil, err
	}
	data, err := root.ReadFile(IndexFileName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := ls.readIndex(data); err != nil {
			return nil, err
		}
		if err := ls.checkIndexedFiles(root); err != nil {
			return nil, err
		}
	}

	return ls, nil
}

func (ls *LocalStorage) saveIndex() error {
	idxFile := filepath.Join(ls.basePath, IndexFileName)
	metas := make([]vfs.Meta, 0, len(ls.idMap))
	for _, m := range ls.idMap {
		metas = append(metas, m)
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return err
	}
	// 완전히 저장한 파일만 교체하여 쓰기 실패 시 기존 인덱스를 보존합니다.
	tmp, err := os.CreateTemp(ls.basePath, ".vfs_index-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(vfs.DefaultFileMode); err != nil {
		return errors.Join(err, tmp.Close())
	}
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Sync(), tmp.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), idxFile); err != nil {
		return err
	}
	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return err
	}
	defer root.Close()
	return syncDirectory(root, ".")
}

func (ls *LocalStorage) toLocalPath(vfsPath string) string {
	// Remove leading slash to make it relative to basePath
	rel := strings.TrimPrefix(vfsPath, "/")
	return filepath.Join(ls.basePath, rel)
}

// List는 지정된 경로의 하위 파일 및 디렉토리 목록을 반환합니다.
func (ls *LocalStorage) List(path string) ([]vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return nil, ls.failed
	}

	var err error
	path, err = storagePath(path, true)
	if err != nil {
		return nil, err
	}

	// ponytail: 전체 메타데이터를 순회합니다. 조회가 병목이 되면 부모 경로별 인덱스를 추가합니다.
	var list []vfs.Meta
	for _, m := range ls.pathMap {
		parent := getParentPath(m.Path)
		if parent == path {
			list = append(list, m)
		}
	}
	return list, nil
}

func getParentPath(path string) string {
	if path == "/" {
		return "/"
	}
	path = strings.TrimSuffix(path, "/")
	dir := filepath.Dir(path)
	if dir == "/" {
		return "/"
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	return dir
}

// Open은 지정된 ID의 파일을 열고 vfs.File 인스턴스를 반환합니다.
func (ls *LocalStorage) Open(id uuid.UUID) (*vfs.File, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return nil, ls.failed
	}
	meta, ok := ls.idMap[id]
	if !ok {
		return nil, vfs.ErrNotFound
	}

	if meta.IsDir {
		return vfs.NewFile(&meta, nil), nil
	}

	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(relative(meta.Path))
	if err != nil {
		return nil, err
	}
	// Note: updating access time could be done here if needed
	return vfs.NewFile(&meta, f), nil
}

// Create은 로컬 시스템에 파일을 생성하고 데이터를 저장합니다.
func (ls *LocalStorage) Create(value string, r io.Reader) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	p, err := storagePath(value, false)
	if err != nil || p == "/" {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}
	if _, err := ls.statPath(p); err == nil {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}
	if err := ls.checkParent(p); err != nil {
		return vfs.Meta{}, err
	}
	meta := vfs.Meta{
		ID: uuid.NewV4(), Path: p, Name: filepath.Base(p),
		Extension: strings.TrimPrefix(filepath.Ext(p), "."), Modified: time.Now(),
	}
	err = ls.transaction(func(root *os.Root) ([]diskMove, error) {
		if _, err := root.Lstat(relative(p)); err == nil {
			return nil, vfs.ErrAlreadyExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		size, err := stageContent(root, r)
		meta.Size = size
		return []diskMove{{transactionDir + "/new", relative(p)}}, err
	}, func() { ls.idMap[meta.ID] = meta; ls.pathMap[p] = meta })
	return meta, err
}

// Write는 기존 파일의 내용을 덮어씁니다.
func (ls *LocalStorage) Write(id uuid.UUID, r io.Reader) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	if meta.IsDir {
		return vfs.Meta{}, vfs.ErrNotDir
	}
	err := ls.transaction(func(root *os.Root) ([]diskMove, error) {
		size, err := stageContent(root, r)
		meta.Size = size
		meta.Modified = time.Now()
		return []diskMove{{relative(meta.Path), transactionDir + "/old"}, {transactionDir + "/new", relative(meta.Path)}}, err
	}, func() { ls.idMap[id] = meta; ls.pathMap[meta.Path] = meta })
	return meta, err
}

// WriteComments는 지정된 ID의 파일 또는 디렉토리에 설명을 추가합니다.
func (ls *LocalStorage) WriteComments(id uuid.UUID, comment string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	meta.Comments = comment
	err := ls.transaction(emptyStage, func() { ls.idMap[id] = meta; ls.pathMap[meta.Path] = meta })
	return meta, err
}

// Delete는 지정된 ID의 파일 또는 디렉토리(및 그 하위 항목)를 영구적으로 삭제합니다.
func (ls *LocalStorage) Delete(id uuid.UUID) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return ls.failed
	}
	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.ErrNotFound
	}
	return ls.transaction(func(_ *os.Root) ([]diskMove, error) {
		return []diskMove{{relative(meta.Path), transactionDir + "/old"}}, nil
	}, func() {
		for uid, m := range ls.idMap {
			if uid == id || (meta.IsDir && strings.HasPrefix(m.Path, meta.Path)) {
				delete(ls.idMap, uid)
				delete(ls.pathMap, m.Path)
			}
		}
	})
}

// Mkdir은 새로운 디렉토리를 생성합니다.
func (ls *LocalStorage) Mkdir(value string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	p, err := storagePath(value, true)
	if err != nil || p == "/" {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}
	if _, err := ls.statPath(p); err == nil {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}
	if err := ls.checkParent(p); err != nil {
		return vfs.Meta{}, err
	}
	meta := vfs.Meta{ID: uuid.NewV4(), Path: p, Name: filepath.Base(strings.TrimSuffix(p, "/")), IsDir: true, Modified: time.Now()}
	err = ls.transaction(func(root *os.Root) ([]diskMove, error) {
		if _, err := root.Lstat(relative(p)); err == nil {
			return nil, vfs.ErrAlreadyExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		err := root.Mkdir(transactionDir+"/new", vfs.DefaultDirMode)
		return []diskMove{{transactionDir + "/new", relative(p)}}, err
	}, func() { ls.idMap[meta.ID] = meta; ls.pathMap[p] = meta })
	return meta, err
}

// Stat은 ID를 통해 파일 또는 디렉토리의 메타데이터를 조회합니다.
func (ls *LocalStorage) Stat(id uuid.UUID) (vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	m, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	return m, nil
}

// StatByPath는 경로를 통해 파일 또는 디렉토리의 메타데이터를 조회합니다.
func (ls *LocalStorage) StatByPath(p string) (vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	return ls.statPath(p)
}

func (ls *LocalStorage) statPath(value string) (vfs.Meta, error) {
	p, err := storagePath(value, false)
	if err != nil {
		return vfs.Meta{}, err
	}
	m, ok := ls.pathMap[p]
	if !ok {
		m, ok = ls.pathMap[p+"/"]
	}
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	return m, nil
}

// Move는 파일 또는 디렉토리를 새로운 경로로 이동시킵니다.
func (ls *LocalStorage) Move(id uuid.UUID, dst string, replaceID ...uuid.UUID) (vfs.Meta, error) {
	return ls.transfer(id, dst, replaceID, false)
}

// Copy는 파일 또는 디렉터리를 하위 항목과 함께 복사합니다.
func (ls *LocalStorage) Copy(id uuid.UUID, dst string, replaceID ...uuid.UUID) (vfs.Meta, error) {
	if len(replaceID) == 0 {
		replaceID = []uuid.UUID{uuid.Nil()}
	}
	return ls.transfer(id, dst, replaceID, true)
}

func (ls *LocalStorage) transfer(id uuid.UUID, dst string, replaceID []uuid.UUID, copyItem bool) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return vfs.Meta{}, ls.failed
	}
	src, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	dst, err := storagePath(dst, false)
	if err != nil {
		return vfs.Meta{}, err
	}
	dst, err = vfs.TransferPath(src, dst)
	if err != nil {
		return vfs.Meta{}, err
	}
	if err := ls.checkParent(dst); err != nil {
		return vfs.Meta{}, err
	}
	target, exists := ls.pathMap[strings.TrimSuffix(dst, "/")]
	if !exists {
		target, exists = ls.pathMap[strings.TrimSuffix(dst, "/")+"/"]
	}
	var targetMeta *vfs.Meta
	if exists {
		targetMeta = &target
	}
	if err := vfs.CheckReplacement(targetMeta, replaceID); err != nil {
		return vfs.Meta{}, err
	}

	var result vfs.Meta
	err = ls.transaction(func(root *os.Root) ([]diskMove, error) {
		source := relative(src.Path)
		if copyItem {
			staged := transactionDir + "/new"
			sourceRoot, err := root.OpenRoot(".")
			if err != nil {
				return nil, err
			}
			defer sourceRoot.Close()
			if src.IsDir {
				subtree, err := sourceRoot.OpenRoot(source)
				if err != nil {
					return nil, err
				}
				defer subtree.Close()
				if err := copyTree(root, subtree, staged); err != nil {
					return nil, err
				}
			} else if err := copyRootFile(root, sourceRoot, source, staged); err != nil {
				return nil, err
			}
			source = staged
		}
		moves := []diskMove{}
		if targetMeta != nil {
			moves = append(moves, diskMove{relative(targetMeta.Path), transactionDir + "/old"})
		} else if _, err := root.Lstat(relative(dst)); err == nil {
			return nil, vfs.ErrAlreadyExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return append(moves, diskMove{source, relative(dst)}), nil
	}, func() { result = ls.transferMetadata(src, dst, copyItem, targetMeta) })
	return result, err
}

// transferMetadata는 transfer가 잠금을 보유한 상태에서 전송 결과를 인덱스에 반영합니다.
func (ls *LocalStorage) transferMetadata(src vfs.Meta, dst string, copyItem bool, target *vfs.Meta) vfs.Meta {
	// 실제 파일 전송이 성공한 뒤에만 인덱스를 변경합니다.
	var items []vfs.Meta
	for _, meta := range ls.idMap {
		if meta.ID == src.ID || (src.IsDir && strings.HasPrefix(meta.Path, src.Path)) {
			items = append(items, meta)
		}
	}
	if target != nil {
		for uid, meta := range ls.idMap {
			if uid == target.ID || (target.IsDir && strings.HasPrefix(meta.Path, target.Path)) {
				delete(ls.idMap, uid)
				delete(ls.pathMap, meta.Path)
			}
		}
	}
	var result vfs.Meta
	for _, meta := range items {
		originalID := meta.ID
		if copyItem {
			meta.ID = uuid.NewV4()
		} else {
			delete(ls.pathMap, meta.Path)
		}
		meta.Path = dst + strings.TrimPrefix(meta.Path, src.Path)
		meta.Name = filepath.Base(strings.TrimSuffix(meta.Path, "/"))
		meta.Extension = strings.TrimPrefix(filepath.Ext(meta.Name), ".")
		meta.Modified = time.Now()
		ls.idMap[meta.ID] = meta
		ls.pathMap[meta.Path] = meta
		if originalID == src.ID {
			result = meta
		}
	}
	return result
}

func copyRootFile(dst, src *os.Root, from, to string) error {
	input, err := src.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := dst.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, vfs.DefaultFileMode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Sync(), output.Close())
}

func copyTree(dst, src *os.Root, target string) error {
	return fs.WalkDir(src.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return vfs.ErrInvalidPath
		}
		to := filepath.Join(target, name)
		if entry.IsDir() {
			return dst.Mkdir(to, vfs.DefaultDirMode)
		}
		if !entry.Type().IsRegular() {
			return vfs.ErrInvalidPath
		}
		return copyRootFile(dst, src, name, to)
	})
}

// Close는 드라이버를 안전하게 종료하고 현재 인덱스를 저장합니다.
func (ls *LocalStorage) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return ls.failed
	}
	return ls.saveIndex()
}

// Backup은 현재 VFS의 모든 파일과 인덱스를 tar.gz 형태로 묶어 출력 스트림으로 백업합니다.
func (ls *LocalStorage) Backup(w io.Writer, _ uint64) (size uint64, err error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return 0, ls.failed
	}

	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	// Use generic Writer wrapper to count bytes
	cw := &countWriter{w: w}

	gw := gzip.NewWriter(cw)
	tw := tar.NewWriter(gw)
	defer func() {
		tarErr := tw.Close()
		gzipErr := gw.Close()
		err = errors.Join(err, tarErr, gzipErr)
		size = cw.n
	}()

	// 1. Snapshot Metadata
	metas := make([]vfs.Meta, 0, len(ls.idMap))
	for _, m := range ls.idMap {
		metas = append(metas, m)
	}
	indexData, err := json.Marshal(metas)
	if err != nil {
		return 0, err
	}

	// Write Index to Tar
	hdr := &tar.Header{
		Name: IndexFileName,
		Mode: 0o644,
		Size: int64(len(indexData)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return 0, err
	}
	if _, err := tw.Write(indexData); err != nil {
		return 0, err
	}

	if err := ls.backupFiles(root, tw); err != nil {
		return 0, err
	}

	return cw.n, nil
}

func (ls *LocalStorage) backupFiles(root *os.Root, tw *tar.Writer) error {
	// 2. Write Files
	for _, m := range ls.idMap {
		if m.IsDir {
			continue
		}

		localPath := relative(m.Path)
		info, err := root.Stat(localPath)
		if err != nil {
			return err
		}

		f, err := root.Open(localPath)
		if err != nil {
			return err
		}

		// Use relative path for tar header to allow portable restore
		relPath := strings.TrimPrefix(m.Path, "/")

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			f.Close()
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			f.Close()
			return err
		}

		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	return nil
}

type countWriter struct {
	w io.Writer
	n uint64
}

func (cw *countWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		cw.n += uint64(n)
	}
	return n, err
}

// Load는 백업 스트림으로부터 데이터와 인덱스를 복구합니다.
func (ls *LocalStorage) Load(r io.Reader, _ int) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.failed != nil {
		return ls.failed
	}

	var restored *LocalStorage
	return ls.transaction(func(root *os.Root) ([]diskMove, error) {
		staging := transactionDir + "/new"
		if err := root.Mkdir(staging, vfs.DefaultDirMode); err != nil {
			return nil, err
		}
		restored = &LocalStorage{basePath: filepath.Join(ls.basePath, staging)}
		if err := restored.extractArchive(r); err != nil {
			return nil, err
		}
		if _, err := root.Stat(staging + "/" + IndexFileName); err != nil {
			return nil, err
		}
		if err := restored.validateRestoredFiles(); err != nil {
			return nil, err
		}
		entries, err := fs.ReadDir(root.FS(), ".")
		if err != nil {
			return nil, err
		}
		moves := []diskMove{}
		for i, entry := range entries {
			if entry.Name() == transactionDir || entry.Name() == IndexFileName {
				continue
			}
			moves = append(moves, diskMove{entry.Name(), fmt.Sprintf("%s/old-%d", transactionDir, i)})
		}
		next, err := fs.ReadDir(root.FS(), staging)
		if err != nil {
			return nil, err
		}
		for _, entry := range next {
			if entry.Name() != IndexFileName {
				moves = append(moves, diskMove{staging + "/" + entry.Name(), entry.Name()})
			}
		}
		return moves, nil
	}, func() { ls.idMap, ls.pathMap = restored.idMap, restored.pathMap })
}

// validateRestoredFiles는 인덱스의 파일이 실제로 복원됐는지 확인합니다.
func (ls *LocalStorage) validateRestoredFiles() error {
	root, err := os.OpenRoot(ls.basePath)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, meta := range ls.idMap {
		if meta.IsDir {
			if err := root.MkdirAll(relative(meta.Path), vfs.DefaultDirMode); err != nil {
				return err
			}
		}
		if _, err := root.Stat(relative(meta.Path)); err != nil {
			return err
		}
	}
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." || name == IndexFileName {
			return nil
		}
		p := "/" + name
		if entry.IsDir() {
			p += "/"
		}
		meta, ok := ls.pathMap[p]
		if !ok || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unindexed restored entry: %s", p)
		}
		info, err := root.Stat(name)
		if err != nil {
			return err
		}
		if meta.IsDir != info.IsDir() || (!meta.IsDir && (!info.Mode().IsRegular() || info.Size() != meta.Size)) {
			return fmt.Errorf("restored file does not match index: %s", p)
		}
		return nil
	})
}

func (ls *LocalStorage) extractArchive(r io.Reader) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	// Reset Maps
	ls.idMap = make(map[uuid.UUID]vfs.Meta)
	ls.pathMap = make(map[string]vfs.Meta)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if err := ls.extractTarEntry(header, tr); err != nil {
			return err
		}
	}

	// 체크섬을 검증하되 tar 종료 뒤의 추가 데이터는 1 MiB까지만 허용합니다.
	_, err = io.CopyN(io.Discard, gr, 1<<20)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("archive has excessive trailing data")
	}
	return err
}

func (ls *LocalStorage) extractTarEntry(header *tar.Header, tr *tar.Reader) error {
	if header.Name == IndexFileName {
		// Write index back to disk too
		return ls.writeIndexFile(tr)
	}

	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
		return vfs.ErrInvalidPath
	}
	clean, err := storagePath(header.Name, header.Typeflag == tar.TypeDir)
	if err != nil || clean == "/" {
		return vfs.ErrInvalidPath
	}
	header.Name = relative(clean)
	// Security: prevent path traversal
	// #nosec G305
	targetPath := filepath.Join(ls.basePath, header.Name)
	rel, err := filepath.Rel(ls.basePath, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("security: illegal path traversal in tar file: %s", header.Name)
	}

	if header.Typeflag == tar.TypeDir {
		return os.MkdirAll(targetPath, vfs.DefaultDirMode)
	}

	if mkdirAllErr := os.MkdirAll(filepath.Dir(targetPath), vfs.DefaultDirMode); mkdirAllErr != nil {
		return mkdirAllErr
	}

	if header.Mode < 0 {
		return fmt.Errorf("security: illegal file mode in tar file: %s", header.Name)
	}

	if header.Mode > math.MaxUint32 {
		return fmt.Errorf("security: illegal file mode in tar file: %s", header.Name)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, tr)
	return errors.Join(copyErr, f.Close())
}

// Tree는 지정된 경로 이하의 파일 시스템 구조를 트리 형태로 반환합니다.
func (ls *LocalStorage) Tree(path string) (*vfs.TreeNode, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	if ls.failed != nil {
		return nil, ls.failed
	}

	var err error
	path, err = storagePath(path, false)
	if err != nil {
		return nil, err
	}

	rootMeta, ok := ls.pathMap[path]
	if !ok {
		rootMeta, ok = ls.pathMap[strings.TrimSuffix(path, "/")+"/"]
	}
	if !ok {
		// Mock root if invalid? BadgerVFS handles virtual root.
		if path == "/" {
			rootMeta = vfs.Meta{
				Path:  "/",
				Name:  "/",
				IsDir: true,
			}
		} else {
			return nil, vfs.ErrNotFound
		}
	}

	rootNode := &vfs.TreeNode{
		Meta:     rootMeta,
		Children: nil,
	}

	// ponytail: 전체 메타데이터를 순회하되 대상 경로의 항목만 트리 구성에 사용합니다.
	prefix := strings.TrimSuffix(rootMeta.Path, "/") + "/"
	childrenMap := make(map[string][]vfs.Meta)
	for _, m := range ls.pathMap {
		if m.Path == rootMeta.Path || !strings.HasPrefix(m.Path, prefix) {
			continue
		}
		parent := getParentPath(m.Path)
		childrenMap[parent] = append(childrenMap[parent], m)
	}

	return buildTree(rootNode, childrenMap), nil
}

func (ls *LocalStorage) writeIndexFile(r io.Reader) error {
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		return readErr
	}
	if err := ls.readIndex(data); err != nil {
		return err
	}
	// Write index back to disk too
	return os.WriteFile(filepath.Join(ls.basePath, IndexFileName), data, vfs.DefaultFileMode)
}

func buildTree(root *vfs.TreeNode, childrenMap map[string][]vfs.Meta) *vfs.TreeNode {
	prefix := root.Meta.Path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	children := childrenMap[prefix]
	for _, m := range children {
		childNode := &vfs.TreeNode{
			Meta:     m,
			Children: nil,
		}
		if m.IsDir {
			childNode = buildTree(childNode, childrenMap)
		}
		root.Children = append(root.Children, childNode)
	}
	return root
}
