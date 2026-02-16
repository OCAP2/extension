package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/storage/memory"
	pgstorage "github.com/OCAP2/extension/v5/internal/storage/postgres"
	sqlitestorage "github.com/OCAP2/extension/v5/internal/storage/sqlite"
	wsstorage "github.com/OCAP2/extension/v5/internal/storage/websocket"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
	"github.com/spf13/viper"
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

	case "websocket":
		wsURL := httpToWS(viper.GetString("api.serverUrl")) + "/api"
		secret := viper.GetString("api.apiKey")
		Logger.Info("WebSocket storage backend initialized", "url", wsURL)
		return wsstorage.New(wsstorage.Config{
			URL:    wsURL,
			Secret: secret,
		}), nil

	default:
		Logger.Info("Memory storage backend initialized")
		return memory.New(storageCfg.Memory), nil
	}
}

// httpToWS converts an HTTP(S) URL to a WebSocket URL.
func httpToWS(httpURL string) string {
	s := strings.TrimRight(httpURL, "/")
	s = strings.Replace(s, "https://", "wss://", 1)
	s = strings.Replace(s, "http://", "ws://", 1)
	return s
}
