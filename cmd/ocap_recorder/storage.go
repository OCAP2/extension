package main

import (
	"fmt"
	"path/filepath"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/database"
	"github.com/OCAP2/extension/v5/internal/storage"
	gormstorage "github.com/OCAP2/extension/v5/internal/storage/gorm"
	"github.com/OCAP2/extension/v5/internal/storage/memory"
	sqlitestorage "github.com/OCAP2/extension/v5/internal/storage/sqlite"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"

	"gorm.io/gorm"
)

func initStorage() error {
	Logger.Debug("Received :INIT:STORAGE: call")

	storageCfg := config.GetStorageConfig()

	// Create DB connection for postgres mode
	if storageCfg.Type == "postgres" || storageCfg.Type == "gorm" || storageCfg.Type == "database" {
		var err error
		DB, err = getPostgresDB()
		if err != nil {
			SlogManager.WriteLog(":INIT:STORAGE:", fmt.Sprintf(`Error connecting to database: %v`, err), "ERROR")
			a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:ERROR:", err.Error())
			return err
		}
		if DB == nil {
			err = fmt.Errorf("database connection is nil")
			SlogManager.WriteLog(":INIT:STORAGE:", err.Error(), "ERROR")
			a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:ERROR:", err.Error())
			return err
		}
	}

	// Create storage backend — DB is now guaranteed to be set for postgres
	backend, err := createStorageBackend(storageCfg)
	if err != nil {
		Logger.Error("Failed to create storage backend", "error", err)
		return err
	}
	storageBackend = backend
	storageBackend.Init()

	// Initialize worker manager
	workerManager = worker.NewManager(worker.Dependencies{
		EntityCache:   EntityCache,
		MarkerCache:   MarkerCache,
		LogManager:    SlogManager,
		ParserService: parserService,
	}, storageBackend)

	// Register worker handlers with the dispatcher
	Logger.Debug("Registering worker handlers with dispatcher")
	workerManager.RegisterHandlers(eventDispatcher)
	Logger.Info("Worker handlers registered with dispatcher")

	// Signal storage ready
	a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:OK:", storageCfg.Type)
	storageReadyOnce.Do(func() { close(storageReady) })
	return nil
}

func getPostgresDB() (db *gorm.DB, err error) {
	Logger.Debug("Connecting to Postgres DB")
	db, err = database.GetPostgresDBStandalone()
	if err != nil {
		return nil, err
	}

	// Test connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to access sql interface: %w", err)
	}
	if err = sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to validate connection: %w", err)
	}

	sqlDB.SetMaxOpenConns(10)
	SlogManager.WriteLog(":INIT:DB:", "Connected to Postgres database", "INFO")
	return db, nil
}

func createStorageBackend(storageCfg config.StorageConfig) (storage.Backend, error) {
	switch storageCfg.Type {
	case "memory":
		Logger.Info("Memory storage backend initialized")
		return memory.New(storageCfg.Memory), nil

	case "sqlite":
		sqliteDBFilePath := filepath.Join(AddonFolder, fmt.Sprintf("%s_%s.db", ExtensionName, SessionStartTime.Format("20060102_150405")))
		backend, err := sqlitestorage.New(sqlitestorage.Config{
			DumpInterval: storageCfg.SQLite.DumpInterval,
			DumpPath:     sqliteDBFilePath,
		}, EntityCache, MarkerCache, SlogManager)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite backend: %w", err)
		}
		Logger.Info("SQLite storage backend initialized")
		return backend, nil

	default: // postgres, gorm, database
		Logger.Info("GORM storage backend initialized")
		return gormstorage.New(gormstorage.Dependencies{
			DB:          DB,
			EntityCache: EntityCache,
			MarkerCache: MarkerCache,
			LogManager:  SlogManager,
		}), nil
	}
}
