package main

import (
	"fmt"
	"path/filepath"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/storage"
	pgstorage "github.com/OCAP2/extension/v5/internal/storage/postgres"
	"github.com/OCAP2/extension/v5/internal/storage/memory"
	sqlitestorage "github.com/OCAP2/extension/v5/internal/storage/sqlite"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
)

func initStorage() error {
	Logger.Debug("Received :INIT:STORAGE: call")

	storageCfg := config.GetStorageConfig()

	backend, err := createStorageBackend(storageCfg)
	if err != nil {
		Logger.Error("Failed to create storage backend", "error", err)
		return err
	}
	storageBackend = backend
	if err := storageBackend.Init(); err != nil {
		Logger.Error("Failed to initialize storage backend", "error", err)
		return err
	}

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
	if err := a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:OK:", storageCfg.Type); err != nil {
		Logger.Warn("Failed to send STORAGE:OK callback", "error", err)
	}
	storageReadyOnce.Do(func() { close(storageReady) })
	return nil
}

func createStorageBackend(storageCfg config.StorageConfig) (storage.Backend, error) {
	switch storageCfg.Type {
	case "postgres":
		Logger.Info("Postgres storage backend initialized")
		return pgstorage.New(pgstorage.Dependencies{
			EntityCache: EntityCache,
			MarkerCache: MarkerCache,
			LogManager:  SlogManager,
		}), nil

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

	default:
		Logger.Info("Memory storage backend initialized")
		return memory.New(storageCfg.Memory), nil
	}
}
