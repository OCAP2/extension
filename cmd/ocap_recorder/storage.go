package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/storage/memory"
	"github.com/OCAP2/extension/v5/internal/storage/websocket"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
)

// startStorage handles :STORAGE:INIT:. It answers with a :STORAGE:OK:
// callback, or :STORAGE:ERROR: so the addon stops waiting for storage.
func startStorage() {
	if err := initStorage(); err != nil {
		Logger.Error("Storage initialization failed", "error", err)
		if cbErr := a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:ERROR:", err.Error()); cbErr != nil {
			Logger.Warn("Failed to send STORAGE:ERROR callback", "error", cbErr)
		}
	}
}

// storageInitMu serializes :STORAGE:INIT:, which the addon sends on every
// mission load.
var storageInitMu sync.Mutex

func initStorage() error {
	Logger.Debug("Received :STORAGE:INIT: call")
	storageInitMu.Lock()
	defer storageInitMu.Unlock()

	storageCfg := config.GetStorageConfig()

	// The backend lives as long as the process: the next mission reuses it,
	// so a mission the game moved on from is ended by the next StartMission
	// instead of staying open in a backend nothing uses any more.
	if storageBackend != nil {
		Logger.Info("Storage already initialized; reusing it", "type", storageCfg.Type)
		if err := a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:OK:", storageCfg.Type); err != nil {
			Logger.Warn("Failed to send STORAGE:OK callback", "error", err)
		}
		return nil
	}

	backend, err := createStorageBackend(storageCfg)
	if err != nil {
		Logger.Error("Failed to create storage backend", "error", err)
		return err
	}
	if err := backend.Init(); err != nil {
		Logger.Error("Failed to initialize storage backend", "error", err)
		return err
	}
	storageBackend = backend

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
		api := config.GetAPIConfig()
		if strings.TrimSpace(api.APIKey) == "" {
			return nil, fmt.Errorf("websocket storage requires api.apiKey")
		}
		ws := storageCfg.Websocket
		resendRate := ws.ResendMBps << 20
		if ws.ResendMBps <= 0 {
			resendRate = -1 // unlimited
		}
		return websocket.New(websocket.Config{
			URL:               httpToWS(api.ServerURL) + "/api/v1/stream",
			Secret:            api.APIKey,
			SaveTimeout:       api.UploadTimeout,
			BufferBytes:       ws.BufferMB << 20,
			SpoolDir:          ws.SpoolDir,
			SpoolMaxBytes:     int64(ws.SpoolMaxMB) << 20,
			ResendBytesPerSec: resendRate,
		}), nil

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
