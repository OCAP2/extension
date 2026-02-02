package main

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
*/
import "C" // This is required to import the C code

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/database"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/handlers"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/monitor"
	intOtel "github.com/OCAP2/extension/v5/internal/otel"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/storage/memory"
	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/internal/worker"
	"github.com/OCAP2/extension/v5/pkg/a3interface"

	"github.com/spf13/viper"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// module defs - BuildDate can be set at build time via ldflags
var (
	CurrentExtensionVersion string = "0.0.1"
	BuildDate               string = "unknown"

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

	// SqliteDBFilePath refers to the sqlite database file
	SqliteDBFilePath string
)

// global variables
var (
	// DB is the GORM DB interface
	DB *gorm.DB

	// ShouldSaveLocal indicates whether we're saving to Postgres or local SQLite
	ShouldSaveLocal bool = false

	// IsDatabaseValid indicates whether or not any DB connection could be established
	IsDatabaseValid bool = false

	// sqlDB is the native Go SQL interface
	sqlDB *sql.DB

	// SlogManager handles all slog-based logging
	SlogManager *logging.SlogManager

	// Logger is the slog logger (convenience reference)
	Logger *slog.Logger

	// OTelProvider handles OpenTelemetry
	OTelProvider *intOtel.Provider

	// testing
	IsDemoData bool = false

	// sqlite flow
	DBInsertsPaused bool = false

	// EntityCache is a map of all entities in the current mission, used to find associated entities by ocapID for entity state processing
	EntityCache *cache.EntityCache = cache.NewEntityCache()

	// MarkerCache maps marker names to their database IDs for the current mission
	MarkerCache *cache.MarkerCache = cache.NewMarkerCache()

	SessionStartTime time.Time = time.Now()

	addonVersion string = "unknown"

	// Services
	handlerService  *handlers.Service
	workerManager   *worker.Manager
	monitorService  *monitor.Service
	queues          *worker.Queues
	eventDispatcher *dispatcher.Dispatcher

	// Storage backend (optional)
	storageBackend storage.Backend
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
		os.Mkdir(AddonFolder, 0755)
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
		os.Mkdir(viper.GetString("logsDir"), 0755)
	}

	OcapLogFilePath = fmt.Sprintf(
		`%s\%s.%s.log`,
		viper.GetString("logsDir"),
		ExtensionName,
		SessionStartTime.Format("20060102_150405"),
	)

	// check if OcapLogFilePath exists
	// if it does, move it to OcapLogFilePath.old
	// if it doesn't, create it
	if _, err := os.Stat(OcapLogFilePath); err == nil {
		os.Rename(OcapLogFilePath, OcapLogFilePath+".old")
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
			LogWriter:    OcapLogFile,       // Write OTel logs to file
			Endpoint:     otelCfg.Endpoint,  // Optional OTLP endpoint
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

	// Set up dynamic state callbacks for logging
	SlogManager.GetMissionName = func() string {
		if handlerService != nil {
			return handlerService.GetMissionContext().GetMission().MissionName
		}
		return ""
	}
	SlogManager.GetMissionID = func() uint {
		if handlerService != nil {
			return handlerService.GetMissionContext().GetMission().ID
		}
		return 0
	}
	SlogManager.IsUsingLocalDB = func() bool { return ShouldSaveLocal }
	SlogManager.IsStatusRunning = func() bool {
		if monitorService != nil {
			return monitorService.IsRunning()
		}
		return false
	}

	SqliteDBFilePath = fmt.Sprintf(`%s\%s_%s.db`, AddonFolder, ExtensionName, SessionStartTime.Format("20060102_150405"))
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
	runtime.GOMAXPROCS(int(math.Max(float64(numCPUs-2), 1)))

	go func() {
		startGoroutines()

		// log frontend status
		checkServerStatus()
	}()
}

func initExtension() {
	// send ready callback to Arma
	a3interface.WriteArmaCallback(ExtensionName, ":EXT:READY:")
	// send extension version
	a3interface.WriteArmaCallback(ExtensionName, ":VERSION:", CurrentExtensionVersion)
}

func initStorage() error {
	Logger.Debug("Received :INIT:STORAGE: call")
	// Config is already loaded in init()
	functionName := ":INIT:STORAGE:"

	storageCfg := config.GetStorageConfig()
	if storageCfg.Type == "memory" {
		Logger.Info("Memory storage mode initialized")
		a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:OK:", "memory")
		return nil
	}

	// Database storage mode
	var err error
	DB, err = getDB()
	if err != nil {
		SlogManager.WriteLog(functionName, fmt.Sprintf(`Error connecting to database: %v`, err), "ERROR")
		a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:ERROR:", err.Error())
		return err
	}
	if DB == nil {
		err = fmt.Errorf("database connection is nil")
		SlogManager.WriteLog(functionName, err.Error(), "ERROR")
		a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:ERROR:", err.Error())
		return err
	}
	a3interface.WriteArmaCallback(ExtensionName, ":STORAGE:OK:", DB.Dialector.Name())
	return nil
}

func setupA3Interface() (err error) {
	a3interface.SetVersion(CurrentExtensionVersion)

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

func checkServerStatus() {
	var err error

	// check if server is running by making a healthcheck API request
	// if server is not running, log error and exit
	_, err = http.Get(viper.GetString("api.serverUrl") + "/healthcheck")
	if err != nil {
		Logger.Info("OCAP Frontend is offline")
	} else {
		Logger.Info("OCAP Frontend is online")
	}
}

///////////////////////
// DATABASE OPS //
///////////////////////

func getSqliteDB(path string) (db *gorm.DB, err error) {
	functionName := "getSqliteDB"
	db, err = database.GetSqliteDBStandalone(path)
	if err != nil {
		IsDatabaseValid = false
		return nil, err
	}
	if path != "" {
		SlogManager.WriteLog(functionName, fmt.Sprintf("Using local SQlite DB at '%s'", path), "INFO")
	} else {
		SlogManager.WriteLog(functionName, "Using local SQlite DB in memory with periodic disk dump", "INFO")
	}
	return db, nil
}

func getBackupDBPaths() (dbPaths []string, err error) {
	return database.GetBackupDBPaths(AddonFolder)
}

func dumpMemoryDBToDisk() (err error) {
	functionName := "dumpMemoryDBToDisk"
	start := time.Now()
	err = database.DumpMemoryDBToDisk(DB, SqliteDBFilePath)
	if err != nil {
		SlogManager.WriteLog(functionName, "Error dumping memory DB to disk", "ERROR")
		return err
	}
	SlogManager.WriteLog(functionName, fmt.Sprintf(`Dumped memory DB to disk in %s`, time.Since(start)), "INFO")
	Logger.Debug("Dumped memory DB to disk", "duration", time.Since(start))
	return nil
}

func getPostgresDB() (db *gorm.DB, err error) {
	Logger.Debug("Connecting to Postgres DB")
	return database.GetPostgresDBStandalone()
}

// getDB connects to the Postgres database, and if it fails, it will use a local SQlite DB
func getDB() (db *gorm.DB, err error) {
	functionName := ":INIT:DB:"

	db, err = getPostgresDB()
	if err != nil {
		Logger.Error("Failed to connect to Postgres DB, trying SQLite", "error", err)
		ShouldSaveLocal = true
		db, err = getSqliteDB("")
		if err != nil || db == nil {
			IsDatabaseValid = false
			return nil, fmt.Errorf("failed to get local SQLite DB: %s", err)
		}
		IsDatabaseValid = true
	}
	// test connection
	sqlDB, err = db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to access sql interface: %s", err)
	}
	err = sqlDB.Ping()
	if err != nil {
		SlogManager.WriteLog(functionName, fmt.Sprintf(`Failed to validate connection. Err: %s`, err), "ERROR")
		ShouldSaveLocal = true
		db, err = getSqliteDB("")
		if err != nil || db == nil {
			IsDatabaseValid = false
			return nil, fmt.Errorf("failed to get local SQLite DB: %s", err)
		}
		IsDatabaseValid = true

	} else {
		SlogManager.WriteLog(functionName, "Connected to database", "INFO")
		IsDatabaseValid = true
	}

	if !IsDatabaseValid {
		return nil, fmt.Errorf("db not valid. not saving")
	}

	if !ShouldSaveLocal {
		sqlDB.SetMaxOpenConns(10)
	}

	return db, nil
}

// setupDB will migrate tables and create default group settings if they don't exist
func setupDB(db *gorm.DB) (err error) {
	functionName := "setupDB"

	// Check if OcapInfo table exists
	if !db.Migrator().HasTable(&model.OcapInfo{}) {
		// Create the table
		err = db.AutoMigrate(&model.OcapInfo{})
		if err != nil {
			SlogManager.WriteLog(functionName, fmt.Sprintf(`Failed to create ocap_info table. Err: %s`, err), "ERROR")
			IsDatabaseValid = false
			return err
		}
		// Create the default settings
		err = db.Create(&model.OcapInfo{
			GroupName:        "OCAP",
			GroupDescription: "OCAP",
			GroupLogo:        "https://i.imgur.com/0Q4z0ZP.png",
			GroupWebsite:     "https://ocap.arma3.com",
		}).Error

		if err != nil {
			IsDatabaseValid = false
			return fmt.Errorf("failed to create ocap_info entry: %s", err)
		}
	}

	/////////////////////////////
	// Migrate the schema
	/////////////////////////////

	// Ensure PostGIS Extension is installed
	if db.Dialector.Name() == "postgres" {
		err = DB.Exec(`
		CREATE Extension IF NOT EXISTS postgis;
		`).Error
		if err != nil {
			IsDatabaseValid = false
			return fmt.Errorf("failed to create PostGIS Extension: %s", err)
		}
		SlogManager.WriteLog(functionName, "PostGIS Extension created", "INFO")
	}

	Logger.Info("Migrating schema")
	if ShouldSaveLocal {
		err = db.AutoMigrate(model.DatabaseModelsSQLite...)
	} else {
		err = db.AutoMigrate(model.DatabaseModels...)
	}
	if err != nil {
		IsDatabaseValid = false
		return fmt.Errorf("failed to migrate schema: %s", err)
	}

	Logger.Info("Database setup complete")
	return nil
}

func startGoroutines() (err error) {
	functionName := "startGoroutines"

	// Initialize queues
	queues = worker.NewQueues()

	// Initialize mission context
	missionCtx := handlers.NewMissionContext()

	// Initialize handler service
	handlerService = handlers.NewService(handlers.Dependencies{
		DB:            DB,
		EntityCache:   EntityCache,
		MarkerCache:   MarkerCache,
		LogManager:    SlogManager,
		ExtensionName: ExtensionName,
		AddonVersion:  addonVersion,
	}, missionCtx)

	// Initialize worker manager
	workerManager = worker.NewManager(worker.Dependencies{
		DB:              DB,
		EntityCache:     EntityCache,
		MarkerCache:     MarkerCache,
		LogManager:      SlogManager,
		HandlerService:  handlerService,
		IsDatabaseValid: func() bool { return IsDatabaseValid },
		ShouldSaveLocal: func() bool { return ShouldSaveLocal },
		DBInsertsPaused: func() bool { return DBInsertsPaused },
	}, queues)

	// Initialize storage backend if configured for memory mode
	storageCfg := config.GetStorageConfig()
	if storageCfg.Type == "memory" {
		storageBackend = memory.New(storageCfg.Memory)
		storageBackend.Init()
		workerManager.SetBackend(storageBackend)
		handlerService.SetBackend(storageBackend)
		Logger.Info("Memory storage backend initialized")
	}

	// Register worker handlers with the early dispatcher (created in setupA3Interface)
	Logger.Debug("Registering worker handlers with dispatcher")
	workerManager.RegisterHandlers(eventDispatcher)
	Logger.Info("Worker handlers registered with dispatcher")

	// Start DB writers (processes queues filled by dispatcher handlers)
	Logger.Debug("Starting DB writers")
	workerManager.StartDBWriters()

	// Initialize monitor service
	monitorService = monitor.NewService(monitor.Dependencies{
		DB:             DB,
		LogManager:     SlogManager,
		HandlerService: handlerService,
		WorkerManager:  workerManager,
		Queues:         queues,
		AddonFolder:    AddonFolder,
		IsDatabaseValid: func() bool { return IsDatabaseValid },
	})

	if !monitorService.IsRunning() {
		Logger.Debug("Status process not running, starting it")
		monitorService.Start()
	}

	// goroutine to, every x seconds, pause insert execution and dump memory sqlite db to disk
	go func() {
		Logger.Debug("Starting DB dump goroutine", "function", functionName)

		for {
			time.Sleep(3 * time.Minute)
			if !ShouldSaveLocal {
				continue
			}

			// pause insert execution
			DBInsertsPaused = true

			// dump memory sqlite db to disk
			SlogManager.WriteLog(functionName, "Dumping in-memory SQLite DB to disk", "DEBUG")
			err = dumpMemoryDBToDisk()
			if err != nil {
				SlogManager.WriteLog(functionName, fmt.Sprintf(`Error dumping memory db to disk: %v`, err), "ERROR")
			}

			// resume insert execution
			DBInsertsPaused = false
		}
	}()

	// log post goroutine creation
	SlogManager.WriteLog(functionName, "Goroutines started successfully", "INFO")
	return nil
}

//////////////////////////////////////////////////////////////
// Direct (exe) functions
//////////////////////////////////////////////////////////////

// get all the table models and data from any sqlite databases in SqlitePath
// then insert into Postgres
func migrateBackupsSqlite() (err error) {

	sqlitePaths, err := getBackupDBPaths()
	if err != nil {
		return fmt.Errorf("error getting backup database paths: %v", err)
	}
	postgresDB, err := getPostgresDB()
	if err != nil {
		return fmt.Errorf("error getting postgres database: %v", err)
	}

	successfulMigrations := make([]string, 0)

	for _, sqlitePath := range sqlitePaths {
		sqliteDB, err := getSqliteDB(sqlitePath)
		if err != nil {
			return fmt.Errorf("error getting sqlite database: %v", err)
		}

		// transaction for Postgres so we can rollback if errors
		tx := postgresDB.Begin()

		// migrate all tables
		err = migrateTable(sqliteDB, tx, model.OcapInfo{}, "ocap_infos")
		if err != nil {
			return fmt.Errorf("error migrating ocapinfo: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.AfterActionReview{}, "after_action_reviews")
		if err != nil {
			return fmt.Errorf("error migrating after_action_reviews: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.World{}, "worlds")
		if err != nil {
			return fmt.Errorf("error migrating worlds: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.Mission{}, "missions")
		if err != nil {
			return fmt.Errorf("error migrating missions: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.Soldier{}, "soldiers")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating soldiers: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.SoldierState{}, "soldier_states")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating soldier_states: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.Vehicle{}, "vehicles")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating vehicles: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.VehicleState{}, "vehicle_states")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating vehicle_states: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.FiredEvent{}, "fired_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating fired_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.ProjectileEvent{}, "projectile_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating projectile_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.GeneralEvent{}, "general_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating general_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.HitEvent{}, "hit_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating hit_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.KillEvent{}, "kill_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating kill_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.ChatEvent{}, "chat_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating chat_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.RadioEvent{}, "radio_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating radio_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.ServerFpsEvent{}, "server_fps_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating server_fps_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.Ace3DeathEvent{}, "ace3_death_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating ace3_death_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.Ace3UnconsciousEvent{}, "ace3_unconscious_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating ace3_unconscious_events: %v", err)
		}
		err = migrateTable(sqliteDB, tx, model.OcapPerformance{}, "ocap_performances")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating ocap_performances: %v", err)
		}

		// With no issues, we commit the transaction
		tx.Commit()

		// if we get here, we've successfully migrated this backup
		// remove connections to the databases
		sqlConnection, err := sqliteDB.DB()
		if err != nil {
			Logger.Error("Error getting sqlite connection", "error", err)
			continue
		}
		err = sqlConnection.Close()
		if err != nil {
			Logger.Error("Error closing sqlite connection", "error", err)
		}
		err = os.Rename(sqlitePath, sqlitePath+".migrated")
		if err != nil {
			Logger.Error("Error renaming sqlite file", "error", err)
		}
		successfulMigrations = append(successfulMigrations, sqlitePath)
	}

	// if we get here, we've successfully migrated all backups
	Logger.Info("Successfully migrated backups, it's recommended to delete these to avoid future data duplication",
		"count", len(successfulMigrations),
		"paths", successfulMigrations)

	return nil
}

// helper function for sqlite migrations
func migrateTable[M any](
	sqliteDB *gorm.DB,
	postgresDB *gorm.DB,
	model M,
	tableName string,
) (err error) {
	var data = &map[string]any{}
	sqliteDB.Model(&model).
		Assign("id", gorm.Expr("NULL")). // remove the id field from the data
		Find(data)
	Logger.Info("Found records", "count", len(*data), "table", tableName)

	if len(*data) == 0 {
		return nil
	}

	Logger.Info("Inserting records", "count", len(*data), "table", tableName)

	// insert into postgres
	postgresDB.Model(&model).Clauses(
		clause.OnConflict{
			DoNothing: true,
		}).Create(data)
	if postgresDB.Error != nil {
		Logger.Error("Error migrating table", "error", err, "database", sqliteDB.Name(), "table", tableName)
		return err
	}

	return nil
}

// registerLifecycleHandlers registers system/lifecycle command handlers with the dispatcher
func registerLifecycleHandlers(d *dispatcher.Dispatcher) {
	// Simple commands (RVExtension style - no args)
	d.Register(":INIT:", func(e dispatcher.Event) (any, error) {
		go initExtension()
		return "ok", nil
	})

	d.Register(":INIT:STORAGE:", func(e dispatcher.Event) (any, error) {
		go func() {
			if err := initStorage(); err != nil {
				Logger.Error("Storage initialization failed", "error", err)
			}
		}()
		return "ok", nil
	})

	// Simple queries - sync return is sufficient, no callback needed
	d.Register(":VERSION:", func(e dispatcher.Event) (any, error) {
		return []string{CurrentExtensionVersion, BuildDate}, nil
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
		return "ok", nil
	})

	d.Register(":NEW:MISSION:", func(e dispatcher.Event) (any, error) {
		if handlerService != nil {
			handlerService.LogNewMission(e.Args)
		}
		return "ok", nil
	})

	d.Register(":SAVE:", func(e dispatcher.Event) (any, error) {
		Logger.Info("Received :SAVE: command, ending mission recording")
		if storageBackend != nil {
			if err := storageBackend.EndMission(); err != nil {
				Logger.Error("Failed to end mission in storage backend", "error", err)
				return nil, err
			}
			Logger.Info("Mission recording saved to storage backend")
		}
		// Flush OTel data if provider is available
		if OTelProvider != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := OTelProvider.Flush(ctx); err != nil {
				Logger.Warn("Failed to flush OTel data", "error", err)
			}
		}
		return "ok", nil
	})
}

// dispatchDemoEvent dispatches an event through the dispatcher for demo/test purposes
func dispatchDemoEvent(command string, args []string) {
	if eventDispatcher == nil {
		return
	}
	eventDispatcher.Dispatch(dispatcher.Event{
		Command:   command,
		Args:      args,
		Timestamp: time.Now(),
	})
}

func populateDemoData() {
	if !IsDatabaseValid {
		return
	}

	// declare test size counts
	var (
		numMissions              int = 2
		missionDuration          int = 60 * 15
		numUnitsPerMission       int = 60
		numSoldiers              int = int(math.Ceil(float64(numUnitsPerMission) * float64(0.8)))
		numFiredEventsPerSoldier int = 2700
		numVehicles              int = numUnitsPerMission - numSoldiers

		waitGroup = sync.WaitGroup{}

		sides []string = []string{"WEST", "EAST", "GUER", "CIV"}

		vehicleClassnames []string = []string{
			"B_MRAP_01_F", "B_MRAP_01_gmg_F", "B_MRAP_01_hmg_F",
			"B_G_Offroad_01_armed_F", "B_G_Offroad_01_AT_F", "B_G_Offroad_01_F",
			"B_G_Offroad_01_repair_F", "B_APC_Wheeled_01_cannon_F",
			"B_APC_Tracked_01_AA_F", "B_APC_Tracked_01_CRV_F",
			"B_APC_Tracked_01_rcws_F",
		}

		ocapTypes []string = []string{
			"ship", "parachute", "heli", "plane", "truck",
			"car", "apc", "tank", "staticMortar", "unknown",
		}

		roles []string = []string{
			"Rifleman", "Team Leader", "Auto Rifleman", "Assistant Auto Rifleman",
			"Grenadier", "Machine Gunner", "Assistant Machine Gunner", "Medic",
			"Engineer", "Explosive Specialist", "Rifleman (AT)", "Rifleman (AA)", "Officer",
		}

		roleDescriptions []string = []string{
			"Rifleman@Alpha", "Team Leader@Alpha", "Auto Rifleman@Alpha",
			"Assistant Auto Rifleman@Alpha", "Grenadier@Alpha", "Machine Gunner@Alpha",
			"Assistant Machine Gunner@Alpha", "Medic@Alpha",
			"Rifleman@Bravo", "Team Leader@Bravo", "Auto Rifleman@Bravo",
			"Assistant Auto Rifleman@Bravo", "Grenadier@Bravo", "Machine Gunner@Bravo",
			"Assistant Machine Gunner@Bravo", "Medic@Bravo",
		}

		weapons []string = []string{
			"Katiba", "MXC 6.5 mm", "MX 6.5 mm", "MX SW 6.5 mm", "MXM 6.5 mm",
			"SPAR-16 5.56 mm", "SPAR-16S 5.56 mm", "SPAR-17 7.62 mm",
			"TAR-21 5.56 mm", "TRG-21 5.56 mm", "TRG-20 5.56 mm",
			"TRG-21 EGLM 5.56 mm", "CAR-95 5.8 mm", "CAR-95-1 5.8 mm", "CAR-95 GL 5.8 mm",
		}

		magazines []string = []string{
			"30rnd 6.5 mm Caseless Mag", "30rnd 6.5 mm Caseless Mag Tracer",
			"100rnd 6.5 mm Caseless Mag", "100rnd 6.5 mm Caseless Mag Tracer",
			"200rnd 6.5 mm Caseless Mag", "200rnd 6.5 mm Caseless Mag Tracer",
			"30rnd 5.56 mm STANAG", "30rnd 5.56 mm STANAG Tracer (Yellow)",
			"30rnd 5.56 mm STANAG Tracer (Red)", "30rnd 5.56 mm STANAG Tracer (Green)",
		}

		firemodes []string = []string{"Single", "FullAuto", "Burst3", "Burst5"}

		worldNames []string = []string{
			"Altis", "Stratis", "VR", "Bootcamp_ACR", "Malden",
			"ProvingGrounds_PMC", "Shapur_BAF", "Sara", "Sara_dbe1", "SaraLite",
			"Woodland_ACR", "Chernarus", "Desert_E", "Desert_Island", "Intro", "Desert2",
		}

		demoAddons [][]any = [][]any{
			{"Community Base Addons v3.15.1", 0},
			{"Arma 3 Contact", 0},
			{"Arma 3 Creator DLC: Global Mobilization - Cold War Germany", 0},
			{"Arma 3 Tanks", 0},
			{"Arma 3 Tac-Ops", 0},
			{"Arma 3 Laws of War", 0},
			{"Arma 3 Apex", 0},
			{"Arma 3 Marksmen", "332350"},
			{"Arma 3 Helicopters", "304380"},
			{"Arma 3 Karts", "288520"},
			{"Arma 3 Zeus", "275700"},
		}
	)

	// write worlds
	missionsStart := time.Now()
	for i := 0; i < numMissions; i++ {
		data := make([]string, 2)

		worldNameOriginal := worldNames[rand.Intn(len(worldNames))]
		displayName := strings.Replace(worldNameOriginal, "_", " ", -1)
		worldName := strings.ToLower(worldNameOriginal)

		worldData := map[string]any{
			"author":            "Demo Author",
			"workshopID":        "123456789",
			"displayName":       displayName,
			"worldName":         worldName,
			"worldNameOriginal": worldNameOriginal,
			"worldSize":         10240,
			"latitude":          rand.Float64() * 180,
			"longitude":         rand.Float64() * 180,
		}
		worldDataJSON, err := json.Marshal(worldData)
		if err != nil {
			fmt.Println(err)
		}
		data[0] = string(worldDataJSON)

		missionData := map[string]any{
			"missionName":                  fmt.Sprintf("Demo Mission %d", i),
			"briefingName":                 fmt.Sprintf("Demo Briefing %d", i),
			"missionNameSource":            fmt.Sprintf("Demo Mission %d", i),
			"onLoadName":                   "TestLoadName",
			"author":                       "Demo Author",
			"serverName":                   "Demo Server",
			"serverProfile":                "Demo Profile",
			"missionStart":                 nil,
			"worldName":                    fmt.Sprintf("demo_world_%d", i),
			"tag":                          "Demo Tag",
			"captureDelay":                 1.0,
			"addonVersion":                 "1.0",
			"extensionVersion":             "1.0",
			"extensionBuild":               "1.0",
			"ocapRecorderExtensionVersion": "1.0",
			"addons":                       demoAddons,
			"playableSlots":                []float64{20, 20, 20, 5, 2},
			"sideFriendly":                 []bool{false, false, true},
		}
		missionDataJSON, err := json.Marshal(missionData)
		if err != nil {
			fmt.Println(err)
		}
		data[1] = string(missionDataJSON)

		dispatchDemoEvent(":NEW:MISSION:", data)

		time.Sleep(500 * time.Millisecond)
	}
	missionsEnd := time.Now()
	missionsElapsed := missionsEnd.Sub(missionsStart)
	fmt.Printf("Sent %d missions in %s\n", numMissions, missionsElapsed)

	// for each mission
	missions := []model.Mission{}
	DB.Model(&model.Mission{}).
		Order("created_at DESC").
		Limit(numMissions).
		Find(&missions)

	waitGroup = sync.WaitGroup{}
	for _, mission := range missions {
		if handlerService != nil {
			handlerService.GetMissionContext().SetMission(&mission, nil)
		}

		fmt.Printf("Populating mission with ID %d\n", mission.ID)

		// write soldiers
		idCounter := 1

		for i := 0; i <= numSoldiers; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				soldierID := strconv.FormatInt(int64(thisId), 10)
				squadParams := []any{
					[]string{"DD", "Diamond Dogs", "t@example.com", "https://example.com", "", "Diamond Dogs"},
					[]string{"12491654", "Sleeping Tiger", "Sleeping Tiger", "st@example.com", "", "Thanks, Boss!"},
					"1294510750",
					"04328572",
				}
				squadParamsJSON, err := json.Marshal(squadParams)
				if err != nil {
					fmt.Println(err)
				}

				soldier := []string{
					soldierID,
					soldierID,
					fmt.Sprintf("Demo Unit %d", i),
					fmt.Sprintf("Demo Group %d", i),
					sides[rand.Intn(len(sides))],
					strconv.FormatBool(rand.Intn(2) == 1),
					roleDescriptions[rand.Intn(len(roleDescriptions))],
					"B_Soldier_F",
					"Rifleman",
					strconv.FormatInt(int64(rand.Intn(1000000000)), 10),
					string(squadParamsJSON),
				}

				soldier = append(soldier, fmt.Sprintf("%d", time.Now().UnixNano()))
				dispatchDemoEvent(":NEW:SOLDIER:", soldier)

				for {
					time.Sleep(100 * time.Millisecond)
					if _, ok := EntityCache.GetSoldier(uint16(thisId)); ok {
						break
					}
				}

				var randomPos [3]float64 = [3]float64{rand.Float64() * 30720, rand.Float64() * 30720, rand.Float64() * 30720}
				var randomDir float64 = rand.Float64() * 360
				var dirMoveOffset float64 = rand.Float64() * 360
				var randomLifestate int = rand.Intn(3)
				var currentRole string = roles[rand.Intn(len(roles))]
				for j := 0; j <= missionDuration; j++ {
					time.Sleep(100 * time.Millisecond)

					stateFrame := j
					var xyTransform [2]float64 = [2]float64{0, 0}
					if randomDir < 180 {
						xyTransform[0] = math.Sin((randomDir + dirMoveOffset) * (math.Pi / 180))
						xyTransform[1] = math.Cos((randomDir + dirMoveOffset) * (math.Pi / 180))
					} else {
						xyTransform[0] = math.Sin((randomDir - dirMoveOffset) * (math.Pi / 180))
						xyTransform[1] = math.Cos((randomDir - dirMoveOffset) * (math.Pi / 180))
					}
					randomPos[0] += xyTransform[0] + (rand.Float64() * 8) - 4
					randomPos[1] += xyTransform[1] + (rand.Float64() * 8) - 4
					randomPos[2] += (rand.Float64() * 8) - 4
					randomDir += (rand.Float64() * 8) - 4
					if rand.Intn(10) == 0 {
						randomLifestate = rand.Intn(3)
					}
					if rand.Intn(20) == 0 {
						currentRole = roles[rand.Intn(len(roles))]
					}

					soldierState := []string{
						soldierID,
						fmt.Sprintf("[%f,%f,%f]", randomPos[0], randomPos[1], randomPos[2]),
						strconv.FormatFloat(randomDir, 'f', 0, 64),
						strconv.FormatInt(int64(randomLifestate), 10),
						strconv.FormatBool(rand.Intn(2) == 1),
						fmt.Sprintf("Demo Unit %d", j),
						strconv.FormatBool(rand.Intn(2) == 1),
						currentRole,
						strconv.FormatInt(int64(stateFrame), 10),
						strconv.FormatInt(int64(rand.Intn(2)), 10),
						strconv.FormatInt(int64(rand.Intn(2)), 10),
						fmt.Sprintf("%d,%d,%d,%d,%d,%d", rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20)),
						"Passenger",
						"-1",
						[]string{"Up", "Middle", "Down"}[rand.Intn(3)],
					}

					soldierState = append(soldierState, fmt.Sprintf("%d", time.Now().UnixNano()))
					dispatchDemoEvent(":NEW:SOLDIER:STATE:", soldierState)
				}
				waitGroup.Done()
			}(idCounter)
			idCounter++
		}

		EntityCache.Lock()
		Logger.Debug("Soldiers cached", "numSoldiersCached", len(EntityCache.Soldiers))
		EntityCache.Unlock()

		// write vehicles
		for i := 0; i <= numVehicles; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				vehicleID := strconv.FormatInt(int64(thisId), 10)
				vehicle := []string{
					vehicleID,
					vehicleID,
					ocapTypes[rand.Intn(len(ocapTypes))],
					fmt.Sprintf("Demo Vehicle %d", i),
					vehicleClassnames[rand.Intn(len(vehicleClassnames))],
					fmt.Sprintf(`"[[""%s"", %d], [""%s"", %d], [""%s"", %d]]"`, "wasp", 1, "AddTread", 1, "AddTread_Short", 1),
				}

				vehicle = append(vehicle, fmt.Sprintf("%d", time.Now().UnixNano()))
				dispatchDemoEvent(":NEW:VEHICLE:", vehicle)

				for {
					time.Sleep(1000 * time.Millisecond)
					if _, ok := EntityCache.GetVehicle(uint16(thisId)); ok {
						break
					}
				}

				for j := 0; j <= missionDuration; j++ {
					time.Sleep(100 * time.Millisecond)
					vehicleState := []string{
						vehicleID,
						fmt.Sprintf("[%f,%f,%f]", rand.Float64()*30720+1, rand.Float64()*30720+1, rand.Float64()*30720+1),
						fmt.Sprintf("%d", rand.Intn(360)),
						strconv.FormatBool(rand.Intn(2) == 1),
						fmt.Sprintf("[%d,%d,%d]", rand.Intn(numSoldiers)+1, rand.Intn(numSoldiers)+1, rand.Intn(numSoldiers)+1),
						strconv.FormatInt(int64(rand.Intn(missionDuration)), 10),
						fmt.Sprintf("%f", rand.Float64()),
						fmt.Sprintf("%f", rand.Float64()),
						strconv.FormatBool(rand.Intn(10) == 0),
						strconv.FormatBool(rand.Intn(10) == 0),
						sides[rand.Intn(len(sides))],
						fmt.Sprintf("[%f,%f,%f]", rand.Float64(), rand.Float64(), rand.Float64()),
						fmt.Sprintf("[%f,%f,%f]", rand.Float64(), rand.Float64(), rand.Float64()),
						fmt.Sprintf("%f", rand.Float64()),
						fmt.Sprintf("%f", rand.Float64()),
					}

					vehicleState = append(vehicleState, fmt.Sprintf("%d", time.Now().UnixNano()))
					dispatchDemoEvent(":NEW:VEHICLE:STATE:", vehicleState)
				}
				waitGroup.Done()
			}(idCounter)
			idCounter++
		}

		waitGroup.Wait()

		fmt.Println("Finished creating units and states.")
		fmt.Println("Creating fired events...")

		wg2 := sync.WaitGroup{}
		for i := 0; i <= numSoldiers*numFiredEventsPerSoldier; i++ {
			wg2.Add(1)

			go func() {
				var randomStartPos []float64 = []float64{rand.Float64()*30720 + 1, rand.Float64()*30720 + 1, rand.Float64()*30720 + 1}
				var randomEndPos []float64
				for j := 0; j < 3; j++ {
					randomEndPos = append(randomEndPos, randomStartPos[j]+rand.Float64()*400-200)
				}

				firedEvent := []string{
					strconv.FormatInt(int64(rand.Intn(numSoldiers)+1), 10),
					strconv.FormatInt(int64(rand.Intn(missionDuration)), 10),
					fmt.Sprintf("%f,%f,%f", randomStartPos[0], randomStartPos[1], randomStartPos[2]),
					fmt.Sprintf("%f,%f,%f", randomEndPos[0], randomEndPos[1], randomEndPos[2]),
					weapons[rand.Intn(len(weapons))],
					magazines[rand.Intn(len(magazines))],
					firemodes[rand.Intn(len(firemodes))],
				}

				firedEvent = append(firedEvent, fmt.Sprintf("%d", time.Now().UnixNano()))
				dispatchDemoEvent(":FIRED:", firedEvent)
				wg2.Done()
			}()
		}
		wg2.Wait()

		// demo markers
		markerTypes := []string{"mil_dot", "mil_triangle", "mil_box", "hd_flag"}
		markerColors := []string{"ColorRed", "ColorBlue", "ColorGreen", "ColorYellow"}
		markerShapes := []string{"ICON", "RECTANGLE", "ELLIPSE"}

		for i := 0; i < 10; i++ {
			marker := []string{
				fmt.Sprintf("DemoMarker_%d", i),
				fmt.Sprintf("%d", rand.Intn(360)),
				markerTypes[rand.Intn(len(markerTypes))],
				fmt.Sprintf("Demo Marker %d", i),
				strconv.Itoa(rand.Intn(missionDuration)),
				"-1",
				strconv.Itoa(rand.Intn(numSoldiers) + 1),
				markerColors[rand.Intn(len(markerColors))],
				"[1,1]",
				sides[rand.Intn(len(sides))],
				fmt.Sprintf("[%f,%f,%f]", rand.Float64()*30720+1, rand.Float64()*30720+1, 0.0),
				markerShapes[rand.Intn(len(markerShapes))],
				"1.0",
				"Solid",
			}
			marker = append(marker, fmt.Sprintf("%d", time.Now().UnixNano()))
			dispatchDemoEvent(":MARKER:CREATE:", marker)

			for j := 0; j < 3; j++ {
				time.Sleep(100 * time.Millisecond)
				markerMove := []string{
					fmt.Sprintf("DemoMarker_%d", i),
					strconv.Itoa(rand.Intn(missionDuration)),
					fmt.Sprintf("[%f,%f,%f]", rand.Float64()*30720+1, rand.Float64()*30720+1, 0.0),
					fmt.Sprintf("%d", rand.Intn(360)),
					"1.0",
				}
				markerMove = append(markerMove, fmt.Sprintf("%d", time.Now().UnixNano()))
				dispatchDemoEvent(":MARKER:MOVE:", markerMove)
			}
		}

		for i := 0; i < 3; i++ {
			markerDelete := []string{
				fmt.Sprintf("DemoMarker_%d", i),
				strconv.Itoa(missionDuration - 10),
			}
			markerDelete = append(markerDelete, fmt.Sprintf("%d", time.Now().UnixNano()))
			dispatchDemoEvent(":MARKER:DELETE:", markerDelete)
		}
	}

	// Give dispatcher time to process buffered events
	time.Sleep(2 * time.Second)
}

func getOcapRecording(missionIDs []string) (err error) {
	fmt.Println("Getting JSON for mission IDs: ", missionIDs)

	for _, missionID := range missionIDs {
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		txStart := time.Now()
		var mission model.Mission
		ocapMission := make(map[string]any)
		err = DB.Model(&model.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return err
		}

		ocapMission["addonVersion"] = mission.AddonVersion
		ocapMission["extensionVersion"] = mission.ExtensionVersion
		ocapMission["extensionBuild"] = mission.ExtensionBuild
		ocapMission["ocapRecorderExtensionVersion"] = mission.OcapRecorderExtensionVersion

		ocapMission["missionAuthor"] = mission.Author
		ocapMission["missionName"] = mission.OnLoadName
		if mission.OnLoadName == "" {
			ocapMission["missionName"] = mission.MissionName
		}

		world := model.World{}
		err = DB.Model(&model.World{}).Where("id = ?", mission.WorldID).First(&world).Error
		if err != nil {
			return fmt.Errorf("error getting world: %w", err)
		}
		ocapMission["worldName"] = world.WorldName

		totalSoldiers := int64(0)
		err = DB.Model(&model.Soldier{}).Where("mission_id = ?", missionIDInt).Count(&totalSoldiers).Error
		if err != nil {
			return fmt.Errorf("error getting soldier count: %w", err)
		}

		totalVehicles := int64(0)
		err = DB.Model(&model.Vehicle{}).Where("mission_id = ?", missionIDInt).Count(&totalVehicles).Error
		if err != nil {
			return fmt.Errorf("error getting vehicle count: %w", err)
		}

		ocapMission["Rone"] = map[string]any{}
		ocapMission["events"] = []any{}

		soldiers := []model.Soldier{}
		soldierTxStart := time.Now()
		err = DB.Model(&model.Soldier{}).Where("mission_id = ?", missionIDInt).Find(&soldiers).Error
		if err != nil {
			return fmt.Errorf("error getting soldiers: %w", err)
		}
		fmt.Println("Got soldiers in ", time.Since(soldierTxStart))

		entities := []map[string]any{}
		for _, soldier := range soldiers {
			entity := map[string]any{}
			entity["id"] = soldier.OcapID
			entity["name"] = soldier.UnitName
			entity["group"] = soldier.GroupID
			entity["side"] = soldier.Side
			entity["isPlayer"] = 0
			if soldier.IsPlayer {
				entity["isPlayer"] = 1
			}
			entity["type"] = "unit"
			entity["startFrameNum"] = soldier.JoinFrame

			soldierStates := []model.SoldierState{}
			err = DB.Model(&model.SoldierState{}).
				Where("mission_id = ? AND soldier_id = ?", missionIDInt, soldier.ID).
				Order("capture_frame ASC").
				Find(&soldierStates).Error
			if err != nil {
				return fmt.Errorf("error getting soldier states: %w", err)
			}

			positions := []any{}
			for _, state := range soldierStates {
				coord, _ := state.Position.Coordinates()
				position := []any{
					[]float64{coord.XY.X, coord.XY.Y},
					state.Bearing,
					state.Lifestate,
					state.InVehicleObjectID,
					state.UnitName,
					state.IsPlayer,
					state.CurrentRole,
				}
				positions = append(positions, position)
			}
			entity["positions"] = positions

			firedEvents := []model.FiredEvent{}
			err = DB.Model(&model.FiredEvent{}).
				Where("mission_id = ? AND soldier_id = ?", missionIDInt, soldier.ID).
				Order("capture_frame ASC").
				Find(&firedEvents).Error
			if err != nil {
				return fmt.Errorf("error getting fired events: %w", err)
			}

			framesFired := []any{}
			for _, event := range firedEvents {
				startCoord, _ := event.StartPosition.Coordinates()
				endCoord, _ := event.EndPosition.Coordinates()
				frameFired := []any{
					event.CaptureFrame,
					[]float64{endCoord.XY.X, endCoord.XY.Y},
					[]float64{startCoord.XY.X, startCoord.XY.Y},
					event.Weapon,
					event.Magazine,
					event.FiringMode,
				}
				framesFired = append(framesFired, frameFired)
			}
			entity["framesFired"] = framesFired

			entities = append(entities, entity)
		}

		vehicles := []model.Vehicle{}
		err = DB.Model(&model.Vehicle{}).Where("mission_id = ?", missionIDInt).Find(&vehicles).Error
		if err != nil {
			return fmt.Errorf("error getting vehicles: %w", err)
		}
		for _, vehicle := range vehicles {
			entity := map[string]any{}
			entity["id"] = vehicle.OcapID
			entity["name"] = vehicle.DisplayName
			entity["class"] = vehicle.ClassName
			entity["side"] = "UNKNOWN"
			entity["type"] = vehicle.OcapType
			entity["startFrameNum"] = vehicle.JoinFrame

			vehicleStates := []model.VehicleState{}
			err = DB.Model(&model.VehicleState{}).
				Where("mission_id = ? AND vehicle_id = ?", missionIDInt, vehicle.ID).
				Order("capture_frame ASC").
				Find(&vehicleStates).Error
			if err != nil {
				return fmt.Errorf("error getting vehicle states: %w", err)
			}

			positions := []any{}
			for _, state := range vehicleStates {
				coord, _ := state.Position.Coordinates()
				position := []any{
					[]float64{coord.XY.X, coord.XY.Y},
					state.Bearing,
					state.IsAlive,
					state.Crew,
				}
				positions = append(positions, position)
			}
			entity["positions"] = positions
			entity["framesFired"] = []any{}

			entities = append(entities, entity)
		}

		ocapMission["entities"] = entities

		// Compute endFrame from the maximum capture_frame across all states
		var endFrame uint
		DB.Model(&model.SoldierState{}).Where("mission_id = ?", missionIDInt).Select("COALESCE(MAX(capture_frame), 0)").Scan(&endFrame)
		ocapMission["endFrame"] = endFrame

		fmt.Println("Got mission data in ", time.Since(txStart))

		ocapMissionJSON, err := json.Marshal(ocapMission)
		if err != nil {
			return fmt.Errorf("error marshalling mission data: %w", err)
		}

		fileName := fmt.Sprintf("%s_%s.json.gz", mission.MissionName, mission.StartTime.Format("20060102_150405"))
		fileName = strings.ReplaceAll(fileName, " ", "_")
		fileName = strings.ReplaceAll(fileName, ":", "_")
		f, err := os.Create(fileName)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer f.Close()

		gzWriter := gzip.NewWriter(f)
		defer gzWriter.Close()
		_, err = gzWriter.Write(ocapMissionJSON)
		if err != nil {
			return fmt.Errorf("error writing to gzip: %w", err)
		}

		fmt.Println("Wrote mission data to ", fileName)
	}

	return nil
}

func reduceMission(missionIDs []string) (err error) {
	for _, missionID := range missionIDs {
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		txStart := time.Now()
		var mission model.Mission
		err = DB.Model(&model.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return fmt.Errorf("error getting mission: %w", err)
		}

		soldierStatesToDelete := []model.SoldierState{}
		err = DB.Model(&model.SoldierState{}).Where(
			"mission_id = ? AND capture_frame % 5 != 0",
			mission.ID,
		).Order("capture_frame ASC").Find(&soldierStatesToDelete).Error
		if err != nil {
			return fmt.Errorf("error getting soldier states to delete: %w", err)
		}

		if len(soldierStatesToDelete) == 0 {
			fmt.Println("No soldier states to delete for missionId ", missionID, ", checked in ", time.Since(txStart))
			continue
		}

		err = DB.Delete(&soldierStatesToDelete).Error
		if err != nil {
			return fmt.Errorf("error deleting soldier states: %w", err)
		}

		fmt.Println("Deleted ", len(soldierStatesToDelete), " soldier states from missionId ", missionID, " in ", time.Since(txStart))
	}

	fmt.Println("")
	fmt.Println("----------------------------------------")
	fmt.Println("")
	fmt.Println("Finished reducing soldier states, running VACUUM to recover space...")
	txStart := time.Now()
	tables := []string{}
	err = DB.Raw(
		`SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&tables).Error
	if err != nil {
		return fmt.Errorf("error getting tables to vacuum: %w", err)
	}

	for _, table := range tables {
		err = DB.Raw(fmt.Sprintf("VACUUM (FULL) %s", table)).Error
		if err != nil {
			return fmt.Errorf("error running VACUUM on table %s: %w", table, err)
		}
	}

	fmt.Println("Finished VACUUM in ", time.Since(txStart))
	fmt.Println("Finished reducing, press enter to exit.")

	return nil
}

func testQuery() (err error) {
	query := `
select
    s.ocap_id,
    ss.capture_frame,
    json_agg(ss.*) as states,
    json_agg(hit.*) as hits,
    json_agg(kill.*) as kills,
    json_agg(fire.*) as fired,
    json_agg(re.*) as radio,
    json_agg(ce.*) as chat
from soldiers s
  left join (
    select *
    from soldier_states
    order by capture_frame asc
  ) ss on ss.soldier_id = s.id
  left join kill_events kill on (
    kill.victim_id_soldier = s.id
    or kill.killer_id_soldier = s.id
  )
  and ss.capture_frame = kill.capture_frame
  left join hit_events hit on (
    hit.victim_id_soldier = s.id
    or hit.shooter_id_soldier = s.id
  )
  and ss.capture_frame = hit.capture_frame
  left join fired_events fire on fire.soldier_id = s.id
  and ss.capture_frame = fire.capture_frame
  left join radio_events re on re.soldier_id = s.id
  and ss.capture_frame = re.capture_frame
  left join chat_events ce on ce.soldier_id = s.id
  and ss.capture_frame = ce.capture_frame
where s.mission_id = ? and ss.capture_frame between ? and ?
group by s.ocap_id,
  ss.capture_frame
order by s.ocap_id,
  ss.capture_frame;
`

	frameData := []model.FrameData{}
	err = DB.Raw(query, 4, 0, 100).Scan(&frameData).Error
	if err != nil {
		fmt.Println(err)
		return
	}

	jsonBytes, err := json.Marshal(frameData)
	if err != nil {
		return err
	}
	err = os.WriteFile("test.json", jsonBytes, 0644)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	return nil
}

func main() {
	var err error
	Logger.Info("Starting up...")

	Logger.Info("Initializing storage...")
	err = initStorage()
	if err != nil {
		panic(err)
	}
	Logger.Info("Storage initialization complete.")
	initExtension()

	args := os.Args[1:]
	if len(args) > 0 {
		if strings.ToLower(args[0]) == "demo" {
			Logger.Info("Populating demo data...")
			IsDemoData = true
			demoStart := time.Now()
			populateDemoData()
			Logger.Info("Demo data populated.", "duration", time.Since(demoStart))
			fmt.Println("Press enter to exit.")
		}
		if strings.ToLower(args[0]) == "setupdb" {
			err = setupDB(DB)
			if err != nil {
				panic(err)
			}
			Logger.Info("DB setup complete.")
		}
		if strings.ToLower(args[0]) == "getjson" {
			missionIds := args[1:]
			if len(missionIds) > 0 {
				err = getOcapRecording(missionIds)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Println("No mission IDs provided.")
			}
		}
		if strings.ToLower(args[0]) == "reducemission" {
			missionIds := args[1:]
			if len(missionIds) > 0 {
				err = reduceMission(missionIds)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Println("No mission IDs provided.")
			}
		}

		if strings.ToLower(args[0]) == "migratebackups" {
			err = migrateBackupsSqlite()
			if err != nil {
				panic(err)
			}
			Logger.Info("Finished migrating backups.")
		}
		if strings.ToLower(args[0]) == "testquery" {
			err = testQuery()
			if err != nil {
				panic(err)
			}
		}
	} else {
		fmt.Println("No arguments provided.")
	}
	fmt.Scanln()
}
