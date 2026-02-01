package database

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Manager handles database connections and operations.
type Manager struct {
	DB              *gorm.DB
	SqlDB           *sql.DB
	IsValid         bool
	ShouldSaveLocal bool
	SqliteFilePath  string
	Logger          zerolog.Logger
}

// NewManager creates a new database manager.
func NewManager(log zerolog.Logger) *Manager {
	return &Manager{
		IsValid:         false,
		ShouldSaveLocal: false,
		Logger:          log,
	}
}

// Connect establishes a database connection, falling back to SQLite if Postgres fails.
func (m *Manager) Connect() error {
	var err error

	m.DB, err = m.GetPostgresDB()
	if err != nil {
		m.Logger.Error().Err(err).Msg("Failed to connect to Postgres DB, trying SQLite")
		m.ShouldSaveLocal = true
		m.DB, err = m.GetSqliteDB("")
		if err != nil || m.DB == nil {
			m.IsValid = false
			return fmt.Errorf("failed to get local SQLite DB: %s", err)
		}
		m.IsValid = true
	}

	// test connection
	m.SqlDB, err = m.DB.DB()
	if err != nil {
		return fmt.Errorf("failed to access sql interface: %s", err)
	}

	err = m.SqlDB.Ping()
	if err != nil {
		m.Logger.Error().Err(err).Msg("Failed to validate connection, trying SQLite")
		m.ShouldSaveLocal = true
		m.DB, err = m.GetSqliteDB("")
		if err != nil || m.DB == nil {
			m.IsValid = false
			return fmt.Errorf("failed to get local SQLite DB: %s", err)
		}
		m.IsValid = true
	} else {
		m.Logger.Info().Msg("Connected to database")
		m.IsValid = true
	}

	if !m.IsValid {
		return fmt.Errorf("db not valid. not saving")
	}

	if !m.ShouldSaveLocal {
		m.SqlDB.SetMaxOpenConns(10)
	}

	return nil
}

// GetPostgresDB returns a connection to the Postgres database.
func (m *Manager) GetPostgresDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable`,
		viper.GetString("db.host"),
		viper.GetString("db.port"),
		viper.GetString("db.username"),
		viper.GetString("db.password"),
		viper.GetString("db.database"),
	)

	m.Logger.Debug().Msgf("Connecting to Postgres DB at '%s'", dsn)

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
		CreateBatchSize:        10000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// GetSqliteDB returns a connection to a SQLite database.
// If path is empty, uses an in-memory database.
func (m *Manager) GetSqliteDB(path string) (*gorm.DB, error) {
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
			m.IsValid = false
			return nil, err
		}
		m.Logger.Info().Str("path", path).Msg("Using local SQLite DB")
	} else {
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
			CreateBatchSize:        2000,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			m.IsValid = false
			return nil, err
		}
		m.Logger.Info().Msg("Using local SQLite DB in memory with periodic disk dump")
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

// Setup migrates tables and creates default settings if they don't exist.
func (m *Manager) Setup() error {
	// Check if OcapInfo table exists
	if !m.DB.Migrator().HasTable(&model.OcapInfo{}) {
		err := m.DB.AutoMigrate(&model.OcapInfo{})
		if err != nil {
			m.IsValid = false
			return fmt.Errorf("failed to create ocap_info table: %s", err)
		}
		err = m.DB.Create(&model.OcapInfo{
			GroupName:        "OCAP",
			GroupDescription: "OCAP",
			GroupLogo:        "https://i.imgur.com/0Q4z0ZP.png",
			GroupWebsite:     "https://ocap.arma3.com",
		}).Error
		if err != nil {
			m.IsValid = false
			return fmt.Errorf("failed to create ocap_info entry: %s", err)
		}
	}

	// Ensure PostGIS Extension is installed for Postgres
	if m.DB.Dialector.Name() == "postgres" {
		err := m.DB.Exec(`CREATE Extension IF NOT EXISTS postgis;`).Error
		if err != nil {
			m.IsValid = false
			return fmt.Errorf("failed to create PostGIS Extension: %s", err)
		}
		m.Logger.Info().Msg("PostGIS Extension created")
	}

	m.Logger.Info().Msg("Migrating schema")
	var err error
	if m.ShouldSaveLocal {
		err = m.DB.AutoMigrate(model.DatabaseModelsSQLite...)
	} else {
		err = m.DB.AutoMigrate(model.DatabaseModels...)
	}
	if err != nil {
		m.IsValid = false
		return fmt.Errorf("failed to migrate schema: %s", err)
	}

	m.Logger.Info().Msg("Database setup complete")
	return nil
}

// DumpMemoryToDisk vacuums the in-memory database to a file.
func (m *Manager) DumpMemoryToDisk() error {
	if m.SqliteFilePath == "" {
		return fmt.Errorf("sqlite file path not set")
	}

	// remove existing file if it exists
	if exists, err := os.Stat(m.SqliteFilePath); err == nil && exists != nil {
		if err := os.Remove(m.SqliteFilePath); err != nil {
			return fmt.Errorf("error removing existing DB file: %s", err)
		}
	}

	start := time.Now()
	err := m.DB.Exec("VACUUM INTO 'file:" + m.SqliteFilePath + "';").Error
	if err != nil {
		return fmt.Errorf("error dumping memory DB to disk: %s", err)
	}

	m.Logger.Debug().Dur("duration", time.Since(start)).Msg("Dumped memory DB to disk")
	return nil
}

// GetBackupDBPaths returns paths to all .db files in the given directory.
func GetBackupDBPaths(addonFolder string) ([]string, error) {
	files, err := os.ReadDir(addonFolder)
	if err != nil {
		return nil, err
	}

	var dbPaths []string
	for _, file := range files {
		if !file.IsDir() && len(file.Name()) > 3 && file.Name()[len(file.Name())-3:] == ".db" {
			dbPaths = append(dbPaths, addonFolder+"/"+file.Name())
		}
	}
	return dbPaths, nil
}

// Standalone functions for direct usage without Manager

// GetPostgresDBStandalone returns a connection to the Postgres database using viper config.
func GetPostgresDBStandalone() (*gorm.DB, error) {
	dsn := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable`,
		viper.GetString("db.host"),
		viper.GetString("db.port"),
		viper.GetString("db.username"),
		viper.GetString("db.password"),
		viper.GetString("db.database"),
	)

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
		CreateBatchSize:        10000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

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
