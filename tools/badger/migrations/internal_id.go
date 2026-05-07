package migrations

import (
	"bytes"
	"fmt"
	"log"

	"github.com/dgraph-io/badger/v4"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
)

// internalMeta는 drivers/badger 패키지의 비공개 구조체 internalMeta의 복사본입니다.
type internalMeta struct {
	vfs.Meta
	InternalID uuid.UUID `json:"internalId,omitempty"`
}

// fallbackMeta는 uuid.UUID의 커스텀 Unmarshal이 표준 대소문자 무시 대체 동작을 건너뛰기 때문에,
// 이전 JSON 키와 새로운 JSON 키를 모두 캡처할 수 있도록 해줍니다.
type fallbackMeta struct {
	vfs.Meta
	InternalIDv1 uuid.UUID `json:"InternalID"`
	InternalIDv2 uuid.UUID `json:"internalId"`
}

// InternalIDMigrator는 메타데이터의 InternalID 필드 마이그레이션을 처리합니다.
type InternalIDMigrator struct{}

func (InternalIDMigrator) Migrate(db *badger.DB) error {
	prefixMeta := []byte("meta:")
	migratedCount := 0

	err := db.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefixMeta); it.ValidForPrefix(prefixMeta); it.Next() {
			item := it.Item()
			key := item.KeyCopy(nil)

			valErr := item.Value(func(v []byte) error {
				var fallback fallbackMeta
				if unmarshalErr := json.Unmarshal(v, &fallback); unmarshalErr != nil {
					log.Printf("Failed to unmarshal meta into struct for key %s: %v", string(key), unmarshalErr)
					return nil // 현재 레코드를 건너뛰고 다음 레코드로 진행
				}

				// internalId를 찾지 못했다면, 예전 방식의 InternalID를 사용합니다.
				targetUUID := fallback.InternalIDv2
				if targetUUID == uuid.Nil {
					targetUUID = fallback.InternalIDv1
				}

				im := internalMeta{
					Meta:       fallback.Meta,
					InternalID: targetUUID,
				}

				newBytes, marshalErr := json.Marshal(&im)
				if marshalErr != nil {
					log.Printf("Failed to marshal migrated meta for key %s: %v", string(key), marshalErr)
					return nil
				}

				// JSON 바이트가 변경된 경우에만 DB에 덮어씁니다.
				if !bytes.Equal(v, newBytes) {
					if setErr := txn.Set(key, newBytes); setErr != nil {
						return fmt.Errorf("failed to set new meta for key %s: %w", string(key), setErr)
					}
					migratedCount++
					log.Printf("Migrated meta record for path: %s", im.Path)
				}

				return nil
			})
			if valErr != nil {
				return valErr
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("migration transaction failed: %w", err)
	}

	log.Printf("Migration complete. Total migrated records: %d", migratedCount)
	return nil
}
