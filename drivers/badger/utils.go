package badger

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-json"
	vfs "github.com/meteormin/govfs"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

const (
	secretFileMode = 0600
	secretDirMode  = 0755
	chunkSeqLen    = 4
)

// GenerateEncryptionKey는 지정된 파일에 랜덤 암호화 키를 생성하고 저장합니다.
func GenerateEncryptionKey(secretFile string, keySize int) ([]byte, error) {
	key, err := randomSecretKey(keySize)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(secretFile), secretDirMode)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(secretFile, key, secretFileMode)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// randomSecretKey는 지정된 길이의 무작위 바이트 슬라이스를 생성합니다.
func randomSecretKey(keySize int) ([]byte, error) {
	key := make([]byte, keySize) // AES-256에 필요한 32바이트 키
	_, err := rand.Read(key)
	return key, err
}

// getEncryptionKey는 파일에서 암호화 키를 읽어옵니다.
func getEncryptionKey(secretFile string) ([]byte, error) {
	if _, err := os.Stat(secretFile); err != nil {
		return nil, err
	}

	key, err := os.ReadFile(secretFile)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// extractExtension은 경로에서 파일 확장자를 추출합니다.
func extractExtension(path string) string {
	ext := filepath.Ext(path)
	if ext != "" {
		return strings.ToLower(strings.TrimPrefix(ext, "."))
	}
	return ""
}

// 경로 문자열에서 부모 경로를 찾는 헬퍼 함수
// getParentPath는 지정된 경로의 부모 디렉토리 경로를 계산하여 반환합니다.
func getParentPath(path string) string {
	if path == vfs.Root || path == "" {
		return vfs.Root
	}

	// 1. 마지막 슬래시가 경로의 맨 끝이라면 제거하고 탐색 (디렉토리 대응)
	// 예: "/test/" -> "/test"
	p := strings.TrimSuffix(path, "/")

	// 2. 마지막 슬래시 위치 찾기
	lastIndex := strings.LastIndex(p, "/")

	// 3. 부모 경로 결정
	if lastIndex < 0 {
		return vfs.Root // 부모가 없음 (이런 경우는 VFS 구조상 드묾)
	}

	// lastIndex가 0이면(예: "/test"), 부모는 바로 "/"
	if lastIndex == 0 {
		return vfs.Root
	}

	// 그 외에는 마지막 슬래시를 포함한 위치까지 자름
	// 예: "/test/abc.txt" -> "/test/"
	return p[:lastIndex+1]
}

// 키 생성을 안전하게 처리하는 헬퍼
// makeKey는 접두사와 접미사를 결합하여 BadgerDB용 키를 생성합니다.
func makeKey(prefix, suffix []byte) []byte {
	k := make([]byte, len(prefix)+len(suffix))
	copy(k, prefix)
	copy(k[len(prefix):], suffix)
	return k
}

// makeChunkKey는 데이터 청크 저장을 위한 고유 키를 생성합니다.
func makeChunkKey(id []byte, seq uint32) []byte {
	k := make([]byte, len(prefixBlob)+len(id)+chunkSeqLen)
	copy(k, prefixBlob)
	copy(k[len(prefixBlob):], id)
	binary.BigEndian.PutUint32(k[len(prefixBlob)+len(id):], seq)
	return k
}

// setMeta는 지정된 트랜잭션을 통해 메타데이터를 저장합니다.
func setMeta(txn *badger.Txn, metaKey []byte, im *internalMeta) error {
	jsonByte, err := json.Marshal(im)
	if err != nil {
		return err
	}
	return txn.Set(metaKey, jsonByte)
}

// getMeta는 Badger 항목에서 메타데이터를 추출합니다.
func getMeta(item *badger.Item) (internalMeta, error) {
	var im internalMeta
	err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &im)
	})
	if err != nil {
		return internalMeta{}, err
	}

	return im, nil
}

// findMetaItemByID는 ID를 기반으로 해당 항목의 메타데이터 Badger 항목을 찾습니다.
func findMetaItemByID(txn *badger.Txn, id uuid.UUID) (*badger.Item, error) {
	idBytes, err := id.MarshalBinary()
	if err != nil {
		return nil, err
	}

	indexKey := makeKey(prefixIndex, idBytes)
	pathItem, err := txn.Get(indexKey)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, vfs.ErrNotFound
		}
		return nil, err
	}

	var path string
	err = pathItem.Value(func(val []byte) error {
		path = string(val)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return txn.Get([]byte("meta:" + path))
}

// findByPath는 경로를 기반으로 메타데이터를 검색합니다. (디렉토리 경로 매칭 포함)
func findByPath(txn *badger.Txn, path string) (internalMeta, error) {
	if path == vfs.Root {
		return internalMeta{}, vfs.ErrNotFound
	}

	metaKey := makeKey(prefixMeta, []byte(path))
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(metaKey); it.ValidForPrefix(metaKey); it.Next() {
		var im internalMeta
		item := it.Item()
		key := item.Key()

		err := item.Value(func(val []byte) error {
			if err := json.Unmarshal(val, &im); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return internalMeta{}, err
		}

		if bytes.Equal(key, metaKey) {
			return im, nil
		}

		// Check directory match (path/ vs path)
		// We want to match if key is exactly metaKey + "/"
		// metaKey is "meta:path". key is "meta:path/"
		if len(key) == len(metaKey)+1 && key[len(key)-1] == '/' && bytes.Equal(key[:len(metaKey)], metaKey) {
			return im, nil
		}
	}

	return internalMeta{}, vfs.ErrNotFound
}

// deleteIndex는 메타데이터 ID 검색용 인덱스를 삭제합니다.
func deleteIndex(txn *badger.Txn, meta *vfs.Meta) error {
	// Delete Index
	idBytes, marshalBinErr := meta.ID.MarshalBinary()
	if marshalBinErr != nil {
		return marshalBinErr
	}
	idxKey := makeKey(prefixIndex, idBytes)
	if err := txn.Delete(idxKey); err != nil {
		return err
	}
	return nil
}

// deleteChunks는 저정된 ID와 연결된 모든 데이터 청크를 삭제합니다.
func deleteChunks(txn *badger.Txn, id []byte) error {
	prefix := make([]byte, len(prefixBlob)+len(id))
	copy(prefix, prefixBlob)
	copy(prefix[len(prefixBlob):], id)

	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	// Collect keys to delete
	var keys [][]byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		keys = append(keys, it.Item().KeyCopy(nil))
	}

	for _, k := range keys {
		if err := txn.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

// deleteItem은 단일 파일 항목(메타데이터, 인덱스, 데이터 청크)을 모두 삭제합니다.
func deleteItem(txn *badger.Txn, im *internalMeta) error {
	// Single file delete
	idBytes, marshalBinErr := im.InternalID.MarshalBinary()
	if marshalBinErr != nil {
		return marshalBinErr
	}
	// Delete chunks
	if err := deleteChunks(txn, idBytes); err != nil {
		return err
	}

	// Delete Index
	if err := deleteIndex(txn, &im.Meta); err != nil {
		return err
	}

	// Delete Meta
	metaKey := makeKey(prefixMeta, []byte(im.Path))
	if err := txn.Delete(metaKey); err != nil {
		return err
	}
	return nil
}

// deleteItems는 여러 항목을 일괄 삭제합니다.
func deleteItems(txn *badger.Txn, items []internalMeta) error {
	for i := range items {
		if !items[i].IsDir {
			if err := deleteItem(txn, &items[i]); err != nil {
				return err
			}
		} else {
			// Delete Index
			if err := deleteIndex(txn, &items[i].Meta); err != nil {
				return err
			}

			// Delete Meta
			mKey := makeKey(prefixMeta, []byte(items[i].Path))
			if err := txn.Delete(mKey); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveItem은 단일 항목의 경로 정보를 업데이트하고 키를 변경합니다.
func moveItem(txn *badger.Txn, im *internalMeta, newPath string) error {
	// 1. Delete old meta key
	oldKey := makeKey(prefixMeta, []byte(im.Path))
	if err := txn.Delete(oldKey); err != nil {
		return err
	}

	// 2. Update meta with new path and time
	im.Path = newPath
	im.Name = filepath.Base(strings.TrimSuffix(newPath, "/"))
	im.Modified = time.Now()
	// 3. Set new meta key
	newKey := makeKey(prefixMeta, []byte(newPath))
	if err := setMeta(txn, newKey, im); err != nil {
		return err
	}

	// 4. Update index (ID -> Path)
	idBytes, err := im.ID.MarshalBinary()
	if err != nil {
		return err
	}
	idxKey := makeKey(prefixIndex, idBytes)
	if err := txn.Set(idxKey, []byte(newPath)); err != nil {
		return err
	}
	return nil
}

// moveChildren은 디렉토리 이동 시 하위 모든 항목들의 경로를 재귀적으로 변경합니다.
func moveChildren(txn *badger.Txn, srcPath, dstPath string) error {
	prefix := srcPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	seekKey := makeKey(prefixMeta, []byte(prefix))

	var children []internalMeta

	// Scan all descendants
	for it.Seek(seekKey); it.ValidForPrefix(seekKey); it.Next() {
		im, err := getMeta(it.Item())
		if err != nil {
			continue
		}
		children = append(children, im)
	}

	// Move children
	for i := range children {
		relPath := strings.TrimPrefix(children[i].Path, prefix)
		newChildPath := filepath.Join(dstPath, relPath)

		// Preserve trailing slash for directories
		if children[i].IsDir && !strings.HasSuffix(newChildPath, "/") {
			newChildPath += "/"
		}

		if err := moveItem(txn, &children[i], newChildPath); err != nil {
			return err
		}
	}
	return nil
}
