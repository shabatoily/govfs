package badger

import (
	"context"
	"errors"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
)

const (
	Byte int64 = 1
	KiB        = 1024 * Byte
	MiB        = 1024 * KiB
)

var (
	DefaultSecretFilename = ".secret"
	DefaultSecretKeySize  = 32
	ChunkSize             = 256 * KiB
)

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, ChunkSize)
		return &b
	},
}

var (
	// prefixMeta key: `meta:{path}` value: `vfs.Meta`
	prefixMeta = []byte("meta:")
	// prefixBlob key: `blob:{ID}` value: `[]byte`
	prefixBlob = []byte("blob:")
	// prefixIndex key: `index:{ID}` value: `uuid.UUID`
	prefixIndex = []byte("index:")
)

// BadgerConfig Badger DB의 설정을 정의합니다.
type Config struct {
	Context                       context.Context `json:"-"`
	Path                          string          `json:"path"`
	CacheSize                     int64           `json:"cacheSize"`
	EncryptKey                    []byte          `json:"-"`
	EncryptionKeyRotationDuration time.Duration   `json:"encryptionKeyRotationDuration"`
	GCInterval                    time.Duration   `json:"gcInterval"`
	GCDiscardRatio                float64         `json:"gcRatio"`
	InMemory                      bool            `json:"inMemory"`
	Logger                        *vfs.Logger     `json:"-"`
}

func (cfg *Config) Options() badger.Options {
	if cfg.Path == "" {
		// 강제 In-memory 모드
		cfg.InMemory = true
	}
	if cfg.EncryptKey == nil {
		var encryptKey []byte
		var err error
		if cfg.InMemory {
			encryptKey, err = randomSecretKey(DefaultSecretKeySize)
		} else {
			secretFile := filepath.Join(cfg.Path, DefaultSecretFilename)
			encryptKey, err = getEncryptionKey(secretFile)
			if err != nil {
				encryptKey, err = GenerateEncryptionKey(secretFile, DefaultSecretKeySize)
			}
		}
		if err == nil {
			cfg.EncryptKey = encryptKey
		}
	}
	if cfg.CacheSize == 0 {
		cfg.CacheSize = 100
	}
	if cfg.EncryptionKeyRotationDuration == 0 {
		cfg.EncryptionKeyRotationDuration = time.Hour * 24
	}

	return badger.DefaultOptions(cfg.Path).
		WithInMemory(cfg.InMemory).
		WithEncryptionKey(cfg.EncryptKey).
		WithEncryptionKeyRotationDuration(cfg.EncryptionKeyRotationDuration).
		WithIndexCacheSize(cfg.CacheSize)
}

type BadgerVFS struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	db                  *badger.DB
	logger              *vfs.Logger
	path                string
	key                 []byte
	keyRotationDuration time.Duration
}

func New(cfg *Config) (*BadgerVFS, error) {
	opts := cfg.Options()
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if cfg.Context == nil {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithCancel(cfg.Context)
	}

	if cfg.Logger == nil {
		cfg.Logger = vfs.DefaultLogger
	}

	bvfs := &BadgerVFS{
		ctx:                 ctx,
		cancel:              cancel,
		db:                  db,
		logger:              cfg.Logger,
		path:                cfg.Path,
		key:                 cfg.EncryptKey,
		keyRotationDuration: cfg.EncryptionKeyRotationDuration,
	}

	if cfg.GCInterval > 0 {
		if cfg.GCDiscardRatio == 0 {
			cfg.GCDiscardRatio = 0.7
		}
		go bvfs.runGC(cfg.GCInterval, cfg.GCDiscardRatio)
	}

	return bvfs, nil
}

func (bvfs *BadgerVFS) List(path string) ([]vfs.Meta, error) {
	// 1. Normalize path to always end with a slash
	path = strings.TrimSpace(path)
	if path == "" {
		path = vfs.Root
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	// 2. Check if the directory itself exists (unless it's the root).
	// The meta key for a directory is stored with a trailing slash.
	if path != "/" {
		err := bvfs.db.View(func(txn *badger.Txn) error {
			metaKey := makeKey(prefixMeta, []byte(path))
			_, internalErr := txn.Get(metaKey) // Check for key like "meta:/your/dir/"
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		})
		if err != nil {
			return nil, err
		}
	}

	// 3. Proceed to list the contents.
	metaKey := makeKey(prefixMeta, []byte(path))
	list := make([]vfs.Meta, 0)
	err := bvfs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(metaKey); it.ValidForPrefix(metaKey); it.Next() {
			item := it.Item()
			key := string(item.Key())
			subpath := strings.TrimPrefix(key, string(metaKey))

			// Skip the directory itself (whose subpath is "")
			if subpath == "" {
				continue
			}

			// --- CRITICAL FIX ---
			// This condition now correctly identifies immediate children.
			// - "file.txt"        -> TrimSuffix -> "file.txt"        -> Contains("/") is false. (INCLUDED)
			// - "subdir/"         -> TrimSuffix -> "subdir"          -> Contains("/") is false. (INCLUDED)
			// - "subdir/file.txt" -> TrimSuffix -> "subdir/file.txt" -> Contains("/") is true.  (SKIPPED)
			if strings.Contains(strings.TrimSuffix(subpath, "/"), "/") {
				continue
			}

			err := item.Value(func(v []byte) error {
				var im internalMeta
				if err := json.Unmarshal(v, &im); err == nil {
					list = append(list, im.Meta)
				} else {
					bvfs.logger.Error().Err(err).Msg("Failed to decode meta")
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	bvfs.logger.Debug().Str("path", path).Int("count", len(list)).Msg("List")

	return list, err
}

func (bvfs *BadgerVFS) Open(id uuid.UUID) (*vfs.File, error) {
	var im internalMeta
	var reader io.ReadCloser

	err := bvfs.db.View(func(txn *badger.Txn) error {
		metaItem, internalErr := findMetaItemByID(txn, id)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		im, internalErr = getMeta(metaItem)
		return internalErr
	})
	if err != nil {
		return nil, err
	}

	if !im.IsDir {
		// Use internalID for data access
		// If internalID is nil (legacy or dir), it will fall back to meta.ID inside getMeta logic but for safety check
		if im.InternalID == uuid.Nil {
			im.InternalID = im.ID
		}
		idBytes, _ := im.InternalID.MarshalBinary()
		reader = &blobReader{
			vfs:  bvfs,
			id:   idBytes,
			size: im.Size,
		}
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("Open")

	return vfs.NewFile(&im.Meta, reader), nil
}

func (bvfs *BadgerVFS) Create(path string, r io.Reader) (vfs.Meta, error) {
	if path == vfs.Root {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}

	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return vfs.Meta{}, err
	}
	// For new file, Public ID and Internal ID are both new and distinct
	// specific internalID is generated for data storage
	internalID, err := uuid.NewRandom()
	if err != nil {
		return vfs.Meta{}, err
	}

	internalIDBytes, err := internalID.MarshalBinary()
	if err != nil {
		return vfs.Meta{}, err
	}

	// 1. Write chunks first (to avoid holding lock on meta or parent)
	size, err := bvfs.writeChunks(internalIDBytes, r)
	if err != nil {
		_ = bvfs.deleteChunks(internalIDBytes) // Clean up orphan chunks on failure
		return vfs.Meta{}, err
	}

	var meta vfs.Meta
	err = bvfs.db.Update(func(txn *badger.Txn) error {
		parentDir := getParentPath(path)
		if parentDir != vfs.Root {
			parentMeta, internalErr := findByPath(txn, parentDir)
			if internalErr != nil {
				return internalErr
			}
			if !parentMeta.IsDir {
				return vfs.ErrNotDir
			}
		}

		metaKey := makeKey(prefixMeta, []byte(path))
		if _, internalErr := txn.Get(metaKey); internalErr == nil {
			return vfs.ErrAlreadyExists
		} else if !errors.Is(internalErr, badger.ErrKeyNotFound) {
			return internalErr
		}

		meta = vfs.Meta{
			ID:        id,
			Path:      path,
			Name:      filepath.Base(path),
			Extension: extractExtension(path),
			IsDir:     false,
			Size:      size,
			Modified:  time.Now(),
		}

		im := internalMeta{
			Meta:       meta,
			InternalID: internalID,
		}

		if internalErr := setMeta(txn, metaKey, &im); internalErr != nil {
			return internalErr
		}

		idBytes, _ := id.MarshalBinary()
		indexKey := makeKey(prefixIndex, idBytes)
		if internalErr := txn.Set(indexKey, []byte(path)); internalErr != nil {
			return internalErr
		}
		return nil
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", meta.ID.String()).Str("Path", meta.Path).Msg("Create")

	return meta, nil
}

func (bvfs *BadgerVFS) Write(id uuid.UUID, r io.Reader) (vfs.Meta, error) {
	var im internalMeta
	var oldInternalID uuid.UUID

	idBytes, err := id.MarshalBinary()
	if err != nil {
		return vfs.Meta{}, err
	}

	// 1. Generate New Internal ID
	newInternalID, err := uuid.NewRandom()
	if err != nil {
		return vfs.Meta{}, err
	}
	newInternalIDBytes, err := newInternalID.MarshalBinary()
	if err != nil {
		return vfs.Meta{}, err
	}

	// 2. Write New Chunks (Stream)
	size, err := bvfs.writeChunks(newInternalIDBytes, r)
	if err != nil {
		_ = bvfs.deleteChunks(newInternalIDBytes) // Cleanup on failure
		return vfs.Meta{}, err
	}

	// 3. Atomic Swap (Transaction)
	err = bvfs.db.Update(func(txn *badger.Txn) error {
		indexKey := makeKey(prefixIndex, idBytes)
		item, getErr := txn.Get(indexKey)
		if getErr != nil {
			if errors.Is(getErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return getErr
		}

		var path string
		getValueErr := item.Value(func(v []byte) error {
			path = string(v)
			return nil
		})
		if getValueErr != nil {
			return getValueErr
		}

		metaKey := makeKey(prefixMeta, []byte(path))
		metaItem, getMetaErr := txn.Get(metaKey)
		if getMetaErr != nil {
			if errors.Is(getMetaErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return getMetaErr
		}

		// Get current meta and old internal ID
		im, getMetaErr = getMeta(metaItem)
		if getMetaErr != nil {
			return getMetaErr
		}

		if im.IsDir {
			return vfs.ErrAlreadyExists
		}

		// Update fields
		im.Size = size
		im.Modified = time.Now()
		oldInternalID = im.InternalID
		im.InternalID = newInternalID

		// Save with NEW Internal ID
		return setMeta(txn, metaKey, &im)
	})
	if err != nil {
		// New chunks are garbage now if update failed
		_ = bvfs.deleteChunks(newInternalIDBytes)
		return vfs.Meta{}, err
	}

	// 4. Delete Old Chunks (Best Effort / Cleanup)
	// If this fails, we have orphan chunks but data is consistent.
	if oldInternalID != uuid.Nil {
		oldIDBytes, _ := oldInternalID.MarshalBinary()
		if err := bvfs.deleteChunks(oldIDBytes); err != nil {
			return vfs.Meta{}, err
		}
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("Write")

	return im.Meta, nil
}

func (bvfs *BadgerVFS) writeChunks(id []byte, r io.Reader) (int64, error) {
	var size int64
	var seq uint32

	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)
	buf := *bufPtr

	wb := bvfs.db.NewWriteBatch()
	defer wb.Cancel()

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			// Create a copy of the key to ensure it's not modified by subsequent iterations
			// before Badger processes it. usage of the same buffer for key caused issues.
			// makeChunkKey allocates a new byte slice for every chunk, ensuring safety.
			currentKey := makeChunkKey(id, seq)

			// Create a copy of the value buffer. Although WriteBatch.Set is supposed to copy,
			// explicit copying guarantees safety against buffer reuse in the loop.
			valCopy := make([]byte, n)
			copy(valCopy, buf[:n])

			if setErr := wb.Set(currentKey, valCopy); setErr != nil {
				return size, setErr
			}
			size += int64(n)
			seq++
		}
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return size, err
		}
	}

	if err := wb.Flush(); err != nil {
		return size, err
	}

	return size, nil
}

func (bvfs *BadgerVFS) deleteChunks(id []byte) error {
	return bvfs.db.Update(func(txn *badger.Txn) error {
		return deleteChunks(txn, id)
	})
}

func (bvfs *BadgerVFS) WriteComments(id uuid.UUID, comments string) (vfs.Meta, error) {
	var im internalMeta

	err := bvfs.db.Update(func(txn *badger.Txn) error {
		metaItem, internalErr := findMetaItemByID(txn, id)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		im, internalErr = getMeta(metaItem)
		if internalErr != nil {
			return internalErr
		}

		im.Comments = comments
		metaKey := makeKey(prefixMeta, []byte(im.Path))
		// Preserve internalID
		return setMeta(txn, metaKey, &im)
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("WriteComments")

	return im.Meta, nil
}

func (bvfs *BadgerVFS) Delete(id uuid.UUID) error {
	return bvfs.db.Update(func(txn *badger.Txn) error {
		metaItem, internalErr := findMetaItemByID(txn, id)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		var im internalMeta
		im, internalErr = getMeta(metaItem)
		if internalErr != nil {
			return internalErr
		}

		// If directory, we need to delete all descendants too.
		// We can do this by scanning for the prefix.
		if im.IsDir {
			prefix := im.Path
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}

			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()

			seekKey := makeKey(prefixMeta, []byte(prefix))
			var itemsToDelete []internalMeta

			// Collect all items (self + descendants)
			for it.Seek(seekKey); it.ValidForPrefix(seekKey); it.Next() {
				m, err := getMeta(it.Item())
				if err != nil {
					continue
				}
				itemsToDelete = append(itemsToDelete, m)
			}

			if err := deleteItems(txn, itemsToDelete); err != nil {
				return err
			}
		} else {
			err := deleteItem(txn, &im)
			if err != nil {
				return err
			}
		}

		bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("Delete")
		return nil
	})
}

func (bvfs *BadgerVFS) Mkdir(path string) (vfs.Meta, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	var im internalMeta
	err := bvfs.db.Update(func(txn *badger.Txn) error {
		parentDir := getParentPath(path)
		if parentDir != vfs.Root {
			parentMeta, internalErr := findByPath(txn, parentDir)
			if internalErr != nil {
				return internalErr
			}
			if !parentMeta.IsDir {
				return vfs.ErrNotDir
			}
		}

		// 존재하는지 확인
		_, internalErr := txn.Get([]byte("meta:" + path))
		if internalErr == nil {
			return vfs.ErrAlreadyExists
		}

		newUUID, err := uuid.NewRandom()
		if err != nil {
			return err
		}

		meta := vfs.Meta{
			ID:       newUUID,
			Path:     path,
			Name:     filepath.Base(strings.TrimSuffix(path, "/")),
			IsDir:    true,
			Size:     0,
			Modified: time.Now(),
		}

		im = internalMeta{
			Meta:       meta,
			InternalID: uuid.Nil,
		}

		metaKey := makeKey(prefixMeta, []byte(im.Path))
		internalErr = setMeta(txn, metaKey, &im) // Dir has no internal ID for data
		if internalErr != nil {
			return internalErr
		}

		idBytes, internalErr := im.ID.MarshalBinary()
		if internalErr != nil {
			return internalErr
		}
		indexKey := makeKey(prefixIndex, idBytes)
		internalErr = txn.Set(indexKey, []byte(path))
		if internalErr != nil {
			return internalErr
		}
		return nil
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("Mkdir")

	return im.Meta, nil
}

func (bvfs *BadgerVFS) StatByPath(p string) (vfs.Meta, error) {
	if p == vfs.Root {
		return vfs.Meta{}, vfs.ErrInvalidPath
	}
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	var im internalMeta
	var internalErr error
	err := bvfs.db.View(func(txn *badger.Txn) error {
		im, internalErr = findByPath(txn, p)
		if internalErr != nil {
			return internalErr
		}
		return nil
	})

	return im.Meta, err
}

func (bvfs *BadgerVFS) Stat(id uuid.UUID) (vfs.Meta, error) {
	var path string
	var im internalMeta
	err := bvfs.db.View(func(txn *badger.Txn) error {
		idBytes, internalErr := id.MarshalBinary()
		if internalErr != nil {
			return internalErr
		}
		indexKey := makeKey(prefixIndex, idBytes)
		pathItem, internalErr := txn.Get(indexKey)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		internalErr = pathItem.Value(func(val []byte) error {
			path = string(val)
			return nil
		})
		if internalErr != nil {
			return internalErr
		}

		metaKey := makeKey(prefixMeta, []byte(path))
		item, internalErr := txn.Get(metaKey)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}
		im, internalErr = getMeta(item)
		return internalErr
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("Path", im.Path).Msg("Stat")

	return im.Meta, nil
}

func (bvfs *BadgerVFS) Move(id uuid.UUID, dst string) (vfs.Meta, error) {
	dst = strings.TrimSpace(dst)
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	var im internalMeta
	err := bvfs.db.Update(func(txn *badger.Txn) error {
		item, internalErr := findMetaItemByID(txn, id)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		im, internalErr = getMeta(item)
		if internalErr != nil {
			return internalErr
		}

		if im.IsDir && !strings.HasSuffix(dst, "/") {
			dst += "/"
		}

		// Check if destination already exists
		dstMetaKey := makeKey(prefixMeta, []byte(dst))
		if _, err := txn.Get(dstMetaKey); err == nil {
			return vfs.ErrAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		oldPath := im.Path

		// Move the directory/file itself
		if err := moveItem(txn, &im, dst); err != nil {
			return err
		}

		// If directory, move children recursively
		if im.IsDir {
			if err := moveChildren(txn, oldPath, dst); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", im.ID.String()).Str("OldPath", im.Path).Str("NewPath", dst).Msg("Move")

	return im.Meta, nil
}

func (bvfs *BadgerVFS) Copy(id uuid.UUID, dst string) (vfs.Meta, error) {
	var im internalMeta
	var newIM internalMeta

	dst = strings.TrimSpace(dst)
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	err := bvfs.db.Update(func(txn *badger.Txn) error {
		item, internalErr := findMetaItemByID(txn, id)
		if internalErr != nil {
			if errors.Is(internalErr, badger.ErrKeyNotFound) {
				return vfs.ErrNotFound
			}
			return internalErr
		}

		im, internalErr = getMeta(item)
		if internalErr != nil {
			return internalErr
		}

		idBytes, internalErr := im.InternalID.MarshalBinary()
		if internalErr != nil {
			return internalErr
		}

		// Read all chunks from source (using InternalID)
		reader := &blobReader{
			vfs:  bvfs,
			id:   idBytes,
			size: im.Size,
		}

		// 새로운 ID로 복제

		newMetaID, err := uuid.NewRandom()
		if err != nil {
			return err
		}

		newInternalID, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		// New internal ID for the Copy
		newInternalIDBytes, _ := newInternalID.MarshalBinary()

		newIM = im
		newIM.ID = newMetaID
		newIM.Path = dst
		newIM.Name = filepath.Base(strings.TrimSuffix(dst, "/"))
		newIM.Modified = time.Now()
		newIM.InternalID = newInternalID

		metaKey := makeKey(prefixMeta, []byte(dst))
		if setMetaErr := setMeta(txn, metaKey, &newIM); setMetaErr != nil {
			return setMetaErr
		}

		newIdBytes, internalErr := newIM.ID.MarshalBinary()
		if internalErr != nil {
			return internalErr
		}

		if _, err := bvfs.writeChunks(newInternalIDBytes, reader); err != nil {
			return err
		}

		newIndexKey := makeKey(prefixIndex, newIdBytes)
		if internalErr := txn.Set(newIndexKey, []byte(dst)); internalErr != nil {
			return internalErr
		}
		return nil
	})
	if err != nil {
		return vfs.Meta{}, err
	}

	bvfs.logger.Debug().Str("ID", newIM.ID.String()).Str("Path", newIM.Path).Msg("Copy")

	return newIM.Meta, nil
}

func (bvfs *BadgerVFS) Close() error {
	bvfs.logger.Info().Msg("Closing Badger VFS")

	// Cancel the context to stop background GC goroutines
	bvfs.cancel()

	var err error
	badgerErr := bvfs.db.Close()
	if badgerErr != nil {
		bvfs.logger.Error().Err(badgerErr).Msg("Close Badger VFS error")
		err = errors.Join(err, badgerErr)
	}
	closeLogErr := bvfs.logger.Close()
	if closeLogErr != nil {
		err = errors.Join(err, closeLogErr)
	}
	return err
}

func (bvfs *BadgerVFS) Backup(w io.Writer, since uint64) (uint64, error) {
	return bvfs.db.Backup(w, since)
}

func (bvfs *BadgerVFS) Load(r io.Reader, maxPendingWrites int) error {
	return bvfs.db.Load(r, maxPendingWrites)
}

func (bvfs *BadgerVFS) Tree(targetPath string) (*vfs.TreeNode, error) {
	if targetPath == "" {
		targetPath = vfs.Root
	}
	if !strings.HasPrefix(targetPath, "/") {
		targetPath = "/" + targetPath
	}
	if !strings.HasSuffix(targetPath, "/") {
		targetPath += "/"
	}
	targetPath = strings.TrimSpace(targetPath)

	var rootNode *vfs.TreeNode
	nodes := make(map[string]*vfs.TreeNode)
	if targetPath == vfs.Root {
		// 1. 가상 루트 우선 생성 (DB에 / 엔트리가 없어도 응답 가능하도록 함)
		rootNode = &vfs.TreeNode{
			Meta: vfs.Meta{
				Path:  targetPath,
				Name:  filepath.Base(strings.TrimSuffix(targetPath, "/")),
				IsDir: true,
			},
			Children: nil,
		}
		nodes[vfs.Root] = rootNode
	}

	err := bvfs.db.View(func(txn *badger.Txn) error {
		// "meta:/path/"로 시작하는 모든 항목 스캔
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		metaKey := makeKey(prefixMeta, []byte(targetPath))
		for it.Seek(metaKey); it.ValidForPrefix(metaKey); it.Next() {
			var im internalMeta
			// Tree doesn't need internalID
			im, internalErr := getMeta(it.Item())
			if internalErr != nil {
				continue
			}

			node := &vfs.TreeNode{
				Meta:     im.Meta,
				Children: nil,
			}
			nodes[im.Path] = node

			// 1. 루트 노드 결정
			if im.Path == targetPath {
				rootNode = node
				continue
			}

			// 2. 부모 경로 찾기 (디렉토리 접미사 / 고려)
			parentPath := getParentPath(im.Path)

			// 3. 연결 시도
			if parent, exists := nodes[parentPath]; exists {
				parent.Children = append(parent.Children, node)
			} else if rootNode != nil {
				// 부모가 맵에 없으면(예: targetPath의 바로 아래 자식들)
				// rootNode에 직접 연결
				rootNode.Children = append(rootNode.Children, node)
			}
		}
		return nil
	})

	if rootNode == nil {
		return nil, vfs.ErrNotFound
	}

	return rootNode, err
}

func (bvfs *BadgerVFS) Rotate(newKey []byte) error {
	opt := badger.KeyRegistryOptions{
		Dir:                           bvfs.path,
		ReadOnly:                      true, // Open for reading first
		EncryptionKey:                 bvfs.key,
		EncryptionKeyRotationDuration: bvfs.keyRotationDuration,
	}

	kr, err := badger.OpenKeyRegistry(opt)
	if err != nil {
		return err
	}
	defer kr.Close()

	// Update key in options for writing
	opt.EncryptionKey = newKey
	err = badger.WriteKeyRegistry(kr, opt)
	if err != nil {
		return err
	}

	// Update current key in struct
	bvfs.key = newKey
	return nil
}

// Badger manage

// AllKeys returns all keys in the database
func (bvfs *BadgerVFS) AllKeys() ([]string, error) {
	keys := make([]string, 0)
	err := bvfs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

// AllKeysByPrefix returns all keys with the given prefix
func (bvfs *BadgerVFS) AllKeysByPrefix(prefix string) ([]string, error) {
	keys := make([]string, 0)
	err := bvfs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			item := it.Item()
			key := string(item.Key())
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

func (bvfs *BadgerVFS) runGC(gcInterval time.Duration, gcRatio float64) {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()

	bvfs.logger.Debug().Msg("started GC ticker")

	for {
		select {
		case <-bvfs.ctx.Done():
			bvfs.logger.Debug().Msg("stopped GC ticker")
			return
		case <-ticker.C:
			err := bvfs.db.RunValueLogGC(gcRatio)
			if errors.Is(err, badger.ErrNoRewrite) {
				bvfs.logger.Debug().Msg("no need to run GC")
			} else if err != nil {
				bvfs.logger.Error().Err(err).Msg("failed to run GC")
			} else {
				bvfs.logger.Debug().Msg("completed run GC")
			}
		}
	}
}

type internalMeta struct {
	vfs.Meta
	InternalID uuid.UUID
}

type blobReader struct {
	vfs            *BadgerVFS
	id             []byte
	size           int64
	offset         int64
	cachedChunk    []byte
	cachedChunkIdx uint32
}

func (br *blobReader) Read(p []byte) (n int, err error) {
	if br.offset >= br.size {
		return 0, io.EOF
	}

	toRead := int64(len(p))
	if br.offset+toRead > br.size {
		toRead = br.size - br.offset
	}

	read := 0
	for read < int(toRead) {
		chunkIdxInt64 := br.offset / ChunkSize
		// overflow check
		if chunkIdxInt64 < 0 || chunkIdxInt64 > math.MaxUint32 {
			return read, errors.New("badger: offset out of range")
		}
		chunkIdx := uint32(chunkIdxInt64)
		chunkOffset := int(br.offset % ChunkSize)

		// 캐시된 청크가 없거나 인덱스가 다르면 로드
		if br.cachedChunk == nil || br.cachedChunkIdx != chunkIdx {
			var chunkData []byte
			err := br.vfs.db.View(func(txn *badger.Txn) error {
				key := makeChunkKey(br.id, chunkIdx)
				item, err := txn.Get(key)
				if err != nil {
					return err
				}
				return item.Value(func(val []byte) error {
					chunkData = append([]byte(nil), val...)
					return nil
				})
			})
			if err != nil {
				return read, err
			}
			br.cachedChunk = chunkData
			br.cachedChunkIdx = chunkIdx
		}

		n := copy(p[read:], br.cachedChunk[chunkOffset:])
		read += n
		br.offset += int64(n)
	}
	return read, nil
}

func (br *blobReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = br.offset + offset
	case io.SeekEnd:
		abs = br.size + offset
	default:
		return 0, errors.New("badger: invalid whence")
	}

	if abs < 0 {
		return 0, errors.New("badger: negative position")
	}

	br.offset = abs
	return abs, nil
}

func (br *blobReader) Close() error {
	br.cachedChunk = nil
	return nil
}
