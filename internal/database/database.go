package database

import (
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GetSqliteDBStandalone returns a connection to a SQLite database.
// If path is empty, uses an in-memory database.
func GetSqliteDBStandalone(path string) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	if path != "" {
		db, err = gorm.Open(sqlite.Open(path), &gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
			CreateBatchSize:        2000,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return nil, err
		}
	} else {
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
			CreateBatchSize:        2000,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return nil, err
		}
	}

	// set PRAGMAS
	pragmas := []string{
		"PRAGMA user_version = 1;",
		"PRAGMA journal_mode = MEMORY;",
		"PRAGMA synchronous = OFF;",
		"PRAGMA cache_size = -32000;",
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA page_size = 32768;",
		"PRAGMA mmap_size = 30000000000;",
	}

	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			return nil, fmt.Errorf("error setting PRAGMA: %s", err)
		}
	}

	return db, nil
}

// DumpMemoryDBToDisk vacuums the in-memory database to a disk file.
func DumpMemoryDBToDisk(db *gorm.DB, sqliteFilePath string) error {
	if sqliteFilePath == "" {
		return fmt.Errorf("sqlite file path not set")
	}

	// remove existing file if it exists
	if exists, err := os.Stat(sqliteFilePath); err == nil && exists != nil {
		if err := os.Remove(sqliteFilePath); err != nil {
			return fmt.Errorf("error removing existing DB file: %s", err)
		}
	}

	err := db.Exec("VACUUM INTO 'file:" + sqliteFilePath + "';").Error
	if err != nil {
		return fmt.Errorf("error dumping memory DB to disk: %s", err)
	}

	return nil
}
