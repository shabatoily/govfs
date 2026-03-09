package migrations

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

type Migrator interface {
	Migrate(db *badger.DB) error
}

func Migrate(db *badger.DB, migrators ...Migrator) error {
	for _, migrator := range migrators {
		fmt.Printf("Migrating %T\n", migrator)
		if err := migrator.Migrate(db); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", migrator, err)
		}
		fmt.Printf("Success to migrate %T\n", migrator)
	}
	return nil
}
