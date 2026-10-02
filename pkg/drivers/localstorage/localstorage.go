// Package localstorage는 로컬 파일 시스템을 백엔드 저장소로 사용하는 VFS 드라이버 구현을 제공합니다.
package localstorage

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	// 저장된 인덱스 로드 시도
	indexFile := filepath.Join(ls.basePath, IndexFileName)
	data, err := os.ReadFile(indexFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		var metas []vfs.Meta
		if err := json.Unmarshal(data, &metas); err != nil {
			return nil, err
		}
		for i := range metas {
			m := metas[i]
			ls.idMap[m.ID] = m
			ls.pathMap[m.Path] = m
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
	data, err := json.MarshalIndent(metas, "", "  ")
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
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), idxFile)
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

	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
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
	meta, ok := ls.idMap[id]
	ls.mu.RUnlock()
	if !ok {
		return nil, vfs.ErrNotFound
	}

	if meta.IsDir {
		return vfs.NewFile(&meta, nil), nil
	}

	f, err := os.Open(ls.toLocalPath(meta.Path))
	if err != nil {
		return nil, err
	}
	// Note: updating access time could be done here if needed
	return vfs.NewFile(&meta, f), nil
}

// Create은 로컬 시스템에 파일을 생성하고 데이터를 저장합니다.
func (ls *LocalStorage) Create(path string, r io.Reader) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == vfs.Root || path == "" {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}
	if !strings.HasPrefix(path, vfs.Root) {
		path = vfs.Root + path
	}

	if _, ok := ls.pathMap[path]; ok {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}

	// 1. Write to disk
	localPath := ls.toLocalPath(path)
	if err := os.MkdirAll(filepath.Dir(localPath), vfs.DefaultDirMode); err != nil {
		return vfs.Meta{}, err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return vfs.Meta{}, err
	}

	size, err := io.Copy(f, r)
	f.Close() // Close immediately after copy
	if err != nil {
		return vfs.Meta{}, err
	}

	// 2. Update Metadata
	id := uuid.NewV4()

	meta := vfs.Meta{
		ID:        id,
		Path:      path,
		Name:      filepath.Base(strings.TrimSuffix(path, "/")),
		Extension: strings.TrimPrefix(filepath.Ext(path), "."),
		Size:      size,
		IsDir:     false,
		Modified:  time.Now(),
	}

	ls.idMap[id] = meta
	ls.pathMap[path] = meta

	return meta, nil
}

// Write는 기존 파일의 내용을 덮어씁니다.
func (ls *LocalStorage) Write(id uuid.UUID, r io.Reader) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	if meta.IsDir {
		return vfs.Meta{}, vfs.ErrNotDir
	}

	localPath := ls.toLocalPath(meta.Path)
	f, err := os.Create(localPath) // Truncate and write
	if err != nil {
		return vfs.Meta{}, err
	}

	size, err := io.Copy(f, r)
	f.Close()
	if err != nil {
		return vfs.Meta{}, err
	}

	meta.Size = size
	meta.Modified = time.Now()
	ls.idMap[id] = meta
	ls.pathMap[meta.Path] = meta
	return meta, nil
}

// WriteComments는 지정된 ID의 파일 또는 디렉토리에 설명을 추가합니다.
func (ls *LocalStorage) WriteComments(id uuid.UUID, comment string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	meta.Comments = comment
	ls.idMap[id] = meta
	ls.pathMap[meta.Path] = meta
	return meta, nil
}

// Delete는 지정된 ID의 파일 또는 디렉토리(및 그 하위 항목)를 영구적으로 삭제합니다.
func (ls *LocalStorage) Delete(id uuid.UUID) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.ErrNotFound
	}

	localPath := ls.toLocalPath(meta.Path)
	if err := os.RemoveAll(localPath); err != nil {
		return err
	}

	delete(ls.idMap, id)
	delete(ls.pathMap, meta.Path)

	if meta.IsDir {
		prefix := meta.Path
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		for uid, m := range ls.idMap {
			if strings.HasPrefix(m.Path, prefix) {
				delete(ls.idMap, uid)
				delete(ls.pathMap, m.Path)
			}
		}
	}

	return nil
}

// Mkdir은 새로운 디렉토리를 생성합니다.
func (ls *LocalStorage) Mkdir(path string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	if _, ok := ls.pathMap[path]; ok {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}

	localPath := ls.toLocalPath(path)
	if err := os.MkdirAll(localPath, vfs.DefaultDirMode); err != nil {
		return vfs.Meta{}, err
	}

	id := uuid.NewV4()
	meta := vfs.Meta{
		ID:       id,
		Path:     path,
		Name:     filepath.Base(strings.TrimSuffix(path, "/")),
		IsDir:    true,
		Modified: time.Now(),
	}
	ls.idMap[id] = meta
	ls.pathMap[path] = meta
	return meta, nil
}

// Stat은 ID를 통해 파일 또는 디렉토리의 메타데이터를 조회합니다.
func (ls *LocalStorage) Stat(id uuid.UUID) (vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
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
	m, ok := ls.pathMap[p]
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
	src, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	dst, err := vfs.TransferPath(src, dst)
	if err != nil {
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

	if err := ls.transferFiles(src, dst, copyItem, targetMeta); err != nil {
		return vfs.Meta{}, err
	}
	return ls.transferMetadata(src, dst, copyItem, targetMeta), nil
}

// transferFiles는 디스크 전송 실패 시 교체 대상을 복구하고, 복구에 실패하면 임시 백업을 보존합니다.
func (ls *LocalStorage) transferFiles(src vfs.Meta, dst string, copyItem bool, target *vfs.Meta) error {
	tmp, err := os.MkdirTemp(ls.basePath, ".vfs-transfer-")
	if err != nil {
		return err
	}
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.RemoveAll(tmp)
		}
	}()
	sourcePath := ls.toLocalPath(src.Path)
	if copyItem {
		staged := filepath.Join(tmp, "item")
		if src.IsDir {
			err = os.CopyFS(staged, os.DirFS(sourcePath))
		} else {
			err = copyFile(sourcePath, staged)
		}
		if err != nil {
			return err
		}
		sourcePath = staged
	}
	destination := ls.toLocalPath(dst)
	if err := os.MkdirAll(filepath.Dir(strings.TrimSuffix(destination, "/")), vfs.DefaultDirMode); err != nil {
		return err
	}
	backup := filepath.Join(tmp, "previous")
	if target != nil {
		if err := os.Rename(ls.toLocalPath(target.Path), backup); err != nil {
			return err
		}
	}
	if err := os.Rename(strings.TrimSuffix(sourcePath, "/"), strings.TrimSuffix(destination, "/")); err != nil {
		if target != nil {
			if rollbackErr := os.Rename(backup, ls.toLocalPath(target.Path)); rollbackErr != nil {
				keepTemp = true
				return fmt.Errorf("transfer failed: %w; restore failed: %w; original retained at %s", err, rollbackErr, backup)
			}
		}
		return err
	}

	return nil
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

// copyFile은 복사가 완료된 임시 파일만 전송에 사용하도록 닫기 오류도 확인합니다.
func copyFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, vfs.DefaultFileMode)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return errors.Join(err, output.Close())
}

// Close는 드라이버를 안전하게 종료하고 현재 인덱스를 저장합니다.
func (ls *LocalStorage) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.saveIndex()
}

// Backup은 현재 VFS의 모든 파일과 인덱스를 tar.gz 형태로 묶어 출력 스트림으로 백업합니다.
func (ls *LocalStorage) Backup(w io.Writer, _ uint64) (uint64, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	// Use generic Writer wrapper to count bytes
	cw := &countWriter{w: w}

	gw := gzip.NewWriter(cw)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// 1. Snapshot Metadata
	metas := make([]vfs.Meta, 0, len(ls.idMap))
	for _, m := range ls.idMap {
		metas = append(metas, m)
	}
	indexData, err := json.MarshalIndent(metas, "", "  ")
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

	// 2. Write Files
	for _, m := range ls.idMap {
		if m.IsDir {
			continue
		}

		localPath := ls.toLocalPath(m.Path)
		info, err := os.Stat(localPath)
		if err != nil {
			// Skip missing files for robustness
			continue
		}

		f, err := os.Open(localPath)
		if err != nil {
			return 0, err
		}

		// Use relative path for tar header to allow portable restore
		relPath := strings.TrimPrefix(m.Path, "/")

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			f.Close()
			return 0, err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			f.Close()
			return 0, err
		}

		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return 0, err
		}
		f.Close()
	}

	return cw.n, nil
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

	// 1. Clear existing data
	if err := ls.clearData(); err != nil {
		return err
	}

	// 2. Extract
	return ls.extractArchive(r)
}

func (ls *LocalStorage) clearData() error {
	if err := os.RemoveAll(ls.basePath); err != nil {
		return err
	}
	return os.MkdirAll(ls.basePath, vfs.DefaultDirMode)
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

	return nil
}

func (ls *LocalStorage) extractTarEntry(header *tar.Header, tr *tar.Reader) error {
	if header.Name == IndexFileName {
		// Write index back to disk too
		return ls.writeIndexFile(tr)
	}

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

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, tr); err != nil {
		return err
	}
	return nil
}

// Tree는 지정된 경로 이하의 파일 시스템 구조를 트리 형태로 반환합니다.
func (ls *LocalStorage) Tree(path string) (*vfs.TreeNode, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if path == "" {
		path = "/"
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
	var metas []vfs.Meta
	if unmarshalErr := json.Unmarshal(data, &metas); unmarshalErr != nil {
		return unmarshalErr
	}
	for i := range metas {
		m := metas[i]
		ls.idMap[m.ID] = m
		ls.pathMap[m.Path] = m
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
