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

// internalMeta is a copy of the unexported internalMeta struct from drivers/badger.
type internalMeta struct {
	vfs.Meta
	InternalID uuid.UUID `json:"internalId,omitempty"`
}

// fallbackMeta allows us to capture both the old and new JSON keys
// because uuid.UUID custom unmarshaller skips standard case-insensitive fallback.
type fallbackMeta struct {
	vfs.Meta
	InternalIDv1 uuid.UUID `json:"InternalID"`
	InternalIDv2 uuid.UUID `json:"internalId"`
}

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
					return nil // Skip this record, try the next
				}

				// If internalId wasn't found, use the old InternalID
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

				// Only write to DB if the JSON bytes have changed
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
