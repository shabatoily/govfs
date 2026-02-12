package localstorage

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/meteormin/go-vfs"
)

const IndexFileName = "vfs_index.json"

type Config struct {
	Path   string `json:"path"`
	Logger *vfs.Logger
}

// LocalStorage implements the VFS interface using the local file system.
type LocalStorage struct {
	basePath string
	mu       sync.RWMutex
	idMap    map[uuid.UUID]vfs.Meta // ID -> Meta mapping
	pathMap  map[string]vfs.Meta    // Path -> Meta mapping (for fast lookup)
	logger   *vfs.Logger
}

func New(cfg Config) (*LocalStorage, error) {
	if err := os.MkdirAll(cfg.Path, 0o755); err != nil {
		return nil, err
	}

	ls := &LocalStorage{
		basePath: cfg.Path,
		idMap:    make(map[uuid.UUID]vfs.Meta),
		pathMap:  make(map[string]vfs.Meta),
		logger:   cfg.Logger,
	}

	// Try to load index
	indexFile := filepath.Join(ls.basePath, IndexFileName)
	if _, err := os.Stat(indexFile); err == nil {
		data, err := os.ReadFile(indexFile)
		if err != nil {
			return nil, err
		}
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
	return os.WriteFile(idxFile, data, vfs.DefaultFileMode)
}

func (ls *LocalStorage) toLocalPath(vfsPath string) string {
	// Remove leading slash to make it relative to basePath
	rel := strings.TrimPrefix(vfsPath, "/")
	return filepath.Join(ls.basePath, rel)
}

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

	// Simplescan: O(N) where N is total files.
	// For a real driver we might want a tree structure, but for benchmark simple is fine.
	var list []vfs.Meta
	for _, m := range ls.pathMap {
		// Parent check
		parent := getParentPath(m.Path)
		if parent == path {
			list = append(list, m)
		} else if strings.TrimSuffix(parent, "/")+"/" == path {
			// Handle root edge case or trailing slashes
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

func (ls *LocalStorage) Open(id uuid.UUID) (*vfs.File, error) {
	ls.mu.RLock()
	meta, ok := ls.idMap[id]
	ls.mu.RUnlock()
	if !ok {
		return nil, vfs.ErrNotFound
	}

	if meta.IsDir {
		return vfs.NewFile(meta, nil), nil
	}

	f, err := os.Open(ls.toLocalPath(meta.Path))
	if err != nil {
		return nil, err
	}
	// Note: updating access time could be done here if needed
	return vfs.NewFile(meta, f), nil
}

func (ls *LocalStorage) Create(path string, r io.Reader) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

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
	id, err := uuid.NewRandom()
	if err != nil {
		return vfs.Meta{}, err
	}

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
	return meta, nil
}

func (ls *LocalStorage) WriteComments(id uuid.UUID, comment string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	meta.Comments = comment
	return meta, nil
}

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

	id := uuid.New()
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

func (ls *LocalStorage) Stat(id uuid.UUID) (vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	m, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	return m, nil
}

func (ls *LocalStorage) StatByPath(p string) (vfs.Meta, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	m, ok := ls.pathMap[p]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}
	return m, nil
}

func (ls *LocalStorage) Move(id uuid.UUID, dst string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	meta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}

	dst = strings.TrimSpace(dst)
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	// Enforce trailing slash for directories to maintain consistency with Mkdir
	if meta.IsDir && !strings.HasSuffix(dst, "/") {
		dst += "/"
	}

	if _, exists := ls.pathMap[dst]; exists {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}

	srcLocal := ls.toLocalPath(meta.Path)
	dstLocal := ls.toLocalPath(dst)

	if err := os.MkdirAll(filepath.Dir(dstLocal), vfs.DefaultDirMode); err != nil {
		return vfs.Meta{}, err
	}

	if err := os.Rename(srcLocal, dstLocal); err != nil {
		return vfs.Meta{}, fmt.Errorf("rename failed: %w", err)
	}

	oldPath := meta.Path
	delete(ls.pathMap, oldPath)

	meta.Path = dst
	meta.Name = filepath.Base(strings.TrimSuffix(dst, "/"))
	meta.Modified = time.Now()

	ls.idMap[id] = meta
	ls.pathMap[dst] = meta

	if meta.IsDir {
		prefix := oldPath
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		dstPrefix := dst
		if !strings.HasSuffix(dstPrefix, "/") {
			dstPrefix += "/"
		}

		for uid, m := range ls.idMap {
			if uid == id {
				continue
			}
			if strings.HasPrefix(m.Path, prefix) {
				rel := strings.TrimPrefix(m.Path, prefix)
				newPath := dstPrefix + rel

				delete(ls.pathMap, m.Path)
				m.Path = newPath
				m.Modified = time.Now()
				ls.idMap[uid] = m
				ls.pathMap[newPath] = m
			}
		}
	}

	return meta, nil
}

func (ls *LocalStorage) Copy(id uuid.UUID, dst string) (vfs.Meta, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	srcMeta, ok := ls.idMap[id]
	if !ok {
		return vfs.Meta{}, vfs.ErrNotFound
	}

	if srcMeta.IsDir {
		return vfs.Meta{}, fmt.Errorf("directory copy not supported in this simplistic implementation")
	}

	dst = strings.TrimSpace(dst)
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	if _, exists := ls.pathMap[dst]; exists {
		return vfs.Meta{}, vfs.ErrAlreadyExists
	}

	srcLocal := ls.toLocalPath(srcMeta.Path)
	dstLocal := ls.toLocalPath(dst)

	if err := os.MkdirAll(filepath.Dir(dstLocal), vfs.DefaultDirMode); err != nil {
		return vfs.Meta{}, err
	}

	sFile, err := os.Open(srcLocal)
	if err != nil {
		return vfs.Meta{}, err
	}
	defer sFile.Close()

	dFile, err := os.Create(dstLocal)
	if err != nil {
		return vfs.Meta{}, err
	}
	defer dFile.Close()

	if _, err := io.Copy(dFile, sFile); err != nil {
		return vfs.Meta{}, err
	}

	newID, err := uuid.NewRandom()
	if err != nil {
		return vfs.Meta{}, err
	}

	newMeta := vfs.Meta{
		ID:        newID,
		Path:      dst,
		Name:      filepath.Base(strings.TrimSuffix(dst, "/")),
		Extension: strings.TrimPrefix(filepath.Ext(dst), "."),
		Size:      srcMeta.Size,
		IsDir:     false,
		Modified:  time.Now(),
	}

	ls.idMap[newID] = newMeta
	ls.pathMap[dst] = newMeta

	return newMeta, nil
}

func (ls *LocalStorage) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.saveIndex()
}

func (ls *LocalStorage) Rotate(_ []byte) error {
	return vfs.ErrNotSupported
}

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
	cw.n += uint64(n)
	return n, err
}

func (ls *LocalStorage) Load(r io.Reader, maxPendingWrites int) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// 1. Clear existing data
	if err := os.RemoveAll(ls.basePath); err != nil {
		return err
	}
	if err := os.MkdirAll(ls.basePath, vfs.DefaultDirMode); err != nil {
		return err
	}

	// 2. Extract
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

		if header.Name == IndexFileName {
			// Read index
			data, readErr := io.ReadAll(tr)
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
			if writeErr := os.WriteFile(filepath.Join(ls.basePath, IndexFileName), data, vfs.DefaultFileMode); writeErr != nil {
				return writeErr
			}
			continue
		}

		// Security: prevent path traversal
		targetPath := filepath.Join(ls.basePath, header.Name)
		if !strings.HasPrefix(targetPath, filepath.Clean(ls.basePath)+string(os.PathSeparator)) {
			return fmt.Errorf("security: illegal path traversal in tar file: %s", header.Name)
		}

		if header.Typeflag == tar.TypeDir {
			if mkdirErr := os.MkdirAll(targetPath, vfs.DefaultDirMode); mkdirErr != nil {
				return mkdirErr
			}
			continue
		}

		if mkdirErr := os.MkdirAll(filepath.Dir(targetPath), vfs.DefaultDirMode); mkdirErr != nil {
			return mkdirErr
		}

		f, openErr := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if openErr != nil {
			return openErr
		}

		if _, copyErr := io.Copy(f, tr); copyErr != nil {
			f.Close()
			return copyErr
		}
		f.Close()
	}

	return nil
}

func (ls *LocalStorage) Tree(path string) (*vfs.TreeNode, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if path == "" {
		path = "/"
	}

	rootMeta, ok := ls.pathMap[path]
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

	// Optimized O(N) implementation for Tree
	// 1. Build adjacency list (parent -> children)
	childrenMap := make(map[string][]vfs.Meta)
	for _, m := range ls.pathMap {
		if m.Path == "/" {
			continue
		}
		parent := getParentPath(m.Path)
		// Normalize parent
		if !strings.HasSuffix(parent, "/") {
			parent += "/"
		}
		childrenMap[parent] = append(childrenMap[parent], m)
	}

	return ls.buildTree(rootNode, childrenMap), nil
}

func (ls *LocalStorage) buildTree(root *vfs.TreeNode, childrenMap map[string][]vfs.Meta) *vfs.TreeNode {
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
			childNode = ls.buildTree(childNode, childrenMap)
		}
		root.Children = append(root.Children, childNode)
	}
	return root
}
