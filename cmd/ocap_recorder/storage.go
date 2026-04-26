package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OCAP2/extension/v5/internal/api"
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
	Logger.Debug("Received :STORAGE:INIT: call")

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
	case "memory":
		Logger.Info("Memory storage backend initialized")
		return memory.New(storageCfg.Memory, Logger), nil

	case "postgres":
		return nil, fmt.Errorf("postgres storage type not fully supported yet")

	case "sqlite":
		return nil, fmt.Errorf("sqlite storage type not fully supported yet")

	case "websocket":
		return nil, fmt.Errorf("websocket storage type not fully supported yet")

	default:
		return nil, fmt.Errorf("unknown storage type %q", storageCfg.Type)
	}
}

// httpToWS converts an HTTP(S) URL to a WebSocket URL.
func httpToWS(httpURL string) string {
	s := strings.TrimRight(httpURL, "/")
	s = strings.Replace(s, "https://", "wss://", 1)
	s = strings.Replace(s, "http://", "ws://", 1)
	return s
}
