package main

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C" // This is required to import the C code

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/OCAP2/extension/v5/internal/api"
	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/logging"
	intOtel "github.com/OCAP2/extension/v5/internal/otel"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"

	"github.com/spf13/viper"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// module defs - can be set at build time via ldflags
var (
	BuildVersion string = "dev"
	BuildCommit  string = "unknown"
	BuildDate    string = "unknown"

	Addon         string = "ocap"
	ExtensionName string = "ocap_recorder"
)

// file paths
var (
	// ArmaDir is the path to the Arma 3 root directory. This is checked in init().
	ArmaDir string

	// AddonFolder is the path to the addon folder. It's coded here to be @ocap, but if the module path is located and isn't the A3 root, we'll use that instead. This allows someone to load the addon from elsewhere on their PC, or use a custom folder name. This is checked in init().
	AddonFolder string

	// ModulePath is the absolute path to this library file.
	ModulePath string

	// ModuleFolder is the parent folder of ModulePath
	ModuleFolder string

	InitLogFilePath string
	InitLogFile     *os.File
	OcapLogFilePath string
	OcapLogFile     *os.File
)

// global variables
var (
	// SlogManager handles all slog-based logging
	SlogManager *logging.SlogManager

	// Logger is the slog logger (convenience reference)
	Logger *slog.Logger

	// OTelProvider handles OpenTelemetry
	OTelProvider *intOtel.Provider

	// EntityCache is a map of all entities in the current mission, used to find associated entities by ocapID for entity state processing
	EntityCache *cache.EntityCache = cache.NewEntityCache()

	// MarkerCache maps marker names to their database IDs for the current mission
	MarkerCache *cache.MarkerCache = cache.NewMarkerCache()

	SessionStartTime time.Time = time.Now()

	addonVersion string = "unknown"

	// storageReady is closed when storage (DB or memory) is initialized and ready
	storageReady     = make(chan struct{})
	storageReadyOnce sync.Once

	// Services
	parserService   parser.Service
	workerManager   *worker.Manager
	eventDispatcher *dispatcher.Dispatcher

	// Storage backend (optional)
	storageBackend storage.Backend

	// API client for OCAP web frontend
	apiClient *api.Client
)

// init is run automatically when the module is loaded
func init() {
	var err error

	ArmaDir, err = a3interface.GetArmaDir()
	if err != nil {
		panic(err)
	}

	ModulePath = a3interface.GetModulePath()
	ModuleFolder = filepath.Dir(ModulePath)

	// if the module dir is not the a3 root, we want to assume the addon folder has been renamed and adjust it accordingly
	AddonFolder = filepath.Dir(ModulePath)

	if AddonFolder == ArmaDir {
		AddonFolder = filepath.Join(ArmaDir, "@"+Addon)
	}

	// check if parent folder exists
	// if it doesn't, create it
	if _, err := os.Stat(AddonFolder); os.IsNotExist(err) {
		if err := os.Mkdir(AddonFolder, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create addon folder: %v\n", err)
		}
	}

	InitLogFilePath = filepath.Join(AddonFolder, "init.log")

	InitLogFile, err = os.Create(InitLogFilePath)
	if err != nil {
		// Log to stderr since logging isn't set up yet
		fmt.Fprintf(os.Stderr, "Failed to create init log file: %v\n", err)
	}

	// Initialize slog manager with initial config
	SlogManager = logging.NewSlogManager()
	SlogManager.Setup(InitLogFile, viper.GetString("logLevel"), nil)
	Logger = SlogManager.Logger()

	// load config
	err = loadConfig()
	if err != nil {
		Logger.Warn("Failed to load config, using defaults!", "error", err)
	} else {
		Logger.Info("Loaded config")
	}

	// resolve path set in config
	// create logs dir if it doesn't exist
	if _, err := os.Stat(viper.GetString("logsDir")); os.IsNotExist(err) {
		if err := os.Mkdir(viper.GetString("logsDir"), 0755); err != nil {
			Logger.Warn("Failed to create logs directory", "error", err)
		}
	}

	OcapLogFilePath = logging.LogFilePath(viper.GetString("logsDir"), ExtensionName, SessionStartTime)

	// check if OcapLogFilePath exists
	// if it does, move it to OcapLogFilePath.old
	// if it doesn't, create it
	if _, err := os.Stat(OcapLogFilePath); err == nil {
		if err := os.Rename(OcapLogFilePath, OcapLogFilePath+".old"); err != nil {
			Logger.Warn("Failed to rotate old log file", "error", err)
		}
	}

	OcapLogFile, err = os.OpenFile(OcapLogFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		Logger.Error("Failed to create/open log file!", "error", err, "path", OcapLogFilePath)
	}

	Logger.Info("Begin logging in logs directory", "path", OcapLogFilePath)

	// Initialize OTel provider if enabled (after log file is created)
	otelCfg := config.GetOTelConfig()
	if otelCfg.Enabled {
		OTelProvider, err = intOtel.New(intOtel.Config{
			Enabled:      otelCfg.Enabled,
			ServiceName:  otelCfg.ServiceName,
			BatchTimeout: otelCfg.BatchTimeout,
			LogWriter:    OcapLogFile,      // Write OTel logs to file
			Endpoint:     otelCfg.Endpoint, // Optional OTLP endpoint
			Insecure:     otelCfg.Insecure,
		})
		if err != nil {
			Logger.Error("Failed to initialize OTel provider", "error", err)
		} else {
			if otelCfg.Endpoint != "" {
				Logger.Info("OTel provider initialized", "file", OcapLogFilePath, "endpoint", otelCfg.Endpoint)
			} else {
				Logger.Info("OTel provider initialized", "file", OcapLogFilePath)
			}
		}
	}

	// Re-setup logging with file output and optional OTel
	var otelLogProvider *sdklog.LoggerProvider
	if OTelProvider != nil {
		otelLogProvider = OTelProvider.LoggerProvider()
	}
	SlogManager.Setup(OcapLogFile, viper.GetString("logLevel"), otelLogProvider)
	Logger = SlogManager.Logger()
	Logger.Info("Logging to file", "path", OcapLogFilePath)

	// set up a3interfaces
	Logger.Info("Setting up a3interface...")
	err = setupA3Interface()
	if err != nil {
		Logger.Error("Failed to set up a3interfaces!", "error", err)
		panic(err)
	} else {
		Logger.Info("Set up a3interfaces")
	}

	// get count of cpus available
	// set GOMAXPROCS to n - 2, minimum 2
	// this is to ensure we're using all available cores

	// get number of CPUs
	numCPUs := runtime.NumCPU()
	Logger.Debug("Number of CPUs", "numCPUs", numCPUs)

	// set GOMAXPROCS
	runtime.GOMAXPROCS(max(numCPUs-2, 1))

	// Initialize parser (no DB dependency)
	parserService = parser.NewParser(Logger)

	go initAPIClient()
}

func initExtension() {
	// send ready callback to Arma
	if err := a3interface.WriteArmaCallback(ExtensionName, ":EXT:READY:"); err != nil {
		Logger.Warn("Failed to send EXT:READY callback", "error", err)
	}
	// send extension version
	if err := a3interface.WriteArmaCallback(ExtensionName, ":VERSION:", BuildVersion); err != nil {
		Logger.Warn("Failed to send VERSION callback", "error", err)
	}
}

func setupA3Interface() (err error) {
	a3interface.SetVersion(BuildVersion)

	// Create early dispatcher for commands that don't need DB/workers
	// This ensures :VERSION:, :INIT:, etc. work immediately when the DLL loads
	dispatcherLogger := logging.NewDispatcherLogger(Logger)
	earlyDispatcher, err := dispatcher.New(dispatcherLogger)
	if err != nil {
		return fmt.Errorf("failed to create early dispatcher: %w", err)
	}

	// Register early handlers
	registerLifecycleHandlers(earlyDispatcher)
	a3interface.SetDispatcher(earlyDispatcher)
	eventDispatcher = earlyDispatcher

	Logger.Info("Early dispatcher initialized with lifecycle handlers")
	return nil
}

func loadConfig() (err error) {
	return config.Load(AddonFolder)
}

func initAPIClient() {
	serverURL := viper.GetString("api.serverUrl")
	if serverURL == "" {
		Logger.Info("API server URL not configured, upload disabled")
		return
	}

	apiKey := viper.GetString("api.apiKey")
	apiClient = api.New(serverURL, apiKey)

	if err := apiClient.Healthcheck(); err != nil {
		Logger.Info("OCAP Frontend is offline", "error", err)
	} else {
		Logger.Info("OCAP Frontend is online")
	}
}

// handleNewMission handles the :NEW:MISSION: command: parses mission data,
// delegates DB persistence to the storage backend, and sets runtime state.
func handleNewMission(e dispatcher.Event) (any, error) {
	if parserService == nil {
		return nil, nil
	}

	// 1. Parse (returns core types)
	coreMission, coreWorld, err := parserService.ParseMission(e.Args)
	if err != nil {
		return nil, err
	}
	coreMission.AddonVersion = addonVersion
	coreMission.ExtensionVersion = BuildVersion

	// 2. Reset caches
	MarkerCache.Reset()
	EntityCache.Reset()

	// 3. Start backend (handles DB ops for GORM/SQLite backends, stores missionID internally)
	if storageBackend != nil {
		if err := storageBackend.StartMission(&coreMission, &coreWorld); err != nil {
			Logger.Error("Failed to start mission in storage backend", "error", err)
			return nil, err
		}
	}

	Logger.Info("New mission logged", "missionName", coreMission.MissionName)

	// 4. ArmA callback
	if err := a3interface.WriteArmaCallback(ExtensionName, ":MISSION:OK:", "OK"); err != nil {
		Logger.Warn("Failed to send MISSION:OK callback", "error", err)
	}
	return nil, nil
}

// registerLifecycleHandlers registers system/lifecycle command handlers with the dispatcher
func registerLifecycleHandlers(d *dispatcher.Dispatcher) {
	// Simple commands (RVExtension style - no args)
	d.Register(":INIT:", func(e dispatcher.Event) (any, error) {
		go initExtension()
		return nil, nil
	})

	d.Register(":INIT:STORAGE:", func(e dispatcher.Event) (any, error) {
		go func() {
			if err := initStorage(); err != nil {
				Logger.Error("Storage initialization failed", "error", err)
			}
		}()
		return nil, nil
	})

	// Simple queries - sync return is sufficient, no callback needed
	d.Register(":VERSION:", func(e dispatcher.Event) (any, error) {
		return []string{BuildVersion, BuildCommit, BuildDate}, nil
	})

	d.Register(":GETDIR:ARMA:", func(e dispatcher.Event) (any, error) {
		return ArmaDir, nil
	})

	d.Register(":GETDIR:MODULE:", func(e dispatcher.Event) (any, error) {
		return ModulePath, nil
	})

	d.Register(":GETDIR:OCAPLOG:", func(e dispatcher.Event) (any, error) {
		return OcapLogFilePath, nil
	})

	// Commands with args (RVExtensionArgs style)
	d.Register(":ADDON:VERSION:", func(e dispatcher.Event) (any, error) {
		if len(e.Args) > 0 {
			addonVersion = util.FixEscapeQuotes(util.TrimQuotes(e.Args[0]))
			Logger.Info("Addon version", "version", addonVersion)
		}
		return nil, nil
	})

	// :LOG: is used by the addon to log messages through the extension
	d.Register(":LOG:", func(e dispatcher.Event) (any, error) {
		if len(e.Args) > 0 {
			msg := util.FixEscapeQuotes(util.TrimQuotes(e.Args[0]))
			Logger.Info("Addon log", "message", msg)
		}
		return nil, nil
	})

	d.Register(":NEW:MISSION:", handleNewMission, dispatcher.Buffered(1), dispatcher.Blocking(), dispatcher.Gated(storageReady))

	d.Register(":SAVE:MISSION:", func(e dispatcher.Event) (any, error) {
		Logger.Info("Received :SAVE:MISSION: command, ending mission recording")
		if storageBackend != nil {
			if err := storageBackend.EndMission(); err != nil {
				Logger.Error("Failed to end mission in storage backend", "error", err)
				return nil, err
			}
			Logger.Info("Mission recording saved to storage backend")

			// Upload if backend supports it and API client is configured
			if u, ok := storageBackend.(storage.Uploadable); ok && apiClient != nil {
				if path := u.GetExportedFilePath(); path != "" {
					meta := u.GetExportMetadata()
					if err := apiClient.Upload(path, meta); err != nil {
						Logger.Error("Failed to upload to OCAP web", "error", err, "path", path)
						// Don't return error - file is saved locally
					} else {
						Logger.Info("Mission uploaded to OCAP web", "path", path)
					}
				}
			}
		}
		// Flush OTel data if provider is available
		if OTelProvider != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := OTelProvider.Flush(ctx); err != nil {
				Logger.Warn("Failed to flush OTel data", "error", err)
			}
		}
		return nil, nil
	})
}
