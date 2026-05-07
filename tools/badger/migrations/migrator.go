package migrations

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// Migrator는 데이터베이스 마이그레이션을 수행하기 위한 인터페이스입니다.
type Migrator interface {
	Migrate(db *badger.DB) error
}

// Migrate는 주어진 하나 이상의 Migrator들을 순차적으로 실행하여 데이터베이스 마이그레이션을 수행합니다.
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
