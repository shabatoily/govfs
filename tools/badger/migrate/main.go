package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	badgervfs "github.com/meteormin/govfs/drivers/badger"
	"github.com/meteormin/govfs/tools/badger/migrations"
)

var migrators = []migrations.Migrator{
	migrations.InternalIDMigrator{},
}

func main() {
	fmt.Println("Migrate badgerDB")

	args := os.Args
	if len(args) < 2 {
		fmt.Println("Usage: badger_migrate <path>")
		return
	}

	path := args[1]

	secretFile, err := filepath.Abs(filepath.Join(path, badgervfs.DefaultSecretFilename))
	if err != nil {
		fmt.Println("Failed to get absolute path of secret file: ", err)
		return
	}
	//nolint:gosec // intentional for CLI tool
	secretKey, err := os.ReadFile(secretFile)
	if err != nil {
		fmt.Println("Failed to read secret file: ", err)
		return
	}

	opts := badger.DefaultOptions(path).
		WithEncryptionKey(secretKey).
		WithIndexCacheSize(badgervfs.DefaultIndexCacheSize)

	db, err := badger.Open(opts)
	if err != nil {
		fmt.Println("Failed to open badger db: ", err)
		return
	}
	defer db.Close()

	err = migrations.Migrate(db, migrators...)
	if err != nil {
		fmt.Println("Failed to migrate badger db: ", err)
		return
	}

	fmt.Println("Migrate badger db success")
}
