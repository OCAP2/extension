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
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	defs "github.com/OCAP2/extension/internal/definitions"
	"github.com/OCAP2/extension/pkg/a3interface"
	"github.com/OCAP2/extension/pkg/assemblyfinder"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2_api "github.com/influxdata/influxdb-client-go/v2/api"
	influxdb2_write "github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/spf13/viper"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/glebarez/sqlite"
	geom "github.com/peterstace/simplefeatures/geom"
	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// module defs
var (
	CurrentExtensionVersion string = "0.0.1"

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

	// InfluxBackupFilePath is the path to the gzip where influx lineprotocol is stored if InfluxDB is not available
	InfluxBackupFilePath string

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

	// IsInfluxValid indicates whether or not InfluxDB connection could be established
	IsInfluxValid bool = false

	// InfluxClient is the InfluxDB client
	InfluxClient influxdb2.Client
	// InfluxBackupWriter is the gzip writer Influx will use
	InfluxBackupWriter *gzip.Writer

	// LogIoConn is the connection to the log.io service
	LogIoConn net.Conn
	// NullByte is a byte slice containing a single null byte
	NullByte byte = 0x00

	// GraylogWriter is the GELF writer
	GraylogWriter *gelf.Writer

	// testing
	IsDemoData bool = false

	// sqlite flow
	DBInsertsPaused bool = false

	// EntityCache is a map of all entities in the current mission, used to find associated entities by ocapID for entity state processing
	EntityCache *defs.EntityCacheStruct = defs.NewEntityCache()

	// MarkerCache maps marker names to their database IDs for the current mission
	MarkerCache     = make(map[string]uint)
	MarkerCacheLock sync.RWMutex

	SessionStartTime       time.Time = time.Now()
	LastDBWriteDuration    time.Duration
	IsStatusProcessRunning bool = false

	addonVersion string = "unknown"
)

// channels
var (
	// caches of processed models pending DB write
	soldiersToWrite              = defs.SoldiersQueue{}
	soldierStatesToWrite         = defs.SoldierStatesQueue{}
	vehiclesToWrite              = defs.VehiclesQueue{}
	vehicleStatesToWrite         = defs.VehicleStatesQueue{}
	firedEventsToWrite           = defs.FiredEventsQueue{}
	projectileEventsToWrite      = defs.ProjectileEventsQueue{}
	generalEventsToWrite         = defs.GeneralEventsQueue{}
	hitEventsToWrite             = defs.HitEventsQueue{}
	killEventsToWrite            = defs.KillEventsQueue{}
	chatEventsToWrite            = defs.ChatEventsQueue{}
	radioEventsToWrite           = defs.RadioEventsQueue{}
	fpsEventsToWrite             = defs.FpsEventsQueue{}
	ace3DeathEventsToWrite       = defs.Ace3DeathEventsQueue{}
	ace3UnconsciousEventsToWrite = defs.Ace3UnconsciousEventsQueue{}
	markersToWrite               = defs.MarkersQueue{}
	markerStatesToWrite          = defs.MarkerStatesQueue{}

	InfluxBucketNames = []string{
		"mission_data",
		"ocap_performance",
		"player_performance",
		"server_performance",
		"Telegraf",
	}
	InfluxWriters = make(map[string]influxdb2_api.WriteAPI)
)

// init is run automatically when the module is loaded
func init() {
	var err error

	ArmaDir, err = a3interface.GetArmaDir()
	if err != nil {
		panic(err)
	}

	ModulePath = assemblyfinder.GetModulePath()
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
		defer defs.Logger.Warn().Err(err).Str("path", InitLogFilePath).
			Msg("Failed to create/open log file!")
	}
	setupLogging(InitLogFile)

	// load config
	err = loadConfig()
	if err != nil {
		defs.Logger.Warn().Err(err).Msg("Failed to load config, using defaults!")
	} else {
		defs.Logger.Info().Msg("Loaded config")
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
		defs.Logger.Error().Err(err).Str("path", OcapLogFilePath).
			Msg("Failed to create/open log file!")
	}

	defs.Logger.Info().Str("path", OcapLogFilePath).Msg("Begin logging in logs directory")

	// set up logging
	setupLogging(OcapLogFile)
	defs.Logger.Info().Str("path", OcapLogFilePath).Msg("Logging to file")

	SqliteDBFilePath = fmt.Sprintf(`%s\%s_%s.db`, AddonFolder, ExtensionName, SessionStartTime.Format("20060102_150405"))
	InfluxBackupFilePath = AddonFolder + "\\influx_backup.log.gzip"
	// set up a3interfaces
	defs.Logger.Info().Msg("Setting up a3interface...")
	err = setupA3Interface()
	if err != nil {
		defs.Logger.Fatal().Err(err).Msg("Failed to set up a3interfaces!")
	} else {
		defs.Logger.Info().Msg("Set up a3interfaces")
	}

	// get count of cpus available
	// set GOMAXPROCS to n - 2, minimum 2
	// this is to ensure we're using all available cores

	// get number of CPUs
	numCPUs := runtime.NumCPU()
	defs.Logger.Debug().Int("numCPUs", numCPUs).Msg("Number of CPUs")

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

func setupLogging(file *os.File) {

	// get log level
	var logLevelActual zerolog.Level
	switch strings.ToUpper(viper.GetString("logLevel")) {
	case "DEBUG":
		logLevelActual = zerolog.DebugLevel
	case "INFO":
		logLevelActual = zerolog.InfoLevel
	case "WARN":
		logLevelActual = zerolog.WarnLevel
	case "ERROR":
		logLevelActual = zerolog.ErrorLevel
	case "TRACE":
		logLevelActual = zerolog.TraceLevel
	default:
		logLevelActual = zerolog.InfoLevel
	}

	// remove old logs (older than 7 days)
	// removeOldLogs(viper.GetString("logsDir"), 7)

	// set up logging
	zerolog.SetGlobalLevel(logLevelActual)
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	// set up multi-level writer
	mlw := zerolog.MultiLevelWriter(
		// write console format with colors to console
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
		// write console format without colors to file
		zerolog.ConsoleWriter{
			Out:        file,
			TimeFormat: time.RFC3339,
			NoColor:    true,
		},
	)

	defs.Logger = zerolog.New(mlw).With().Timestamp().Logger().
		Hook(
			zerolog.HookFunc(
				func(e *zerolog.Event, level zerolog.Level, msg string) {
					// add current mission
					// add runtime info
					e.Bool("usingLocalDB", ShouldSaveLocal).
						Str("currentMission", CurrentMission.MissionName).
						Uint("currentMissionID", CurrentMission.ID).
						Bool("statusMonitorActive", IsStatusProcessRunning)
				}))

	defs.TraceSample = defs.Logger.With().
		Bool("asmpled", true).Logger().Sample(&zerolog.BurstSampler{
		// allow max 5 entries per 10 seconds
		// once reached, sample 1 in 100
		Burst:       5,
		Period:      10 * time.Second,
		NextSampler: &zerolog.BasicSampler{N: 100},
	})

	defs.Logger.Info().Str("loglevel", defs.Logger.GetLevel().String()).Msg("Logging set up")
}

func writeToLogIo(level zerolog.Level, msg string) {
	if LogIoConn == nil {
		return
	}
	data := make([]byte, 0)
	message := fmt.Sprintf(
		`%s %s %s`,
		time.Now().UTC().Format(time.RFC3339),
		level.String(),
		msg,
	)
	data = append(data, []byte(
		"+msg|ocap2|ocap_recorder|"+message,
	)...)
	data = append(data, NullByte)

	_, err := LogIoConn.Write(data)
	if err != nil {
		defs.Logger.Debug().Err(err).Msg("Failed to send log to log.io")
	}
}

// removeOldLogs will remove all .log and .jsonl files older than daysDelta days
func removeOldLogs(path string, daysDelta int) {
	files, err := os.ReadDir(path)
	if err != nil {
		defs.Logger.Warn().Err(err).Msg("Failed to read logs dir")
		return
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		// get file info
		r, err := f.Info()
		if err != nil {
			defs.Logger.Warn().Err(err).Msg("Failed to get file info")
			continue
		}
		// check if file is a log file and if it's older than daysDelta days
		if filepath.Ext(f.Name()) == ".log" || filepath.Ext(f.Name()) == ".jsonl" {
			if time.Since(r.ModTime()).Hours() > float64(daysDelta*24) {
				os.Remove(path + "\\" + f.Name())
			}
		}
	}
}

// //////////////////////
// A3Interface Section //
// //////////////////////
// A3Interface is the interface for interacting with Arma 3
// These variables are used to communicate with it
var (
	// RVExtArgsDataChannels is a map of channels for receiving data from RVExtensionArgs
	RVExtArgsDataChannels = map[string]chan []string{
		// channels for receiving data from Arma
		":ADDON:VERSION:": make(chan []string, 1),
		":NEW:MISSION:":   make(chan []string, 1),
		// channels for receiving new data and filing to DB
		":NEW:SOLDIER:":       make(chan []string, 1000),
		":NEW:VEHICLE:":       make(chan []string, 1000),
		":NEW:SOLDIER:STATE:": make(chan []string, 10000),
		":NEW:VEHICLE:STATE:": make(chan []string, 10000),
		":EVENT:":             make(chan []string, 1000),
		":FIRED:":             make(chan []string, 10000),
		":PROJECTILE:":        make(chan []string, 5000),
		":HIT:":               make(chan []string, 2000),
		":KILL:":              make(chan []string, 2000),
		":CHAT:":              make(chan []string, 1000),
		":RADIO:":             make(chan []string, 1000),
		":FPS:":               make(chan []string, 1000),
		":ACE3:DEATH:":        make(chan []string, 1000),
		":ACE3:UNCONSCIOUS:":  make(chan []string, 1000),
		// marker channels
		":MARKER:CREATE:": make(chan []string, 500),
		":MARKER:MOVE:":   make(chan []string, 1000),
		":MARKER:DELETE:": make(chan []string, 500),
		// metric channel
		":METRIC:": make(chan []string, 1000),
	}
	// RVExtDataChannels is a map of channels for receiving data from RVExtension
	RVExtDataChannels = map[string]chan string{
		":VERSION:":        make(chan string, 1),
		":INIT:":           make(chan string, 1),
		":INIT:DB:":        make(chan string, 1),
		":GETDIR:ARMA:":    make(chan string, 1),
		":GETDIR:MODULE:":  make(chan string, 1),
		":GETDIR:OCAPLOG:": make(chan string, 1),
	}

	// a3ErrorChan is used to send errors from the a3interface package to the main thread
	a3ErrorChan = make(chan []string, 50)
)

func initDB() (err error) {
	defs.Logger.Trace().Msg("Received :INIT:DB: call")
	loadConfig()
	functionName := ":INIT:DB:"
	DB, err = getDB()
	if err != nil || DB == nil {
		// if we couldn't connect to the database, send a callback to the addon to let it know
		writeLog(functionName, fmt.Sprintf(`Error connecting to database: %v`, err), "ERROR")
		a3interface.WriteArmaCallback(ExtensionName, ":DB:ERROR:", err.Error())
	} else {
		go func() {
			// err = setupDB(DB)
			// if err != nil {
			// 	writeLog(functionName, fmt.Sprintf(`Error setting up database: %v`, err), "ERROR")
			// }
			InfluxClient, err = connectToInflux()
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Error connecting to InfluxDB: %v`, err), "ERROR")
			}
			// only if everything worked should we send a callback letting the addon know we're ready to receive the mission data
			a3interface.WriteArmaCallback(ExtensionName, ":DB:OK:", DB.Dialector.Name())
		}()
	}
	return nil
}

func setupA3Interface() (err error) {

	a3interface.SetVersion(CurrentExtensionVersion)
	a3interface.RegisterErrorChan(a3ErrorChan)

	// register channels
	a3interface.RegisterRvExtensionChannels(RVExtDataChannels)
	a3interface.RegisterRvExtensionArgsChannels(RVExtArgsDataChannels)

	// in our case, add listeners for things not already handled by startAsyncProcessors
	go func() {
		for {
			select {
			case errData := <-a3ErrorChan:
				defs.Logger.Error().
					Str("command", errData[0]).
					Str("error", errData[1]).
					Msg("Error in A3Interface")
				// RVExtDataChannels
			case <-RVExtDataChannels[":INIT:"]:
				go initExtension()
			case <-RVExtDataChannels[":INIT:DB:"]:
				go initDB()
			case <-RVExtDataChannels[":VERSION"]:
				a3interface.WriteArmaCallback(ExtensionName, ":VERSION:", CurrentExtensionVersion)
			case <-RVExtDataChannels[":GETDIR:ARMA:"]:
				a3interface.WriteArmaCallback(ExtensionName, ":GETDIR:ARMA:", ArmaDir)
			case <-RVExtDataChannels[":GETDIR:MODULE:"]:
				a3interface.WriteArmaCallback(ExtensionName, ":GETDIR:MODULE:", ModulePath)
			case <-RVExtDataChannels[":GETDIR:OCAPLOG:"]:
				a3interface.WriteArmaCallback(ExtensionName, ":GETDIR:OCAPLOG:", OcapLogFilePath)
				// RVExtArgsDataChannels
			case data := <-RVExtArgsDataChannels[":ADDON:VERSION:"]:
				addonVersion = fixEscapeQuotes(trimQuotes(data[0]))
				defs.Logger.Info().Str("version", addonVersion).Msg("Addon version")
			case data := <-RVExtArgsDataChannels[":NEW:MISSION:"]:
				go logNewMission(data)
			}
		}
	}()

	return nil
}

func loadConfig() (err error) {
	// load config from file as JSON

	// set default values
	viper.SetDefault("logLevel", "info")
	viper.SetDefault("defaultTag", "Op")
	viper.SetDefault("logsDir", "./ocaplogs")

	viper.SetDefault("api.serverUrl", "http://localhost:5000")
	viper.SetDefault("api.apiKey", "")

	viper.SetDefault("db.host", "localhost")
	viper.SetDefault("db.port", "5432")
	viper.SetDefault("db.username", "postgres")
	viper.SetDefault("db.password", "postgres")
	viper.SetDefault("db.database", "ocap")

	viper.SetDefault("influx.enabled", true)
	viper.SetDefault("influx.host", "localhost")
	viper.SetDefault("influx.port", "8086")
	viper.SetDefault("influx.protocol", "http")
	viper.SetDefault("influx.token", "supersecrettoken")
	viper.SetDefault("influx.org", "ocap-metrics")

	viper.SetDefault("graylog.enabled", true)
	viper.SetDefault("graylog.address", "localhost:12201")

	viper.SetDefault("logio.enabled", true)
	viper.SetDefault("logio.host", "localhost")
	viper.SetDefault("logio.port", "28777")

	viper.SetConfigName("ocap_recorder.cfg.json")
	viper.AddConfigPath(AddonFolder)
	viper.SetConfigType("json")
	err = viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	return nil
}

func checkServerStatus() {
	var err error

	// check if server is running by making a healthcheck API request
	// if server is not running, log error and exit
	_, err = http.Get(viper.GetString("api.serverUrl") + "/healthcheck")
	if err != nil {
		defs.Logger.Info().Msg("OCAP Frontend is offline")
	} else {
		defs.Logger.Info().Msg("OCAP Frontend is online")
	}
}

func getProgramStatus(
	rawBuffers bool,
	writeQueues bool,
	lastWrite bool,
) (output []string, model defs.OcapPerformance) {
	// returns a slice of strings indicating raw arrays pending processing as rawBuffers
	// returns a slice of strings indicating pending data to write to DB as writeQueues
	// returns a string indicating the last write duration in ms as lastWrite

	buffersObj := defs.BufferLengths{
		Soldiers:              uint16(len(RVExtArgsDataChannels[":NEW:SOLDIER:"])),
		Vehicles:              uint16(len(RVExtArgsDataChannels[":NEW:VEHICLE:"])),
		SoldierStates:         uint16(len(RVExtArgsDataChannels[":NEW:SOLDIER:STATE:"])),
		VehicleStates:         uint16(len(RVExtArgsDataChannels[":NEW:VEHICLE:STATE:"])),
		FiredEvents:           uint16(len(RVExtArgsDataChannels[":FIRED:"])),
		GeneralEvents:         uint16(len(RVExtArgsDataChannels[":EVENT:"])),
		HitEvents:             uint16(len(RVExtArgsDataChannels[":HIT:"])),
		KillEvents:            uint16(len(RVExtArgsDataChannels[":KILL:"])),
		ChatEvents:            uint16(len(RVExtArgsDataChannels[":CHAT:"])),
		RadioEvents:           uint16(len(RVExtArgsDataChannels[":RADIO:"])),
		ServerFpsEvents:       uint16(len(RVExtArgsDataChannels[":FPS:"])),
		Ace3DeathEvents:       uint16(len(RVExtArgsDataChannels[":ACE3:DEATH:"])),
		Ace3UnconsciousEvents: uint16(len(RVExtArgsDataChannels[":ACE3:UNCONSCIOUS:"])),
	}

	writeQueuesObj := defs.WriteQueueLengths{
		Soldiers:              uint16(soldiersToWrite.Len()),
		Vehicles:              uint16(vehiclesToWrite.Len()),
		SoldierStates:         uint16(soldierStatesToWrite.Len()),
		VehicleStates:         uint16(vehicleStatesToWrite.Len()),
		FiredEvents:           uint16(firedEventsToWrite.Len()),
		GeneralEvents:         uint16(generalEventsToWrite.Len()),
		HitEvents:             uint16(hitEventsToWrite.Len()),
		KillEvents:            uint16(killEventsToWrite.Len()),
		ChatEvents:            uint16(chatEventsToWrite.Len()),
		RadioEvents:           uint16(radioEventsToWrite.Len()),
		ServerFpsEvents:       uint16(fpsEventsToWrite.Len()),
		Ace3DeathEvents:       uint16(ace3DeathEventsToWrite.Len()),
		Ace3UnconsciousEvents: uint16(ace3UnconsciousEventsToWrite.Len()),
	}

	perf := defs.OcapPerformance{
		Time:              time.Now(),
		Mission:           *CurrentMission,
		BufferLengths:     buffersObj,
		WriteQueueLengths: writeQueuesObj,
		// get float32 in ms
		LastWriteDurationMs: float32(LastDBWriteDuration.Milliseconds()),
	}

	if rawBuffers {
		rawBuffersStr, err := json.MarshalIndent(buffersObj, "", "  ")
		if err != nil {
			rawBuffersStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(rawBuffersStr))
	}
	if writeQueues {
		writeQueuesStr, err := json.MarshalIndent(writeQueuesObj, "", "  ")
		if err != nil {
			writeQueuesStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(writeQueuesStr))
	}
	if lastWrite {
		lastWriteStr, err := json.MarshalIndent(perf.LastWriteDurationMs, "", "  ")
		if err != nil {
			lastWriteStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(lastWriteStr))
	}

	return output, perf
}

// /////////////////////
// INFLUXDB OPS //
// /////////////////////
// return db client and error
func connectToInflux() (influxdb2.Client, error) {

	if !viper.GetBool("influx.enabled") {
		return nil, errors.New("influxdb.Enabled is false")
	}

	InfluxClient = influxdb2.NewClientWithOptions(
		fmt.Sprintf(
			"%s://%s:%s",
			viper.GetString("influx.protocol"),
			viper.GetString("influx.host"),
			viper.GetString("influx.port"),
		),
		viper.GetString("influx.token"),
		influxdb2.DefaultOptions().
			SetBatchSize(2500).
			SetFlushInterval(1000),
	)

	// validate client connection health
	running, err := InfluxClient.Ping(context.Background())

	if err != nil || !running {
		IsInfluxValid = false
		// create backup writer
		if InfluxBackupWriter == nil {
			writeLog("connectToInflux", fmt.Sprintf(`Failed to initialize InfluxDB client, writing to backup file: %s`, InfluxBackupFilePath), "INFO")

			// create if not exists
			file, err := os.OpenFile(InfluxBackupFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, fmt.Errorf("error creating backup file: %v", err)
			}
			InfluxBackupWriter = gzip.NewWriter(file)
			if err != nil {
				return nil, fmt.Errorf("error creating gzip writer: %v", err)
			}
		}
	} else {
		IsInfluxValid = true
	}

	if IsInfluxValid {
		// ensure org exists
		ctx := context.Background()
		_, err = InfluxClient.OrganizationsAPI().
			FindOrganizationByName(ctx, viper.GetString("influx.org"))
		if err != nil {
			writeLog("connectToInflux", fmt.Sprintf(`Organization not found, creating: %s`, viper.GetString("influx.org")), "INFO")
			_, err = InfluxClient.OrganizationsAPI().
				CreateOrganizationWithName(ctx, viper.GetString("influx.org"))
			if err != nil {
				writeLog("connectToInflux", fmt.Sprintf(`Error creating organization: %s`, err), "ERROR")
				return nil, err
			}
		}

		// get influxOrg
		influxOrg, err := InfluxClient.OrganizationsAPI().
			FindOrganizationByName(ctx, viper.GetString("influx.org"))
		if err != nil {
			writeLog("connectToInflux", fmt.Sprintf(`Error getting organization: %s`, err), "ERROR")
			return nil, err
		}

		// ensure buckets exist with 90 day retention
		for _, bucket := range InfluxBucketNames {
			_, err = InfluxClient.BucketsAPI().
				FindBucketByName(ctx, bucket)
			if err != nil {
				writeLog("connectToInflux", fmt.Sprintf(`Bucket not found, creating: %s`, bucket), "INFO")

				rule := domain.RetentionRuleTypeExpire
				_, err = InfluxClient.BucketsAPI().
					CreateBucketWithName(ctx, influxOrg, bucket, domain.RetentionRule{
						Type:         &rule,
						EverySeconds: 60 * 60 * 24 * 90, // 90 days
					})
				if err != nil {
					writeLog("connectToInflux", fmt.Sprintf(`Error creating bucket: %s`, err), "ERROR")
					return nil, err
				}
			}
		}

		// create influx writers
		createInfluxWriters()
		writeLog("connectToInflux", `InfluxDB client initialized`, "INFO")
	} else {
		writeLog("connectToInflux", `InfluxDB client failed to initialize, using backup writer`, "WARN")
	}

	return InfluxClient, nil
}

func createInfluxWriters() {
	// create influx writers
	for _, bucket := range InfluxBucketNames {
		defs.Logger.Trace().Msgf("Creating InfluxDB writer for bucket '%s'", bucket)
		defs.JSONLogger.Trace().Msgf(`Creating InfluxDB writer for bucket '%s'`, bucket)
		InfluxWriters[bucket] = InfluxClient.WriteAPI(
			viper.GetString("influx.org"),
			bucket,
		)
		errorsCh := InfluxWriters[bucket].Errors()
		go func(bucketName string, errorsCh <-chan error) {
			for writeErr := range errorsCh {
				writeLog(bucketName, fmt.Sprintf(`Error sending data to InfluxDB: %s`, writeErr.Error()), "ERROR")
			}
		}(bucket, errorsCh)
		defs.Logger.Trace().Msgf("InfluxDB writer for bucket '%s' created", bucket)
		defs.JSONLogger.Trace().Msgf(`InfluxDB writer for bucket '%s' created`, bucket)
	}

	defs.Logger.Debug().Msg("InfluxDB writers initialized")
	defs.JSONLogger.Debug().Msg("InfluxDB writers initialized")
}

func writeInfluxPoint(
	ctx context.Context,
	bucket string,
	point *influxdb2_write.Point,
) error {

	if IsInfluxValid {
		defs.TraceSample.Trace().Msgf("Writing point to InfluxDB bucket '%s'", bucket)
		if _, ok := InfluxWriters[bucket]; !ok {
			defs.TraceSample.Warn().Msgf("InfluxDB bucket '%s' not registered, skipping", bucket)
			return fmt.Errorf("influxDB bucket '%s' not registered", bucket)
		}

		// write to influx
		InfluxWriters[bucket].WritePoint(point)
		defs.TraceSample.Trace().Msgf("Point written to InfluxDB bucket '%s'", bucket)
	} else {
		if InfluxBackupWriter == nil {
			return fmt.Errorf("influxDB client not initialized and backup writer not available")
		}

		// write to backup file
		defs.TraceSample.Trace().Msgf("Writing point to InfluxDB backup file")
		lineProtocol := influxdb2_write.PointToLineProtocol(
			point, time.Duration(1*time.Nanosecond),
		)
		_, err := InfluxBackupWriter.Write([]byte(lineProtocol + "\n"))
		if err != nil {
			return fmt.Errorf("error writing to InfluxDB backup file: %s", err)
		}
		defs.TraceSample.Trace().Msgf("Point written to InfluxDB backup file")
	}

	return nil
}

func processMetricData(data []string) (
	bucket string,
	point *influxdb2_write.Point,
	err error,
) {
	functionName := ":METRIC:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// each metric will come through as a string array
	// 0 = bucket name
	// 1 = measurement name
	// n with "tag" prefix = tag name
	// n with "field" prefix = field
	// tag and field values use "::" separator

	// get bucket name
	bucket = data[0]

	// get measurement name
	measurementName := data[1]

	// create point
	point = influxdb2_write.NewPointWithMeasurement(measurementName)

	// add tags
	for _, tag := range data[2:] {
		if !strings.HasPrefix(tag, "tag::") {
			continue
		}
		tagName := strings.Split(tag, "::")[1]
		tagValue := strings.Split(tag, "::")[2]
		point.AddTag(tagName, tagValue)
	}

	// add fields
	for _, field := range data[2:] {
		if !strings.HasPrefix(field, "field::") {
			continue
		}
		fieldType := strings.Split(field, "::")[1]
		fieldName := strings.Split(field, "::")[2]
		fieldValue := strings.Split(field, "::")[3]
		switch fieldType {
		case "string":
			point.AddField(fieldName, fieldValue)
		case "int":
			tagValueInt, err := strconv.Atoi(fieldValue)
			if err != nil {
				defs.Logger.Error().Err(err).Str("function", functionName).Msgf("Error converting tag value '%s' to int", fieldValue)
				return "", nil, err
			}
			point.AddField(fieldName, tagValueInt)
		case "float":
			tagValueFloat, err := strconv.ParseFloat(fieldValue, 64)
			if err != nil {
				defs.Logger.Error().Err(err).Str("function", functionName).Msgf("Error converting tag value '%s' to float", fieldValue)
				return "", nil, err
			}
			point.AddField(fieldName, tagValueFloat)
		}
	}

	return bucket, point, nil
}

///////////////////////
// DATABASE OPS //
///////////////////////

func getSqliteDB(path string) (db *gorm.DB, err error) {
	functionName := "getSqliteDB"

	// if path is an empty string, use a memory db
	// otherwise, use the path provided (like in retrieving backups)
	if path != "" {
		// connect to database (SQLite)
		db, err = gorm.Open(sqlite.Open(path), &gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
			CreateBatchSize:        2000,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			IsDatabaseValid = false
			return nil, err
		}
		writeLog(functionName, fmt.Sprintf("Using local SQlite DB at '%s'", path), "INFO")
	} else {
		// connect to database (SQLite)
		db, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
			PrepareStmt:            true,
			SkipDefaultTransaction: true,
			CreateBatchSize:        2000,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			IsDatabaseValid = false
			return nil, err
		}
		writeLog(functionName, "Using local SQlite DB in memory with periodic disk dump", "INFO")
	}

	// set PRAGMAS
	err = db.Exec("PRAGMA user_version = 1;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting user_version PRAGMA: %s", err)
	}
	err = db.Exec("PRAGMA journal_mode = MEMORY;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting journal_mode PRAGMA: %s", err)
	}
	err = db.Exec("PRAGMA synchronous = OFF;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting synchronous PRAGMA: %s", err)
	}
	err = db.Exec("PRAGMA cache_size = -32000;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting cache_size PRAGMA: %s", err)
	}
	err = db.Exec("PRAGMA temp_store = MEMORY;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting temp_store PRAGMA: %s", err)
	}

	err = db.Exec("PRAGMA page_size = 32768;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting page_size PRAGMA: %s", err)
	}

	err = db.Exec("PRAGMA mmap_size = 30000000000;").Error
	if err != nil {
		return nil, fmt.Errorf("error setting mmap_size PRAGMA: %s", err)
	}

	return db, nil
}

func getBackupDBPaths() (dbPaths []string, err error) {
	// search addon folder for .db files
	path := AddonFolder
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	// filter out non .db files
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".db") {
			// return path of file
			dbPaths = append(dbPaths, filepath.Join(path, file.Name()))
		}
	}
	return dbPaths, nil
}

func dumpMemoryDBToDisk() (err error) {
	functionName := "dumpMemoryDBToDisk"
	// remove existing file if it exists
	exists, err := os.Stat(SqliteDBFilePath)
	if err == nil {
		if exists != nil {
			err = os.Remove(SqliteDBFilePath)
			if err != nil {
				writeLog(functionName, "Error removing existing DB file", "ERROR")
				return err
			}
		}
	}

	// dump memory DB to disk
	start := time.Now()
	err = DB.Exec("VACUUM INTO 'file:" + SqliteDBFilePath + "';").Error
	if err != nil {
		writeLog(functionName, "Error dumping memory DB to disk", "ERROR")
		return err
	}
	writeLog(functionName, fmt.Sprintf(`Dumped memory DB to disk in %s`, time.Since(start)), "INFO")
	defs.Logger.Debug().
		Dur("duration", time.Since(start)).Msg("Dumped memory DB to disk")
	defs.JSONLogger.Debug().
		Dur("duration", time.Since(start)).Msg("Dumped memory DB to disk")
	return nil
}

func getPostgresDB() (db *gorm.DB, err error) {
	// connect to database (Postgres) using gorm
	dsn := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable`,
		viper.GetString("db.host"),
		viper.GetString("db.port"),
		viper.GetString("db.username"),
		viper.GetString("db.password"),
		viper.GetString("db.database"),
	)

	defs.Logger.Debug().Msgf("Connecting to Postgres DB at '%s'", dsn)

	db, err = gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{
		// PrepareStmt:            true,
		SkipDefaultTransaction: true,
		CreateBatchSize:        10000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// getDB connects to the Postgres database, and if it fails, it will use a local SQlite DB
func getDB() (db *gorm.DB, err error) {
	functionName := ":INIT:DB:"

	db, err = getPostgresDB()
	if err != nil {
		defs.Logger.Error().Err(err).Msg("Failed to connect to Postgres DB, trying SQLite")
		defs.JSONLogger.Error().Err(err).Msg("Failed to connect to Postgres DB, trying SQLite")
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
		writeLog(functionName, fmt.Sprintf(`Failed to validate connection. Err: %s`, err), "ERROR")
		ShouldSaveLocal = true
		db, err = getSqliteDB("")
		if err != nil || db == nil {
			IsDatabaseValid = false
			return nil, fmt.Errorf("failed to get local SQLite DB: %s", err)
		}
		IsDatabaseValid = true

	} else {
		writeLog(functionName, "Connected to database", "INFO")
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
	if !db.Migrator().HasTable(&defs.OcapInfo{}) {
		// Create the table
		err = db.AutoMigrate(&defs.OcapInfo{})
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create ocap_info table. Err: %s`, err), "ERROR")
			IsDatabaseValid = false
			return err
		}
		// Create the default settings
		err = db.Create(&defs.OcapInfo{
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
		writeLog(functionName, "PostGIS Extension created", "INFO")
	}

	defs.Logger.Info().Msg("Migrating schema")
	if ShouldSaveLocal {
		err = db.AutoMigrate(defs.DatabaseModelsSQLite...)
	} else {
		err = db.AutoMigrate(defs.DatabaseModels...)
	}
	if err != nil {
		IsDatabaseValid = false
		return fmt.Errorf("failed to migrate schema: %s", err)
	}

	defs.Logger.Info().Msg("Database setup complete")
	return nil
}

func startGoroutines() (err error) {

	functionName := "startGoroutines"

	defs.Logger.Trace().Msg("Starting async processors")
	startAsyncProcessors()
	defs.Logger.Trace().Msg("Starting DB writers")
	startDBWriters()

	if !IsStatusProcessRunning {
		defs.Logger.Trace().Msg("Status process not runnning, starting it")
		// start status monitor
		startStatusMonitor()
	}

	// goroutine to, every x seconds, pause insert execution and dump memory sqlite db to disk
	go func() {
		defs.Logger.Debug().
			Str("function", functionName).
			Msg("Starting DB dump goroutine")

		for {

			time.Sleep(3 * time.Minute)
			if !ShouldSaveLocal {
				continue
			}

			// pause insert execution
			DBInsertsPaused = true

			// dump memory sqlite db to disk
			writeLog(functionName, "Dumping in-memory SQLite DB to disk", "DEBUG")
			err = dumpMemoryDBToDisk()
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Error dumping memory db to disk: %v`, err), "ERROR")
			}

			// resume insert execution
			DBInsertsPaused = false
		}
	}()

	// log post goroutine creation
	writeLog(functionName, "Goroutines started successfully", "INFO")
	return nil
}

func validateHypertables(tables map[string][]string) (err error) {
	functionName := "validateHypertables"
	// HYPERTABLES

	all := []interface{}{}
	DB.Exec(`SELECT x.* FROM timescaledb_information.hypertables`).Scan(&all)
	for _, row := range all {
		writeLog(functionName, fmt.Sprintf(`hypertable row: %v`, row), "DEBUG")
	}

	// iterate through each provided table
	for table := range tables {
		hypertable := interface{}(nil)
		// see if table is already configured
		DB.Exec(`SELECT x.* FROM timescaledb_information.hypertables WHERE hypertable_name = ?`, table).Scan(&hypertable)
		if hypertable != nil {
			// table is already configured
			writeLog(functionName, fmt.Sprintf(`Table %s is already configured`, table), "INFO")
			continue
		}

		// if table doesn't exist, create it
		queryCreateHypertable := fmt.Sprintf(`
				SELECT create_hypertable('%s', 'time', chunk_time_interval => interval '1 day', if_not_exists => true);
			`, table)
		err = DB.Exec(queryCreateHypertable).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create hypertable for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		writeLog(functionName, fmt.Sprintf(`Created hypertable for %s`, table), "INFO")

		// set compression
		queryCompressHypertable := fmt.Sprintf(`
				ALTER TABLE %s SET (
					timescaledb.compress,
					timescaledb.compress_segmentby = ?);
			`, table)
		err = DB.Exec(
			queryCompressHypertable,
			strings.Join(tables[table], ","),
		).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to enable compression for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		writeLog(functionName, fmt.Sprintf(`Enabled hypertable compression for %s`, table), "INFO")

		// set compress_after
		queryCompressAfterHypertable := fmt.Sprintf(`
				SELECT add_compression_policy(
					'%s',
					compress_after => interval '14 day');
			`, table)
		err = DB.Exec(queryCompressAfterHypertable).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to set compress_after for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		writeLog(functionName, fmt.Sprintf(`Set compress_after for %s`, table), "INFO")

	}
	return nil
}

// start a goroutine that will output channel lengths to status.txt and influxdb every X time
func startStatusMonitor() {
	go func() {
		IsStatusProcessRunning = true
		defer func() {
			IsStatusProcessRunning = false
		}()

		defs.Logger.Debug().
			Str("function", "startStatusMonitor").
			Msg("Starting status monitor goroutine")

		// get status file writer with full control of file
		defs.Logger.Trace().
			Str("function", "startStatusMonitor").
			Str("file", AddonFolder+"/status.txt")

		statusFile, err := os.Create(AddonFolder + "/status.txt")
		if err != nil {
			defs.Logger.Error().Err(err).Msg("Error creating status file")
		}
		defer statusFile.Close()

		for {
			time.Sleep(1000 * time.Millisecond)

			if CurrentMission.ID == 0 {
				continue
			}

			statusStr, perfModel := getProgramStatus(true, true, true)

			if statusFile != nil {
				// clear the file contents and then write status
				statusFile.Truncate(0)
				statusFile.Seek(0, 0)
				for _, line := range statusStr {
					statusFile.WriteString(line + "\n")
				}
			}

			// ! write model to Postgres
			if IsDatabaseValid {
				err = DB.Create(&perfModel).Error
				if err != nil {
					defs.Logger.Error().Err(err).Msg("Error writing perf model to Postgres")
				}
			}

			// write to influxDB
			if viper.GetBool("influx.enabled") {

				// write buffer lengths
				p := influxdb2.NewPointWithMeasurement(
					"ext_buffer_lengths",
				).
					AddTag("db_url", viper.GetString("influxdb.url")).
					AddTag("mission_name", perfModel.Mission.MissionName).
					AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
					AddField("soldiers", perfModel.BufferLengths.Soldiers).
					AddField("vehicles", perfModel.BufferLengths.Vehicles).
					AddField("soldier_states", perfModel.BufferLengths.SoldierStates).
					AddField("vehicle_states", perfModel.BufferLengths.VehicleStates).
					AddField("general_events", perfModel.BufferLengths.GeneralEvents).
					AddField("hit_events", perfModel.BufferLengths.HitEvents).
					AddField("kill_events", perfModel.BufferLengths.KillEvents).
					AddField("fired_events", perfModel.BufferLengths.FiredEvents).
					AddField("chat_events", perfModel.BufferLengths.ChatEvents).
					AddField("radio_events", perfModel.BufferLengths.RadioEvents).
					AddField("server_fps_events", perfModel.BufferLengths.ServerFpsEvents).
					SetTime(time.Now())

				writeInfluxPoint(
					context.Background(),
					"ocap_performance",
					p,
				)

				// write db write queue lengths
				p = influxdb2.NewPointWithMeasurement(
					"ext_db_queue_lengths",
				).
					AddTag("db_url", viper.GetString("influxdb.url")).
					AddTag("mission_name", perfModel.Mission.MissionName).
					AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
					AddField("soldiers", perfModel.WriteQueueLengths.Soldiers).
					AddField("vehicles", perfModel.WriteQueueLengths.Vehicles).
					AddField("soldier_states", perfModel.WriteQueueLengths.SoldierStates).
					AddField("vehicle_states", perfModel.WriteQueueLengths.VehicleStates).
					AddField("general_events", perfModel.WriteQueueLengths.GeneralEvents).
					AddField("hit_events", perfModel.WriteQueueLengths.HitEvents).
					AddField("kill_events", perfModel.WriteQueueLengths.KillEvents).
					AddField("fired_events", perfModel.WriteQueueLengths.FiredEvents).
					AddField("chat_events", perfModel.WriteQueueLengths.ChatEvents).
					AddField("radio_events", perfModel.WriteQueueLengths.RadioEvents).
					AddField("server_fps_events", perfModel.WriteQueueLengths.ServerFpsEvents).
					SetTime(time.Now())

				writeInfluxPoint(
					context.Background(),
					"ocap_performance",
					p,
				)

				// write last write duration
				p = influxdb2.NewPointWithMeasurement(
					"ext_db_lastwrite_duration_ms",
				).
					AddTag("db_url", viper.GetString("influxdb.url")).
					AddTag("mission_name", perfModel.Mission.MissionName).
					AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
					AddField("value", perfModel.LastWriteDurationMs).
					SetTime(time.Now())

				writeInfluxPoint(
					context.Background(),
					"ocap_performance",
					p,
				)
			}
		}
	}()
}

/////////////////////////////////////
// WORLDS AND MISSIONS
/////////////////////////////////////

// CurrentWorld is the world that is currently being played
var CurrentWorld *defs.World = &defs.World{
	WorldName: "No world loaded",
}

// CurrentMission is the mission that is currently being played
var CurrentMission *defs.Mission = &defs.Mission{
	MissionName: "No mission loaded",
}

// logNewMission logs a new mission to the database and associates the world it's played on
func logNewMission(data []string) (err error) {
	functionName := ":NEW:MISSION:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	world := defs.World{}
	mission := defs.Mission{}
	// unmarshal data[0]
	err = json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling world data: %v`, err), "ERROR")
		return err
	}

	// preprocess the world 'location' to geopoint
	worldLocation, err := defs.Coords3857From4326(
		float64(world.Longitude),
		float64(world.Latitude),
	)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting world location to geopoint: %v`, err), "ERROR")
		return err
	}
	world.Location = worldLocation

	// unmarshal data[1]
	// unmarshal to temp object too to extract addons
	missionTemp := map[string]interface{}{}
	if err = json.Unmarshal([]byte(data[1]), &missionTemp); err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling mission data: %v`, err), "ERROR")
		return err
	}

	// add addons
	addons := []defs.Addon{}
	for _, addon := range missionTemp["addons"].([]interface{}) {
		thisAddon := defs.Addon{
			Name: addon.([]interface{})[0].(string),
		}
		// if addon[1] workshopId is int, convert to string
		if reflect.TypeOf(addon.([]interface{})[1]).Kind() == reflect.Float64 {
			thisAddon.WorkshopID = strconv.Itoa(int(addon.([]interface{})[1].(float64)))
		} else {
			thisAddon.WorkshopID = addon.([]interface{})[1].(string)
		}

		// if addon doesn't exist, insert it
		err = DB.Where("name = ?", thisAddon.Name).First(&thisAddon).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			writeLog(functionName, fmt.Sprintf(`Error checking if addon exists: %v`, err), "ERROR")
			return err
		}
		if thisAddon.ID == 0 {
			// addon does not exist, create it
			if err = DB.Create(&thisAddon).Error; err != nil {
				writeLog(functionName, fmt.Sprintf(`Error creating addon: %v`, err), "ERROR")
				return err
			}
			addons = append(addons, thisAddon)

		} else {
			// addon exists, append it
			addons = append(addons, thisAddon)
		}
	}
	mission.Addons = addons

	mission.StartTime = time.Now()

	mission.CaptureDelay = float32(missionTemp["captureDelay"].(float64))
	mission.MissionNameSource = missionTemp["missionNameSource"].(string)
	mission.MissionName = missionTemp["missionName"].(string)
	mission.BriefingName = missionTemp["briefingName"].(string)
	mission.ServerName = missionTemp["serverName"].(string)
	mission.ServerProfile = missionTemp["serverProfile"].(string)
	mission.OnLoadName = missionTemp["onLoadName"].(string)
	mission.Author = missionTemp["author"].(string)
	mission.Tag = missionTemp["tag"].(string)

	// playableSlots
	playableSlotsJSON := missionTemp["playableSlots"].([]interface{})
	mission.PlayableSlots.East = uint8(playableSlotsJSON[0].(float64))
	mission.PlayableSlots.West = uint8(playableSlotsJSON[1].(float64))
	mission.PlayableSlots.Independent = uint8(playableSlotsJSON[2].(float64))
	mission.PlayableSlots.Civilian = uint8(playableSlotsJSON[3].(float64))
	mission.PlayableSlots.Logic = uint8(playableSlotsJSON[4].(float64))

	// sideFriendly
	sideFriendlyJSON := missionTemp["sideFriendly"].([]interface{})
	mission.SideFriendly.EastWest = sideFriendlyJSON[0].(bool)
	mission.SideFriendly.EastIndependent = sideFriendlyJSON[1].(bool)
	mission.SideFriendly.WestIndependent = sideFriendlyJSON[2].(bool)

	// received at extension init and saved to local memory
	mission.AddonVersion = addonVersion

	// get or insert world
	created, err := world.GetOrInsert(DB)
	if err != nil {
		defs.Logger.Error().Err(err).Msgf("Failed to get or insert world")
		return err
	}
	if created {
		defs.Logger.Debug().Msgf("New world inserted: %s", world.WorldName)
	} else {
		defs.Logger.Debug().Msgf("World already exists: %s", world.WorldName)
	}

	// always write new mission
	mission.World = world
	err = DB.Create(&mission).Error
	if err != nil {
		defs.Logger.Error().Err(err).Msgf("Failed to insert new mission")
		return err
	} else {
		defs.Logger.Debug().Msgf("New mission inserted: %s", mission.MissionName)
	}

	// set current world and mission
	CurrentWorld = &world
	CurrentMission = &mission

	// write to log
	writeLog(functionName, `New mission logged`, "INFO")

	defs.Logger.Debug().Dict("worldData", zerolog.Dict().
		Str("worldName", world.WorldName).
		Str("displayName", world.DisplayName),
	).Send()
	defs.Logger.Debug().Dict(
		"missionData", zerolog.Dict().
			Str("missionName", mission.MissionName).
			Str("briefingName", mission.BriefingName).
			Str("serverName", mission.ServerName).
			Str("serverProfile", mission.ServerProfile).
			Str("onLoadName", mission.OnLoadName).
			Str("author", mission.Author).
			Str("tag", mission.Tag),
	).Send()

	// callback to addon to begin sending data
	a3interface.WriteArmaCallback(ExtensionName, `:MISSION:OK:`, "OK")
	return nil
}

// (UNITS) AND VEHICLES

// logNewSoldier logs a new soldier to the database
func logNewSoldier(data []string) (soldier defs.Soldier, err error) {
	functionName := ":NEW:SOLDIER:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return soldier, err
	}

	// parse array
	soldier.MissionID = CurrentMission.ID
	soldier.JoinFrame = uint(capframe)

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return soldier, err
	}
	soldier.JoinTime = time.Unix(0, timestampInt)

	ocapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return soldier, err
	}
	soldier.OcapID = uint16(ocapID)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, err = strconv.ParseBool(data[5])
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
		return soldier, err
	}
	soldier.RoleDescription = data[6]
	soldier.ClassName = data[7]
	soldier.DisplayName = data[8]
	// player uid
	soldier.PlayerUID = data[9]

	// marshal squadparams
	squadParams := data[10]
	err = json.Unmarshal([]byte(squadParams), &soldier.SquadParams)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling squadParams: %v`, err), "ERROR")
		return soldier, err
	}

	return soldier, nil
}

// logSoldierState logs a SoldierState state to the database
func logSoldierState(data []string) (soldierState defs.SoldierState, err error) {
	functionName := ":NEW:SOLDIER:STATE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	soldierState.MissionID = CurrentMission.ID

	frameStr := data[8]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return soldierState, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}
	soldierState.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		return soldierState, fmt.Errorf(`error converting ocapId to uint: %v`, err)
	}

	// try and find soldier in DB to associate
	soldier, ok := EntityCache.GetSoldier(uint16(ocapID))
	if !ok {
		return soldierState, fmt.Errorf("soldier %d not found in cache", ocapID)
	}

	soldierState.SoldierID = soldier.ID

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return soldierState, err
	}
	soldierState.Time = time.Unix(0, timestampInt)

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := defs.Coord3857FromString(pos)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return soldierState, err
	}
	// fmt.Println(point.ToString())
	soldierState.Position = point
	soldierState.ElevationASL = float32(elev)

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	soldierState.Bearing = uint16(bearing)
	// lifestate
	lifeState, _ := strconv.Atoi(data[3])
	soldierState.Lifestate = uint8(lifeState)
	// in vehicle
	soldierState.InVehicle, _ = strconv.ParseBool(data[4])
	// name
	soldierState.UnitName = data[5]
	// is player
	isPlayer, err := strconv.ParseBool(data[6])
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
		return soldierState, err
	}
	soldierState.IsPlayer = isPlayer
	// current role
	soldierState.CurrentRole = data[7]

	// parse ace3/medical data, is true/false default for vanilla
	// has stable vitals
	hasStableVitals, _ := strconv.ParseBool(data[9])
	soldierState.HasStableVitals = hasStableVitals
	// is dragged/carried
	isDraggedCarried, _ := strconv.ParseBool(data[10])
	soldierState.IsDraggedCarried = isDraggedCarried

	// player scores come in as an array
	if isPlayer {
		scoresStr := data[11]
		scoresArr := strings.Split(scoresStr, ",")
		scoresInt := make([]uint8, len(scoresArr))
		for i, v := range scoresArr {
			num, _ := strconv.Atoi(v)
			scoresInt[i] = uint8(num)
		}

		soldierState.Scores = defs.SoldierScores{
			InfantryKills: scoresInt[0],
			VehicleKills:  scoresInt[1],
			ArmorKills:    scoresInt[2],
			AirKills:      scoresInt[3],
			Deaths:        scoresInt[4],
			TotalScore:    scoresInt[5],
		}
	}

	// seat in vehicle
	soldierState.VehicleRole = data[12]

	// InVehicleObjectID, if -1 not in a vehicle
	inVehicleID, _ := strconv.Atoi(data[13])
	if inVehicleID == -1 {
		soldierState.InVehicleObjectID = sql.NullInt32{
			Int32: 0,
			Valid: false,
		}
	} else {
		soldierState.InVehicleObjectID = sql.NullInt32{
			Int32: int32(inVehicleID),
			Valid: true,
		}
	}

	// stance
	soldierState.Stance = data[14]

	return soldierState, nil
}

// log a new vehicle
func logNewVehicle(data []string) (vehicle defs.Vehicle, err error) {
	functionName := ":NEW:VEHICLE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return vehicle, err
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return vehicle, err
	}
	vehicle.JoinTime = time.Unix(0, timestampInt)

	// parse array
	vehicle.MissionID = CurrentMission.ID
	vehicle.JoinFrame = uint(capframe)
	ocapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapID to uint: %v`, err), "ERROR")
		return vehicle, err
	}
	vehicle.OcapID = uint16(ocapID)
	vehicle.OcapType = data[2]
	vehicle.DisplayName = data[3]
	vehicle.ClassName = data[4]
	vehicle.Customization = data[5]

	return vehicle, nil
}

func logVehicleState(data []string) (vehicleState defs.VehicleState, err error) {
	functionName := ":NEW:VEHICLE:STATE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	vehicleState.MissionID = CurrentMission.ID

	// get frame
	frameStr := data[5]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return vehicleState, err
	}

	// try and find vehicle in DB to associate
	vehicle, ok := EntityCache.GetVehicle(uint16(ocapID))
	if !ok {
		return vehicleState, fmt.Errorf("vehicle %d not found in cache", ocapID)
	}
	vehicleState.VehicleID = vehicle.ID

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.Time = time.Unix(0, timestampInt)

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := defs.Coord3857FromString(pos)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return vehicleState, err
	}
	// fmt.Println(point.ToString())
	vehicleState.Position = point
	vehicleState.ElevationASL = float32(elev)

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	vehicleState.Bearing = uint16(bearing)
	// is alive
	isAlive, _ := strconv.ParseBool(data[3])
	vehicleState.IsAlive = isAlive
	// parse crew, which is an array of ocap ids of soldiers
	crew := data[4]
	crew = strings.TrimPrefix(crew, "[")
	crew = strings.TrimSuffix(crew, "]")
	vehicleState.Crew = crew

	// fuel
	fuel, err := strconv.ParseFloat(data[6], 32)
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting fuel to float: %v`, err)
	}
	vehicleState.Fuel = float32(fuel)

	// damage
	damage, err := strconv.ParseFloat(data[7], 32)
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting damage to float: %v`, err)
	}
	vehicleState.Damage = float32(damage)

	// isEngineOn
	isEngineOn, err := strconv.ParseBool(data[8])
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting isEngineOn to bool: %v`, err)
	}
	vehicleState.EngineOn = isEngineOn

	// locked
	locked, err := strconv.ParseBool(data[9])
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting locked to bool: %v`, err)
	}
	vehicleState.Locked = locked

	vehicleState.Side = data[10]

	vehicleState.VectorDir = data[11]
	vehicleState.VectorUp = data[12]

	turretAzimuth, err := strconv.ParseFloat(data[13], 32)
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting turretAzimuth to float: %v`, err)
	}
	vehicleState.TurretAzimuth = float32(turretAzimuth)

	turretElevation, err := strconv.ParseFloat(data[14], 32)
	if err != nil {
		return vehicleState, fmt.Errorf(`error converting turretElevation to float: %v`, err)
	}
	vehicleState.TurretElevation = float32(turretElevation)

	return vehicleState, nil
}

// FIRED EVENTS
func logFiredEvent(data []string) (firedEvent defs.FiredEvent, err error) {
	functionName := ":FIRED:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	firedEvent.MissionID = CurrentMission.ID

	// get frame
	frameStr := data[1]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapID to uint: %v`, err), "ERROR")
		return firedEvent, err
	}

	// try and find soldier in DB to associate
	soldier, ok := EntityCache.GetSoldier(uint16(ocapID))
	if !ok {
		return firedEvent, fmt.Errorf("soldier %d not found in cache", ocapID)
	}
	firedEvent.SoldierID = soldier.ID

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.Time = time.Unix(0, timestampInt)

	// parse BULLET END POS from an arma string
	endpos := data[2]
	endpoint, endelev, err := defs.Coord3857FromString(endpos)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.EndPosition = endpoint
	firedEvent.EndElevationASL = float32(endelev)

	// parse BULLET START POS from an arma string
	startpos := data[3]
	startpoint, startelev, err := defs.Coord3857FromString(startpos)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.StartPosition = startpoint
	firedEvent.StartElevationASL = float32(startelev)

	// weapon name
	firedEvent.Weapon = data[4]
	// magazine name
	firedEvent.Magazine = data[5]
	// firing mode
	firedEvent.FiringMode = data[6]

	return firedEvent, nil
}

func logProjectileEvent(data []string) (projectileEvent defs.ProjectileEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	projectileEvent.MissionID = CurrentMission.ID

	// this event will be sent as JSON, so we need to unmarshal it

	defs.Logger.Trace().Msgf("Projectile data: %v", data[0])

	var rawJsonData map[string]interface{}
	err = json.Unmarshal([]byte(data[0]), &rawJsonData)
	if err != nil {
		return projectileEvent, fmt.Errorf(`error unmarshalling json data: %v`, err)
	}

	// get data from json - this is done bit by bit to avoid errors if the data is missing or mismatching type due to SQF vs Go

	defs.Logger.Trace().Msg("Processing time and frame")
	firedTime := rawJsonData["firedTime"].(string)
	firedTimeInt, err := strconv.ParseInt(firedTime, 10, 64)
	if err != nil {
		return projectileEvent, fmt.Errorf(`error converting firedTime to int: %v`, err)
	}
	projectileEvent.Time = time.Unix(0, firedTimeInt)
	projectileEvent.CaptureFrame = uint(rawJsonData["firedFrame"].(float64))

	defs.Logger.Trace().Msg("Processing soldierFired")
	soldierFired, ok := EntityCache.GetSoldier(uint16(rawJsonData["firerID"].(float64)))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d not found in cache", uint16(rawJsonData["firerID"].(float64)))
	}
	projectileEvent.FirerID = soldierFired.ID

	defs.Logger.Trace().Msg("Processing actualFirer")
	actualFirer, ok := EntityCache.GetSoldier(uint16(rawJsonData["remoteControllerID"].(float64)))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d not found in cache", uint16(rawJsonData["remoteControllerID"].(float64)))
	}
	projectileEvent.ActualFirerID = actualFirer.ID

	defs.Logger.Trace().Msg("Processing vehicleID")
	vehicleID := rawJsonData["vehicleID"].(float64)
	vehicle, ok := EntityCache.GetVehicle(uint16(vehicleID))
	if ok {
		projectileEvent.VehicleID = sql.NullInt32{
			Int32: int32(vehicle.ID),
			Valid: true,
		}
		// its ok if the vehicle isn't found in the vehicle cache, it might be the person themselves. we'll just leave it null
	} else {
		projectileEvent.VehicleID = sql.NullInt32{
			Int32: 0,
			Valid: false,
		}
	}

	// for Positions parsing, we need to create a Linestring with XYZM dimensions
	// X, Y, Z, time (unix time nanoseconds)

	// first, we'll grab the positions and store XYZM in a sequence
	defs.Logger.Trace().Msgf("Projectile positions: %v", rawJsonData["positions"])
	positionSequence := []float64{}
	for _, v := range rawJsonData["positions"].([]interface{}) {
		posArr := v.([]interface{})

		defs.Logger.Trace().Msgf("Projectile posArr: %v", posArr)

		// process time as posArr[0]
		unixTimeNano := posArr[0].(string)
		// convert to float64
		unixTimeNanoFloat, err := strconv.ParseFloat(unixTimeNano, 64)
		if err != nil {
			json, _ := json.Marshal(posArr)
			defs.Logger.Error().Err(err).Str("json", string(json)).Msg("Error converting timestamp to float64")
			return projectileEvent, err
		}

		// process actual position xyz as posArr[2]
		pos := posArr[2].(string)
		point, _, err := defs.Coord3857FromString(pos)
		if err != nil {
			json, _ := json.Marshal(posArr)
			defs.Logger.Error().Err(err).Str("json", string(json)).Msg("Error converting position to Point")
			return projectileEvent, err
		}
		coords, _ := point.Coordinates()

		// append to sequence
		positionSequence = append(
			positionSequence,
			coords.XY.X,
			coords.XY.Y,
			coords.Z,
			unixTimeNanoFloat,
		)
	}

	// next, we'll create the linestring
	posSeq := geom.NewSequence(positionSequence, geom.DimXYZM)
	ls, err := geom.NewLineString(posSeq)
	if err != nil {
		json, _ := json.Marshal(posSeq)
		defs.Logger.Error().Err(err).Str("json", string(json)).Msg("Error creating linestring")
		return projectileEvent, err
	}

	defs.Logger.Trace().
		Interface("sequence", posSeq).
		Interface("linestring", ls).
		Interface("wkt", ls.AsText()).
		Interface("wkb", ls.AsBinary()).
		Msg("Created linestring")

	projectileEvent.Positions = ls.AsGeometry()

	defs.Logger.Trace().Msg("Processing hit events")
	// hit events

	projectileEvent.HitSoldiers = []defs.ProjectileHitsSoldier{}
	projectileEvent.HitVehicles = []defs.ProjectileHitsVehicle{}
	for _, event := range rawJsonData["hitParts"].([]interface{}) {
		eventArr := event.([]interface{})

		defs.Logger.Trace().Interface("eventArr", eventArr).Msg("Processing hit event")

		// [1] is []string containing hit components
		hitComponents := []string{}
		for _, v := range eventArr[1].([]interface{}) {
			hitComponents = append(hitComponents, v.(string))
		}

		// [2] is string with positionASL
		hitPos := eventArr[2].(string)
		hitPoint, _, err := defs.Coord3857FromString(hitPos)
		if err != nil {
			json, _ := json.Marshal(eventArr)
			defs.Logger.Error().Err(err).Str("json", string(json)).Msg("Error converting hit position to Point")
			return projectileEvent, err
		}

		// [3] is uint capture frame
		hitFrame := eventArr[3].(float64)

		// marshal hit components to json array
		hitComponentsJSON, err := json.Marshal(hitComponents)
		if err != nil {
			defs.Logger.Error().Err(err).Msg("Error marshalling hit components to json")
			return projectileEvent, err
		}

		defs.Logger.Trace().Interface("hitComponents", hitComponents).Msg("Processed hit components")

		// [0] is the hit entity ocap id
		hitEntityID := eventArr[0].(float64)
		hitEntity, ok := EntityCache.GetSoldier(uint16(hitEntityID))
		if ok {
			projectileEvent.HitSoldiers = append(
				projectileEvent.HitSoldiers,
				defs.ProjectileHitsSoldier{
					SoldierID:     hitEntity.ID,
					ComponentsHit: hitComponentsJSON,
					CaptureFrame:  uint(hitFrame),
					Position:      hitPoint,
				},
			)
		} else {
			hitEntity, ok := EntityCache.GetVehicle(uint16(hitEntityID))
			if ok {
				projectileEvent.HitVehicles = append(
					projectileEvent.HitVehicles,
					defs.ProjectileHitsVehicle{
						VehicleID:     hitEntity.ID,
						ComponentsHit: hitComponentsJSON,
						CaptureFrame:  uint(hitFrame),
						Position:      hitPoint,
					},
				)
			} else {
				defs.Logger.Warn().Msgf("Hit entity %d not found in cache", uint16(hitEntityID))
			}
		}
	}

	defs.Logger.Trace().Msg("Processing other properties")

	projectileEvent.VehicleRole = rawJsonData["vehicleRole"].(string)
	projectileEvent.Weapon = rawJsonData["weapon"].(string)
	projectileEvent.WeaponDisplay = rawJsonData["weaponDisplay"].(string)
	projectileEvent.Magazine = rawJsonData["magazine"].(string)
	projectileEvent.MagazineDisplay = rawJsonData["magazineDisplay"].(string)
	projectileEvent.Muzzle = rawJsonData["muzzle"].(string)
	projectileEvent.MuzzleDisplay = rawJsonData["muzzleDisplay"].(string)
	projectileEvent.Ammo = rawJsonData["ammo"].(string)
	projectileEvent.Mode = rawJsonData["fireMode"].(string)
	projectileEvent.InitialVelocity = rawJsonData["initialVelocity"].(string)

	return projectileEvent, nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// function to process events of different kinds
func logGeneralEvent(data []string) (thisEvent defs.GeneralEvent, err error) {

	functionName := ":EVENT:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog("processEvent", fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return thisEvent, err
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting timestamp to int: %v`, err), "ERROR")
		return thisEvent, err
	}
	thisEvent.Time = time.Unix(0, timestampInt)

	thisEvent.Mission = *CurrentMission

	// get event type
	thisEvent.CaptureFrame = uint(capframe)

	// get event type
	thisEvent.Name = data[1]

	// get event message
	thisEvent.Message = data[2]

	// get extra event data
	if len(data) > 3 {
		// unmarshal the json string
		err = json.Unmarshal([]byte(data[3]), &thisEvent.ExtraData)
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Error unmarshalling extra data: %s`, err), "ERROR")
			return thisEvent, err
		}
	}

	return thisEvent, nil
}

func logHitEvent(data []string) (hitEvent defs.HitEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	hitEvent.CaptureFrame = uint(capframe)
	hitEvent.Mission = *CurrentMission

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	hitEvent.Time = time.Unix(0, timestampInt)

	// parse data in array
	victimOcapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	victimSoldier, ok := EntityCache.GetSoldier(uint16(victimOcapID))
	if ok {
		hitEvent.VictimSoldier = victimSoldier
	} else {
		victimVehicle, ok := EntityCache.GetVehicle(uint16(victimOcapID))
		if ok {
			hitEvent.VictimVehicle = victimVehicle
		} else {
			return hitEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimOcapID)
		}
	}

	// now look for the shooter
	shooterOcapID, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting shooter ocap id to uint: %v`, err)
	}

	// try and find shooter in DB to associate
	// first, look in soldiers
	shooterSoldier, ok := EntityCache.GetSoldier(uint16(shooterOcapID))
	if ok {
		hitEvent.ShooterSoldier = shooterSoldier
	} else {
		// if not found, look in vehicles
		shooterVehicle, ok := EntityCache.GetVehicle(uint16(shooterOcapID))
		if ok {
			hitEvent.ShooterVehicle = shooterVehicle
		} else {
			return hitEvent, fmt.Errorf(`shooter ocap id not found in cache: %d`, shooterOcapID)
		}
	}

	// get event text
	hitEvent.EventText = data[3]

	// get event distance
	distance, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting distance to float: %v`, err)
	}
	hitEvent.Distance = float32(distance)

	return hitEvent, nil
}

func logKillEvent(data []string) (killEvent defs.KillEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	killEvent.Time = time.Unix(0, timestampInt)

	killEvent.CaptureFrame = uint(capframe)
	killEvent.Mission = *CurrentMission

	// parse data in array
	victimOcapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	// first, look in soldiers
	victimSoldier, ok := EntityCache.GetSoldier(uint16(victimOcapID))
	if ok {
		killEvent.VictimSoldier = victimSoldier
	} else {
		// if not found, look in vehicles
		victimVehicle, ok := EntityCache.GetVehicle(uint16(victimOcapID))
		if ok {
			killEvent.VictimVehicle = victimVehicle
		} else {
			return killEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimOcapID)
		}
	}

	// now look for the killer
	killerOcapID, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting killer ocap id to uint: %v`, err)
	}

	// try and find killer in DB to associate
	// first, look in soldiers
	killerSoldier, ok := EntityCache.GetSoldier(uint16(killerOcapID))
	if ok {
		killEvent.KillerSoldier = killerSoldier
	} else {
		// if not found, look in vehicles
		killerVehicle, ok := EntityCache.GetVehicle(uint16(killerOcapID))
		if ok {
			killEvent.KillerVehicle = killerVehicle
		} else {
			return killEvent, fmt.Errorf(`killer ocap id not found in cache: %d`, killerOcapID)
		}
	}

	// get event text
	killEvent.EventText = data[3]

	// get event distance
	distance, err := strconv.ParseFloat(data[4], 64)

	if err != nil {
		return killEvent, fmt.Errorf(`error converting distance to float: %v`, err)
	}

	killEvent.Distance = float32(distance)

	return killEvent, nil
}

func logChatEvent(data []string) (chatEvent defs.ChatEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	chatEvent.Time = time.Unix(0, timestampInt)

	chatEvent.CaptureFrame = uint(capframe)
	chatEvent.Mission = *CurrentMission

	// parse data in array
	senderOcapID, err := strconv.ParseInt(data[1], 10, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// try and find sender solder in DB to associate if not -1
	if senderOcapID > -1 {
		sendSoldier, ok := EntityCache.GetSoldier(uint16(senderOcapID))
		if ok {
			chatEvent.Soldier = sendSoldier
		} else {
			return chatEvent, fmt.Errorf(`sender ocap id not found in cache: %d`, senderOcapID)
		}
	}

	// parse the rest of array

	// channel is the 3rd element, compare against map
	// parse string to int
	channelInt, err := strconv.ParseInt(data[2], 10, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting channel to int: %v`, err)
	}
	channelName, ok := defs.ChatChannels[int(channelInt)]
	if ok {
		chatEvent.Channel = channelName
	} else {
		if channelInt > 5 && channelInt < 16 {
			chatEvent.Channel = "Custom"
		} else {
			chatEvent.Channel = "System"
		}
	}

	// next is from (formatted as the game message)
	chatEvent.FromName = data[3]

	// next is actual name
	chatEvent.SenderName = data[4]

	// next is message
	chatEvent.Message = data[5]

	// next is playerUID
	chatEvent.PlayerUID = data[6]

	return chatEvent, nil
}

// radio events
func logRadioEvent(data []string) (radioEvent defs.RadioEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	radioEvent.Time = time.Unix(0, timestampInt)

	radioEvent.CaptureFrame = uint(capframe)
	radioEvent.Mission = *CurrentMission

	// parse data in array
	senderOcapID, err := strconv.ParseInt(data[1], 10, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// try and find sender solder in DB to associate if not -1
	if senderOcapID > -1 {
		senderSoldier, ok := EntityCache.GetSoldier(uint16(senderOcapID))

		if !ok {
			return radioEvent, fmt.Errorf(`sender ocap id not found in cache: %d`, senderOcapID)
		}
		radioEvent.Soldier = senderSoldier
	}

	// parse the rest of array
	// radio
	radioEvent.Radio = data[2]
	// radio type (SW or LR)
	radioEvent.RadioType = data[3]
	// transmission type (start/end)
	radioEvent.StartEnd = data[4]
	// channel on radio (1-8) int8
	channelInt, err := strconv.ParseInt(data[5], 10, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting channel to int: %v`, err)
	}
	radioEvent.Channel = int8(channelInt)
	// is primary or additional channel
	isAddtl, err := strconv.ParseBool(data[6])
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting isAddtl to bool: %v`, err)
	}
	radioEvent.IsAdditional = isAddtl

	// frequency
	freq, err := strconv.ParseFloat(data[7], 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting freq to float: %v`, err)
	}
	radioEvent.Frequency = float32(freq)

	radioEvent.Code = data[8]

	return radioEvent, nil
}

func logFpsEvent(data []string) (fpsEvent defs.ServerFpsEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return fpsEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fpsEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}

	fpsEvent.CaptureFrame = uint(capframe)
	fpsEvent.Time = time.Unix(0, timestampInt)
	fpsEvent.Mission = *CurrentMission

	// parse data in array
	fps, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return fpsEvent, fmt.Errorf(`error converting fps to float: %v`, err)
	}
	fpsEvent.FpsAverage = float32(fps)

	fpsMin, err := strconv.ParseFloat(data[2], 64)
	if err != nil {
		return fpsEvent, fmt.Errorf(`error converting fpsMin to float: %v`, err)
	}
	fpsEvent.FpsMin = float32(fpsMin)

	return fpsEvent, nil
}

func logAce3DeathEvent(data []string) (deathEvent defs.Ace3DeathEvent, err error) {

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	deathEvent.Time = time.Unix(0, timestampInt)

	deathEvent.CaptureFrame = uint(capframe)
	deathEvent.Mission = *CurrentMission

	// parse data in array
	victimOcapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	// first, look in soldiers
	soldier, ok := EntityCache.GetSoldier(uint16(victimOcapID))
	if ok {
		deathEvent.Soldier = soldier
	} else {
		return deathEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimOcapID)
	}

	deathEvent.Reason = data[2]

	// get last damage source id [3]
	lastDamageSourceID, err := strconv.ParseInt(data[3], 10, 64)
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting last damage source id to uint: %v`, err)
	}

	if lastDamageSourceID > -1 {
		lastDamageSource, ok := EntityCache.GetSoldier(uint16(lastDamageSourceID))
		if !ok {
			return deathEvent, fmt.Errorf(`last damage source id not found in cache: %d`, lastDamageSourceID)
		}
		deathEvent.LastDamageSourceID = sql.NullInt32{Int32: int32(lastDamageSource.ID), Valid: true}
	}

	return deathEvent, nil
}

func logAce3UnconsciousEvent(data []string) (unconsciousEvent defs.Ace3UnconsciousEvent, err error) {
	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	unconsciousEvent.CaptureFrame = uint(capframe)
	unconsciousEvent.Mission = *CurrentMission

	// parse data in array
	ocapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting ocap id to uint: %v`, err)
	}

	// try and find soldier in DB to associate
	soldier, ok := EntityCache.GetSoldier(uint16(ocapID))
	if !ok {
		return unconsciousEvent, fmt.Errorf("soldier %d not found in cache", ocapID)
	}
	unconsciousEvent.Soldier = soldier

	isAwake, err := strconv.ParseBool(data[2])
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting isAwake to bool: %v`, err)
	}
	unconsciousEvent.IsAwake = isAwake

	return unconsciousEvent, nil
}

// logMarkerCreate processes :MARKER:CREATE: command
// Data format: [markerName, dir, type, text, frameNo, -1, ownerId, color, size, side, pos, shape, alpha, brush, timestamp]
func logMarkerCreate(data []string) (marker defs.Marker, err error) {
	functionName := ":MARKER:CREATE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	marker.MissionID = CurrentMission.ID

	// markerName
	marker.MarkerName = data[0]

	// direction
	dir, err := strconv.ParseFloat(data[1], 32)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing direction: %v", err), "ERROR")
		return marker, err
	}
	marker.Direction = float32(dir)

	// type
	marker.MarkerType = data[2]

	// text
	marker.Text = data[3]

	// frameNo
	frameStr := data[4]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return marker, err
	}
	marker.CaptureFrame = uint(capframe)

	// data[5] is -1, skip

	// ownerId
	ownerId, err := strconv.Atoi(data[6])
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing ownerId: %v", err), "WARN")
		ownerId = -1
	}
	marker.OwnerID = ownerId

	// color
	marker.Color = data[7]

	// size (stored as string "[w,h]")
	marker.Size = data[8]

	// side
	marker.Side = data[9]

	// position - parse from arma string "[x,y,z]"
	pos := data[10]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, _, err := defs.Coord3857FromString(pos)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing position: %v", err), "ERROR")
		return marker, err
	}
	marker.Position = point

	// shape
	marker.Shape = data[11]

	// alpha
	alpha, err := strconv.ParseFloat(data[12], 32)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing alpha: %v", err), "WARN")
		alpha = 1.0
	}
	marker.Alpha = float32(alpha)

	// brush
	marker.Brush = data[13]

	// timestamp
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing timestamp: %v", err), "ERROR")
		return marker, err
	}
	marker.Time = time.Unix(0, timestampInt)

	marker.IsDeleted = false

	return marker, nil
}

// logMarkerMove processes :MARKER:MOVE: command
// Data format: [markerName, frameNo, pos, dir, alpha, timestamp]
func logMarkerMove(data []string) (markerState defs.MarkerState, err error) {
	functionName := ":MARKER:MOVE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	markerState.MissionID = CurrentMission.ID

	// markerName - look up marker ID from cache
	markerName := data[0]
	MarkerCacheLock.RLock()
	markerID, ok := MarkerCache[markerName]
	MarkerCacheLock.RUnlock()
	if !ok {
		return markerState, fmt.Errorf("marker %s not found in cache", markerName)
	}
	markerState.MarkerID = markerID

	// frameNo
	frameStr := data[1]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return markerState, err
	}
	markerState.CaptureFrame = uint(capframe)

	// position
	pos := data[2]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, _, err := defs.Coord3857FromString(pos)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing position: %v", err), "ERROR")
		return markerState, err
	}
	markerState.Position = point

	// direction
	dir, err := strconv.ParseFloat(data[3], 32)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing direction: %v", err), "WARN")
		dir = 0
	}
	markerState.Direction = float32(dir)

	// alpha
	alpha, err := strconv.ParseFloat(data[4], 32)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing alpha: %v", err), "WARN")
		alpha = 1.0
	}
	markerState.Alpha = float32(alpha)

	// timestamp
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing timestamp: %v", err), "ERROR")
		return markerState, err
	}
	markerState.Time = time.Unix(0, timestampInt)

	return markerState, nil
}

// logMarkerDelete processes :MARKER:DELETE: command
// Data format: [markerName, frameNo, timestamp]
func logMarkerDelete(data []string) (markerName string, frameNo uint, err error) {
	functionName := ":MARKER:DELETE:"

	// fix received data
	for i, v := range data {
		data[i] = fixEscapeQuotes(trimQuotes(v))
	}

	markerName = data[0]

	// frameNo
	frameStr := data[1]
	capframe, err := strconv.ParseInt(frameStr, 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return markerName, 0, err
	}
	frameNo = uint(capframe)

	return markerName, frameNo, nil
}

///////////////////////
// GOROUTINES //
///////////////////////

func startAsyncProcessors() {
	functionName := ":DATA:PROCESSOR:"

	// these goroutines will receive data from buffered channels & process them into defined models, then push them into queues for writing
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":NEW:SOLDIER:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logNewSoldier(v)
			if err == nil {
				soldiersToWrite.Push([]defs.Soldier{obj})
			} else {
				writeLog(functionName, fmt.Sprintf(`Failed to log new soldier. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":NEW:VEHICLE:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logNewVehicle(v)
			if err == nil {
				vehiclesToWrite.Push([]defs.Vehicle{obj})
			} else {

				writeLog(functionName, fmt.Sprintf(`Failed to log new vehicle. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":NEW:SOLDIER:STATE:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logSoldierState(v)
			if err == nil {
				soldierStatesToWrite.Push([]defs.SoldierState{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log soldier state. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":NEW:VEHICLE:STATE:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logVehicleState(v)
			if err == nil {
				vehicleStatesToWrite.Push([]defs.VehicleState{obj})
				continue
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log vehicle state. Err: %s`, err), "ERROR")
				continue
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":FIRED:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logFiredEvent(v)
			if err == nil {
				firedEventsToWrite.Push([]defs.FiredEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log fired event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// projectile events
	go func() {
		for v := range RVExtArgsDataChannels[":PROJECTILE:"] {
			// new projectile events use linestringzm geo format, which is not supported by SQLite
			if !IsDatabaseValid || ShouldSaveLocal {
				continue
			}

			obj, err := logProjectileEvent(v)
			if err == nil {
				projectileEventsToWrite.Push([]defs.ProjectileEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log projectile event. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":EVENT:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logGeneralEvent(v)
			if err == nil {
				generalEventsToWrite.Push([]defs.GeneralEvent{obj})
			} else {
				writeLog(functionName, fmt.Sprintf(`Failed to log fired event. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":HIT:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logHitEvent(v)
			if err == nil {
				hitEventsToWrite.Push([]defs.HitEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log hit event. Err: %s`, err), "ERROR")
			}
		}
	}()

	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":KILL:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logKillEvent(v)
			if err == nil {
				killEventsToWrite.Push([]defs.KillEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log kill event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// chat events
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":CHAT:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logChatEvent(v)
			if err == nil {
				chatEventsToWrite.Push([]defs.ChatEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log chat event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// radio events
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":RADIO:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logRadioEvent(v)
			if err == nil {
				radioEventsToWrite.Push([]defs.RadioEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log radio event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// fps events
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":FPS:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logFpsEvent(v)
			if err == nil {
				fpsEventsToWrite.Push([]defs.ServerFpsEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log fps event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// ace3 death events
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":ACE3:DEATH:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logAce3DeathEvent(v)
			if err == nil {
				ace3DeathEventsToWrite.Push([]defs.Ace3DeathEvent{obj})
			} else {
				// if its within the first 10 frames, we don't want to log the error (because it's likely the unit itself isn't inserted yet)
				if err == errTooEarlyForStateAssociation {
					continue
				}
				writeLog(functionName, fmt.Sprintf(`Failed to log ace3 death event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// ace3 unconscious events
	go func() {
		// process channel data
		for v := range RVExtArgsDataChannels[":ACE3:UNCONSCIOUS:"] {
			if !IsDatabaseValid {
				continue
			}

			obj, err := logAce3UnconsciousEvent(v)
			if err == nil {
				ace3UnconsciousEventsToWrite.Push([]defs.Ace3UnconsciousEvent{obj})
			} else {
				writeLog(functionName, fmt.Sprintf(`Failed to log ace3 unconscious event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// metric data
	go func() {
		var err error
		for data := range RVExtArgsDataChannels[":METRIC:"] {

			if InfluxClient == nil && InfluxBackupWriter == nil {
				continue
			}

			// process data
			var bucket string
			var point *influxdb2_write.Point
			bucket, point, err = processMetricData(data)
			if err != nil {
				defs.Logger.Error().Err(err).Msg("error processing metric data")
				continue
			}

			err = writeInfluxPoint(context.Background(), bucket, point)
			if err != nil {
				defs.Logger.Error().Err(err).Msg("error writing influx point")
				continue
			}
		}
	}()

	// :MARKER:CREATE: processor
	go func() {
		for data := range RVExtArgsDataChannels[":MARKER:CREATE:"] {
			if !IsDatabaseValid {
				continue
			}

			marker, err := logMarkerCreate(data)
			if err != nil {
				defs.Logger.Error().Err(err).Msg("Error processing marker create")
				continue
			}
			markersToWrite.Push([]defs.Marker{marker})
		}
	}()

	// :MARKER:MOVE: processor
	go func() {
		for data := range RVExtArgsDataChannels[":MARKER:MOVE:"] {
			if !IsDatabaseValid {
				continue
			}

			markerState, err := logMarkerMove(data)
			if err != nil {
				defs.Logger.Warn().Err(err).Msg("Error processing marker move")
				continue
			}
			markerStatesToWrite.Push([]defs.MarkerState{markerState})
		}
	}()

	// :MARKER:DELETE: processor
	go func() {
		for data := range RVExtArgsDataChannels[":MARKER:DELETE:"] {
			if !IsDatabaseValid {
				continue
			}

			markerName, frameNo, err := logMarkerDelete(data)
			if err != nil {
				defs.Logger.Error().Err(err).Msg("Error processing marker delete")
				continue
			}

			// Look up marker and mark as deleted
			MarkerCacheLock.RLock()
			markerID, ok := MarkerCache[markerName]
			MarkerCacheLock.RUnlock()
			if ok {
				// Create a marker state marking deletion
				deleteState := defs.MarkerState{
					MissionID:    CurrentMission.ID,
					MarkerID:     markerID,
					CaptureFrame: frameNo,
					Time:         time.Now(),
					Alpha:        0, // Zero alpha indicates deletion
				}
				markerStatesToWrite.Push([]defs.MarkerState{deleteState})

				// Update marker as deleted in DB
				DB.Model(&defs.Marker{}).Where("id = ?", markerID).Update("is_deleted", true)
			}
		}
	}()
}

var errTooEarlyForStateAssociation error = fmt.Errorf(`too early for state association`)

func startDBWriters() {
	functionName := ":DB:WRITER:"
	// start the DB Write goroutine
	go func() {
		for {
			if !IsDatabaseValid {
				time.Sleep(1 * time.Second)
				continue
			}

			if DBInsertsPaused {
				time.Sleep(1 * time.Second)
				continue
			}

			var (
				writeStart time.Time = time.Now()
			)

			// write new soldiers
			if !soldiersToWrite.Empty() {
				tx := DB.Begin()
				toWrite := soldiersToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldiers: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()

					// update entity cache
					for _, v := range toWrite {
						EntityCache.AddSoldier(v)
					}
				}
			}

			// write soldier states
			if !soldierStatesToWrite.Empty() {
				tx := DB.Begin()
				toWrite := soldierStatesToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldier states: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write new vehicles
			if !vehiclesToWrite.Empty() {
				tx := DB.Begin()
				toWrite := vehiclesToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicles: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()

					// update entity cache
					for _, v := range toWrite {
						EntityCache.AddVehicle(v)
					}
				}
			}

			// write vehicle states
			if !vehicleStatesToWrite.Empty() {
				tx := DB.Begin()
				toWrite := vehicleStatesToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicle states: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					writeLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write fired events
			if !firedEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := firedEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating fired events: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					writeLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write projectile events
			if !projectileEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := projectileEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating projectile events: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					writeLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write general events
			if !generalEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := generalEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating general events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write hit events
			if !hitEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := hitEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating hit events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write kill events
			if !killEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := killEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating killed events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write chat events
			if !chatEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := chatEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating chat events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write radio events
			if !radioEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := radioEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating radio events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write serverfps events
			if !fpsEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := fpsEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating serverfps events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write ace3 death events
			if !ace3DeathEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := ace3DeathEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating ace3 death events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write ace3 unconscious events
			if !ace3UnconsciousEventsToWrite.Empty() {
				tx := DB.Begin()
				toWrite := ace3UnconsciousEventsToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating ace3 unconscious events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write markers
			if !markersToWrite.Empty() {
				tx := DB.Begin()
				toWrite := markersToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating markers: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()

					// update marker cache with new IDs
					MarkerCacheLock.Lock()
					for _, m := range toWrite {
						if m.ID != 0 {
							MarkerCache[m.MarkerName] = m.ID
						}
					}
					MarkerCacheLock.Unlock()
				}
			}

			// write marker states
			if !markerStatesToWrite.Empty() {
				tx := DB.Begin()
				toWrite := markerStatesToWrite.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating marker states: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			LastDBWriteDuration = time.Since(writeStart)

			// sleep
			time.Sleep(2 * time.Second)

		}
	}()
}

// /////////////////////
// EXPORTED FUNCTIONS //
// /////////////////////

func trimQuotes(s string) string {
	// trim the start and end quotes from a string
	return strings.Trim(s, `"`)
}

func fixEscapeQuotes(s string) string {
	// fix the escape quotes in a string
	return strings.Replace(s, `""`, `"`, -1)
}

func writeLog(
	functionName string,
	data string,
	level string,
) {
	// get calling function & line
	// _, file, line, _ := runtime.Caller(1)

	var logLevelActual zerolog.Level
	switch level {
	case "DEBUG":
		logLevelActual = zerolog.DebugLevel
	case "INFO":
		logLevelActual = zerolog.InfoLevel
	case "WARN":
		logLevelActual = zerolog.WarnLevel
	case "ERROR":
		logLevelActual = zerolog.ErrorLevel
	case "FATAL":
		logLevelActual = zerolog.FatalLevel
	}

	// debug limits configured in global defs based on config
	defs.Logger.WithLevel(logLevelActual).
		Str("function", functionName).
		Msg(data)

	defs.JSONLogger.WithLevel(logLevelActual).
		Str("function", functionName).
		Msg(data)
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
		// ocap_infos
		err = migrateTable(sqliteDB, tx, defs.OcapInfo{}, "ocap_infos")
		if err != nil {
			return fmt.Errorf("error migrating ocapinfo: %v", err)
		}
		// after_action_reviews
		err = migrateTable(sqliteDB, tx, defs.AfterActionReview{}, "after_action_reviews")
		if err != nil {
			return fmt.Errorf("error migrating after_action_reviews: %v", err)
		}
		// worlds
		err = migrateTable(sqliteDB, tx, defs.World{}, "worlds")
		if err != nil {
			return fmt.Errorf("error migrating worlds: %v", err)
		}
		// missions
		err = migrateTable(sqliteDB, tx, defs.Mission{}, "missions")
		if err != nil {
			return fmt.Errorf("error migrating missions: %v", err)
		}

		// soldiers
		err = migrateTable(sqliteDB, tx, defs.Soldier{}, "soldiers")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating soldiers: %v", err)
		}
		//soldier_states
		err = migrateTable(sqliteDB, tx, defs.SoldierState{}, "soldier_states")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating soldier_states: %v", err)
		}
		// vehicles
		err = migrateTable(sqliteDB, tx, defs.Vehicle{}, "vehicles")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating vehicles: %v", err)
		}
		// vehicle_states
		err = migrateTable(sqliteDB, tx, defs.VehicleState{}, "vehicle_states")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating vehicle_states: %v", err)
		}
		// fired_events
		err = migrateTable(sqliteDB, tx, defs.FiredEvent{}, "fired_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating fired_events: %v", err)
		}
		// projectile_events
		err = migrateTable(sqliteDB, tx, defs.ProjectileEvent{}, "projectile_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating projectile_events: %v", err)
		}
		// general_events
		err = migrateTable(sqliteDB, tx, defs.GeneralEvent{}, "general_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating general_events: %v", err)
		}
		// hit_events
		err = migrateTable(sqliteDB, tx, defs.HitEvent{}, "hit_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating hit_events: %v", err)
		}
		// kill_events
		err = migrateTable(sqliteDB, tx, defs.KillEvent{}, "kill_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating kill_events: %v", err)
		}
		// chat_events
		err = migrateTable(sqliteDB, tx, defs.ChatEvent{}, "chat_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating chat_events: %v", err)
		}
		// radio_events
		err = migrateTable(sqliteDB, tx, defs.RadioEvent{}, "radio_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating radio_events: %v", err)
		}
		// server_fps_events
		err = migrateTable(sqliteDB, tx, defs.ServerFpsEvent{}, "server_fps_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating server_fps_events: %v", err)
		}
		// ace3_death_events
		err = migrateTable(sqliteDB, tx, defs.Ace3DeathEvent{}, "ace3_death_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating ace3_death_events: %v", err)
		}
		// ace3_unconscious_events
		err = migrateTable(sqliteDB, tx, defs.Ace3UnconsciousEvent{}, "ace3_unconscious_events")
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error migrating ace3_unconscious_events: %v", err)
		}
		// ocap_performances
		err = migrateTable(sqliteDB, tx, defs.OcapPerformance{}, "ocap_performances")
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
			defs.Logger.Error().Msgf("Error getting sqlite connection: %v", err)
			defs.JSONLogger.Error().Msgf("Error getting sqlite connection: %v", err)
			continue
		}
		err = sqlConnection.Close()
		if err != nil {
			defs.Logger.Error().Msgf("Error closing sqlite connection: %v", err)
		}
		err = os.Rename(sqlitePath, sqlitePath+".migrated")
		if err != nil {
			defs.Logger.Error().Msgf("Error renaming sqlite file: %v", err)
		}
		successfulMigrations = append(successfulMigrations, sqlitePath)
	}

	// if we get here, we've successfully migrated all backups
	successArr := zerolog.Arr()
	for _, successfulMigration := range successfulMigrations {
		successArr.Str(successfulMigration)
	}
	defs.Logger.Info().Array(
		"successfulMigrations",
		successArr,
	).Msgf("Successfully migrated %d backups, it's recommended to delete these to avoid future data duplication", len(successfulMigrations))

	return nil
}

// helper function for sqlite migrations
func migrateTable[M any](
	sqliteDB *gorm.DB,
	postgresDB *gorm.DB,
	model M,
	tableName string,
) (err error) {
	var data = &map[string]interface{}{}
	sqliteDB.Model(&model).
		Assign("id", gorm.Expr("NULL")). // remove the id field from the data
		Find(data)
	defs.Logger.Info().Msgf("Found %d %s", len(*data), tableName)
	defs.JSONLogger.Info().Msgf("Found %d %s", len(*data), tableName)

	if len(*data) == 0 {
		return nil
	}

	defs.Logger.Info().Msgf("Inserting %d %s", len(*data), tableName)

	// insert into postgres
	postgresDB.Model(&model).Clauses(
		clause.OnConflict{
			DoNothing: true,
		}).Create(data)
	if postgresDB.Error != nil {
		defs.Logger.Error().Err(err).
			Str("database", sqliteDB.Name()).
			Msgf("Error migrating %s", tableName)
		defs.JSONLogger.Error().Err(err).
			Str("database", sqliteDB.Name()).
			Msgf("Error migrating %s", tableName)
		return err
	}

	return nil
}

func populateDemoData() {
	if !IsDatabaseValid {
		return
	}

	// declare test size counts
	var (
		numMissions              int = 2
		missionDuration          int = 60 * 15                                                    // s * value (m) = total (s)
		numUnitsPerMission       int = 60                                                         // num units per mission
		numSoldiers              int = int(math.Ceil(float64(numUnitsPerMission) * float64(0.8))) // numUnits / 3
		numFiredEventsPerSoldier int = 2700
		numVehicles              int = numUnitsPerMission - numSoldiers // numUnits / 3

		waitGroup = sync.WaitGroup{}

		sides []string = []string{
			"WEST",
			"EAST",
			"GUER",
			"CIV",
		}

		vehicleClassnames []string = []string{
			"B_MRAP_01_F",
			"B_MRAP_01_gmg_F",
			"B_MRAP_01_hmg_F",
			"B_G_Offroad_01_armed_F",
			"B_G_Offroad_01_AT_F",
			"B_G_Offroad_01_F",
			"B_G_Offroad_01_repair_F",
			"B_APC_Wheeled_01_cannon_F",
			"B_APC_Tracked_01_AA_F",
			"B_APC_Tracked_01_CRV_F",
			"B_APC_Tracked_01_rcws_F",
			"B_APC_Tracked_01_CRV_F",
		}

		ocapTypes []string = []string{
			"ship",
			"parachute",
			"heli",
			"plane",
			"truck",
			"car",
			"apc",
			"tank",
			"staticMortar",
			"unknown",
		}

		roles []string = []string{
			"Rifleman",
			"Team Leader",
			"Auto Rifleman",
			"Assistant Auto Rifleman",
			"Grenadier",
			"Machine Gunner",
			"Assistant Machine Gunner",
			"Medic",
			"Engineer",
			"Explosive Specialist",
			"Rifleman (AT)",
			"Rifleman (AA)",
			"Officer",
		}

		roleDescriptions []string = []string{
			"Rifleman@Alpha",
			"Team Leader@Alpha",
			"Auto Rifleman@Alpha",
			"Assistant Auto Rifleman@Alpha",
			"Grenadier@Alpha",
			"Machine Gunner@Alpha",
			"Assistant Machine Gunner@Alpha",
			"Medic@Alpha",
			"Rifleman@Bravo",
			"Team Leader@Bravo",
			"Auto Rifleman@Bravo",
			"Assistant Auto Rifleman@Bravo",
			"Grenadier@Bravo",
			"Machine Gunner@Bravo",
			"Assistant Machine Gunner@Bravo",
			"Medic@Bravo",
		}

		weapons []string = []string{
			"Katiba",
			"MXC 6.5 mm",
			"MX 6.5 mm",
			"MX SW 6.5 mm",
			"MXM 6.5 mm",
			"SPAR-16 5.56 mm",
			"SPAR-16S 5.56 mm",
			"SPAR-17 7.62 mm",
			"TAR-21 5.56 mm",
			"TRG-21 5.56 mm",
			"TRG-20 5.56 mm",
			"TRG-21 EGLM 5.56 mm",
			"CAR-95 5.8 mm",
			"CAR-95-1 5.8 mm",
			"CAR-95 GL 5.8 mm",
		}

		magazines []string = []string{
			"30rnd 6.5 mm Caseless Mag",
			"30rnd 6.5 mm Caseless Mag Tracer",
			"100rnd 6.5 mm Caseless Mag",
			"100rnd 6.5 mm Caseless Mag Tracer",
			"200rnd 6.5 mm Caseless Mag",
			"200rnd 6.5 mm Caseless Mag Tracer",
			"30rnd 5.56 mm STANAG",
			"30rnd 5.56 mm STANAG Tracer (Yellow)",
			"30rnd 5.56 mm STANAG Tracer (Red)",
			"30rnd 5.56 mm STANAG Tracer (Green)",
		}

		firemodes []string = []string{
			"Single",
			"FullAuto",
			"Burst3",
			"Burst5",
		}

		worldNames []string = []string{
			"Altis",
			"Stratis",
			"VR",
			"Bootcamp_ACR",
			"Malden",
			"ProvingGrounds_PMC",
			"Shapur_BAF",
			"Sara",
			"Sara_dbe1",
			"SaraLite",
			"Woodland_ACR",
			"Chernarus",
			"Desert_E",
			"Desert_Island",
			"Intro",
			"Desert2",
		}

		demoAddons [][]interface{} = [][]interface{}{
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

		// WORLD CONTEXT SENT AS JSON IN DATA[0]
		// ["author", _author],
		// ["workshopID", _workshopID],
		// ["displayName", _name],
		// ["worldName", toLower worldName],
		// ["worldNameOriginal", worldName],
		// ["worldSize", getNumber(configFile >> "CfgWorlds" >> worldName >> "worldSize")],
		// ["latitude", getNumber(configFile >> "CfgWorlds" >> worldName >> "latitude")],
		// ["longitude", getNumber(configFile >> "CfgWorlds" >> worldName >> "longitude")]

		// pick random name from list
		worldNameOriginal := worldNames[rand.Intn(len(worldNames))]
		// displayname, replace underscores with spaces
		displayName := strings.Replace(worldNameOriginal, "_", " ", -1)
		// worldName, lowercase
		worldName := strings.ToLower(worldNameOriginal)

		worldData := map[string]interface{}{
			"author":            "Demo Author",
			"workshopID":        "123456789",
			"displayName":       displayName,
			"worldName":         worldName,
			"worldNameOriginal": worldNameOriginal,
			"worldSize":         10240,
			// random lon lat
			"latitude":  rand.Float64() * 180,
			"longitude": rand.Float64() * 180,
		}
		worldDataJSON, err := json.Marshal(worldData)
		if err != nil {
			fmt.Println(err)
		}
		data[0] = string(worldDataJSON)

		// MISSION CONTEXT SENT AS STRING IN DATA[1]

		// ["tag", EGVAR(settings,saveTag)]
		missionData := map[string]interface{}{
			"missionName":                  fmt.Sprintf("Demo Mission %d", i),
			"briefingName":                 fmt.Sprintf("Demo Briefing %d", i),
			"missionNameSource":            fmt.Sprintf("Demo Mission %d", i),
			"onLoadName":                   "TestLoadName",
			"author":                       "Demo Author",
			"serverName":                   "Demo Server",
			"serverProfile":                "Demo Profile",
			"missionStart":                 nil, // random time
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

		RVExtArgsDataChannels[":NEW:MISSION:"] <- data

		time.Sleep(500 * time.Millisecond)
	}
	missionsEnd := time.Now()
	missionsElapsed := missionsEnd.Sub(missionsStart)
	fmt.Printf("Sent %d missions in %s\n", numMissions, missionsElapsed)

	// for each mission
	missions := []defs.Mission{}
	DB.Model(&defs.Mission{}).
		Order("created_at DESC").
		Limit(numMissions).
		Find(&missions)

	waitGroup = sync.WaitGroup{}
	for _, mission := range missions {
		CurrentMission = &mission

		fmt.Printf("Populating mission with ID %d\n", mission.ID)

		// write soldiers, now that our missions exist and the channels have been created
		idCounter := 1

		for i := 0; i <= numSoldiers; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				// these will be sent as an array
				// frame := strconv.FormatInt(int64(rand.Intn(missionDuration)), 10)
				soldierID := strconv.FormatInt(int64(thisId), 10)
				squadParams := []interface{}{
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
					soldierID,                                          // join frame
					soldierID,                                          // ocapid
					fmt.Sprintf("Demo Unit %d", i),                     // unit name
					fmt.Sprintf("Demo Group %d", i),                    // group id
					sides[rand.Intn(len(sides))],                       // side
					strconv.FormatBool(rand.Intn(2) == 1),              // isplayer
					roleDescriptions[rand.Intn(len(roleDescriptions))], // roleDescription
					"B_Soldier_F",                                      // unit classname
					"Rifleman",                                         // unit display name
					strconv.FormatInt(int64(rand.Intn(1000000000)), 10), // random player uid
					// json squad params
					string(squadParamsJSON),
				}

				// add timestamp to end of array
				soldier = append(soldier, fmt.Sprintf("%d", time.Now().UnixNano()))
				RVExtArgsDataChannels[":NEW:SOLDIER:"] <- soldier

				//wait loop to check if in cache yet
				for {
					time.Sleep(100 * time.Millisecond)
					if _, ok := EntityCache.GetSoldier(uint16(thisId)); ok {
						break
					}
				}

				// write soldier states
				var randomPos [3]float64 = [3]float64{rand.Float64() * 30720, rand.Float64() * 30720, rand.Float64() * 30720}
				var randomDir float64 = rand.Float64() * 360
				var dirMoveOffset float64 = rand.Float64() * 360
				var randomLifestate int = rand.Intn(3)
				var currentRole string = roles[rand.Intn(len(roles))]
				for i := 0; i <= missionDuration; i++ {
					// sleep 500 ms to make sure there's enough time between processing so time doesn't match exactly
					time.Sleep(100 * time.Millisecond)

					stateFrame := i
					// determine xy transform to translate pos in randomDir limited to 180 degress of dirMoveOffset
					var xyTransform [2]float64 = [2]float64{0, 0}
					if randomDir < 180 {
						xyTransform[0] = math.Sin((randomDir + dirMoveOffset) * (math.Pi / 180))
						xyTransform[1] = math.Cos((randomDir + dirMoveOffset) * (math.Pi / 180))
					} else {
						xyTransform[0] = math.Sin((randomDir - dirMoveOffset) * (math.Pi / 180))
						xyTransform[1] = math.Cos((randomDir - dirMoveOffset) * (math.Pi / 180))
					}
					// adjust random pos by random + or - 4m in xyTransform direction
					randomPos[0] += xyTransform[0] + (rand.Float64() * 8) - 4
					randomPos[1] += xyTransform[1] + (rand.Float64() * 8) - 4
					randomPos[2] += (rand.Float64() * 8) - 4

					// adjust random dir by random + or - 4
					randomDir += (rand.Float64() * 8) - 4

					// 10% chance of setting random lifestate
					if rand.Intn(10) == 0 {
						randomLifestate = rand.Intn(3)
					}

					// 5% chance of setting random role
					if rand.Intn(20) == 0 {
						currentRole = roles[rand.Intn(len(roles))]
					}

					soldierState := []string{
						// soldier id we just made
						soldierID,
						// random pos
						fmt.Sprintf("[%f,%f,%f]", randomPos[0], randomPos[1], randomPos[2]),
						// random dir rounded to int
						strconv.FormatFloat(randomDir, 'f', 0, 64),
						// random lifestate (0 to 2)
						strconv.FormatInt(int64(randomLifestate), 10),
						// random inVehicle bool
						strconv.FormatBool(rand.Intn(2) == 1),
						// random name
						fmt.Sprintf("Demo Unit %d", i),
						// random isPlayer bool
						strconv.FormatBool(rand.Intn(2) == 1),
						// random role
						currentRole,
						// capture frame
						strconv.FormatInt(int64(stateFrame), 10),
						// hasStableVitals 0 or 1
						strconv.FormatInt(int64(rand.Intn(2)), 10),
						// is being dragged/carried 0 or 1
						strconv.FormatInt(int64(rand.Intn(2)), 10),
						// scores, random uint array of length 6
						fmt.Sprintf("%d,%d,%d,%d,%d,%d", rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20), rand.Intn(20)),
						// vehicle role, select random
						"Passenger",
						// random stance
						[]string{"Up", "Middle", "Down"}[rand.Intn(3)],
					}

					// add timestamp to end of array
					soldierState = append(soldierState, fmt.Sprintf("%d", time.Now().UnixNano()))
					RVExtArgsDataChannels[":NEW:SOLDIER:STATE:"] <- soldierState
				}
				waitGroup.Done()
			}(idCounter)
			idCounter++
		}

		EntityCache.Lock()
		defs.Logger.Debug().Int("numSoldiersCached", len(EntityCache.Soldiers)).Msg("Soldiers cached")
		EntityCache.Unlock()

		// write vehicles
		for i := 0; i <= numVehicles; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				vehicleID := strconv.FormatInt(int64(thisId), 10)
				vehicle := []string{
					// join frame
					vehicleID,
					// ocapid
					vehicleID,
					// random ocap type
					ocapTypes[rand.Intn(len(ocapTypes))],
					// random display name
					fmt.Sprintf("Demo Vehicle %d", i),
					// random classname
					vehicleClassnames[rand.Intn(len(vehicleClassnames))],
					// random vehicle customization
					fmt.Sprintf(
						`"[[""%s"", %d], [""%s"", %d], [""%s"", %d]]"`,
						"wasp",
						1,
						"AddTread",
						1,
						"AddTread_Short",
						1,
					),
				}

				// add timestamp to end of array
				vehicle = append(vehicle, fmt.Sprintf("%d", time.Now().UnixNano()))
				RVExtArgsDataChannels[":NEW:VEHICLE:"] <- vehicle

				// use loop, waiting for vehicle to be written (available in cache)
				for {
					// sleep 1000 ms
					time.Sleep(1000 * time.Millisecond)
					// check if vehicle is in cache
					if _, ok := EntityCache.GetVehicle(uint16(thisId)); ok {
						break
					}
				}

				// send vehicle states
				for i := 0; i <= missionDuration; i++ {
					// sleep for timestamp
					time.Sleep(100 * time.Millisecond)
					vehicleState := []string{
						// ocap id
						vehicleID,
						// random pos
						fmt.Sprintf("[%f,%f,%f]", rand.Float64()*30720+1, rand.Float64()*30720+1, rand.Float64()*30720+1),
						// random dir
						fmt.Sprintf("%d", rand.Intn(360)),
						// random isAlive bool
						strconv.FormatBool(rand.Intn(2) == 1),
						// random crew (array of soldiers)
						fmt.Sprintf("[%d,%d,%d]", rand.Intn(numSoldiers)+1, rand.Intn(numSoldiers)+1, rand.Intn(numSoldiers)+1),
						// random frame
						strconv.FormatInt(int64(rand.Intn(missionDuration)), 10),
						// random fuel 0 to 1.0
						fmt.Sprintf("%f", rand.Float64()),
						// random damage 0 to 1.0
						fmt.Sprintf("%f", rand.Float64()),
						// random isEngineOn bool (1 in 10 chance of being true)
						strconv.FormatBool(rand.Intn(10) == 0),
						// random locked bool (1 in 10 chance of being true)
						strconv.FormatBool(rand.Intn(10) == 0),
						// random side
						sides[rand.Intn(len(sides))],
						// vectordir
						fmt.Sprintf("[%f,%f,%f]", rand.Float64(), rand.Float64(), rand.Float64()),
						// vectorup
						fmt.Sprintf("[%f,%f,%f]", rand.Float64(), rand.Float64(), rand.Float64()),
					}

					// add timestamp to end of array
					vehicleState = append(vehicleState, fmt.Sprintf("%d", time.Now().UnixNano()))
					RVExtArgsDataChannels[":NEW:VEHICLE:STATE:"] <- vehicleState
				}
				waitGroup.Done()
			}(idCounter)
			idCounter++
		}

		// wait for all units and states to be created so fired events can associate
		waitGroup.Wait()

		fmt.Println("Finished creating units and states.")
		fmt.Println("Creating fired events...")

		// create a new wait group
		wg2 := sync.WaitGroup{}
		// demo fired events, X per soldier per mission
		for i := 0; i <= numSoldiers*numFiredEventsPerSoldier; i++ {
			wg2.Add(1)

			// sleep 100 ms
			// time.Sleep(100 * time.Millisecond)

			go func() {
				var randomStartPos []float64 = []float64{rand.Float64()*30720 + 1, rand.Float64()*30720 + 1, rand.Float64()*30720 + 1}
				// generate random end pos within 200 m
				var randomEndPos []float64
				for j := 0; j < 3; j++ {
					randomEndPos = append(randomEndPos, randomStartPos[j]+rand.Float64()*400-200)
				}

				firedEvent := []string{
					// random soldier id
					strconv.FormatInt(int64(rand.Intn(numSoldiers)+1), 10),
					// random frame
					strconv.FormatInt(int64(rand.Intn(missionDuration)), 10),
					// random start pos
					fmt.Sprintf("%f,%f,%f", randomStartPos[0], randomStartPos[1], randomStartPos[2]),
					// random end pos within 200m of start pos
					fmt.Sprintf("%f,%f,%f", randomEndPos[0], randomEndPos[1], randomEndPos[2]),
					// random weapon
					weapons[rand.Intn(len(weapons))],
					// random magazine
					magazines[rand.Intn(len(magazines))],
					// random firemode
					firemodes[rand.Intn(len(firemodes))],
				}

				// add timestamp to end of array
				firedEvent = append(firedEvent, fmt.Sprintf("%d", time.Now().UnixNano()))
				RVExtArgsDataChannels[":FIRED:"] <- firedEvent
				wg2.Done()
			}()
		}
		wg2.Wait()
	}

	// pause until RVExtArgsDataChannels[":FIRED:"] is empty
	for len(RVExtArgsDataChannels[":FIRED:"]) > 0 {
		time.Sleep(1000 * time.Millisecond)
	}

}

func getOcapRecording(missionIDs []string) (err error) {
	fmt.Println("Getting JSON for mission IDs: ", missionIDs)

	queries := []string{}

	for _, missionID := range missionIDs {

		// get missionIdInt
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		// var result string
		var txStart time.Time

		// get mission data
		txStart = time.Now()
		var mission defs.Mission
		ocapMission := make(map[string]interface{})
		err = DB.Model(&defs.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return err
		}

		// preprocess mission data into an object
		ocapMission["addonVersion"] = mission.AddonVersion
		ocapMission["extensionVersion"] = mission.ExtensionVersion
		ocapMission["extensionBuild"] = mission.ExtensionBuild
		ocapMission["ocapRecorderExtensionVersion"] = mission.OcapRecorderExtensionVersion

		ocapMission["missionAuthor"] = mission.Author
		ocapMission["missionName"] = mission.OnLoadName
		if mission.OnLoadName == "" {
			ocapMission["missionName"] = mission.BriefingName
		}
		ocapMission["tags"] = mission.Tag
		ocapMission["captureDelay"] = mission.CaptureDelay

		ocapMission["Markers"] = make([]interface{}, 0)
		ocapMission["entities"] = make([]interface{}, 0)
		ocapMission["events"] = make([]interface{}, 0)
		ocapMission["times"] = make([]interface{}, 0)

		// get world name by checking relation
		var world defs.World
		err = DB.Model(&defs.World{}).Where("id = ?", mission.WorldID).Select([]string{"world_name", "display_name"}).Scan(&world).Error
		if err != nil {
			return err
		}
		ocapMission["worldName"] = world.WorldName
		ocapMission["worldDisplayName"] = world.DisplayName

		// check for an endmission event in the general events table
		var endMissionEventFrame uint
		err = DB.Model(&defs.GeneralEvent{}).Where("name = ?", "endMission").Order("capture_frame DESC").Limit(1).Pluck("capture_frame", &endMissionEventFrame).Error
		if err != nil {
			return err
		}
		// if not found,
		// get last soldier state
		if endMissionEventFrame == 0 {
			var endSoldierStatesFrame uint
			err = DB.Model(&defs.SoldierState{}).Where("mission_id = ?", mission.ID).Order("capture_frame DESC").Limit(1).Pluck("capture_frame", &endSoldierStatesFrame).Error
			if err != nil {
				return err
			}

			// get last vehicle state
			var endVehicleStatesFrame uint
			err = DB.Model(&defs.VehicleState{}).Where("mission_id = ?", mission.ID).Order("capture_frame DESC").Limit(1).Pluck("capture_frame", &endVehicleStatesFrame).Error
			if err != nil {
				return err
			}

			// take the higher of the two
			if endSoldierStatesFrame > endVehicleStatesFrame {
				endMissionEventFrame = endSoldierStatesFrame
			} else {
				endMissionEventFrame = endVehicleStatesFrame
			}

			fmt.Println("No endMission event found, using last soldier or vehicle state frame: ", endMissionEventFrame)
		} else {
			fmt.Println("Found endMission event at frame: ", endMissionEventFrame)
		}
		ocapMission["endFrame"] = endMissionEventFrame

		fmt.Printf("Got mission data from DB in %s\n", time.Since(txStart))

		// process soldier entities
		txStart = time.Now()
		var missionSoldiers []defs.Soldier

		err = DB.Model(&mission).Association("Soldiers").Find(&missionSoldiers)
		if err != nil {
			return err
		}
		fmt.Printf("Found %d soldiers in mission in %s\n", len(missionSoldiers), time.Since(txStart))

		// process soldier data and states into json
		txStart = time.Now()
		for _, dbSoldier := range missionSoldiers {
			// preprocess soldier data into an object
			jsonSoldier := make(map[string]interface{})
			jsonSoldier["id"] = dbSoldier.OcapID
			jsonSoldier["name"] = dbSoldier.UnitName
			jsonSoldier["group"] = dbSoldier.GroupID
			jsonSoldier["side"] = dbSoldier.Side
			jsonSoldier["isPlayer"] = 0
			if dbSoldier.IsPlayer {
				jsonSoldier["isPlayer"] = 1
			}
			jsonSoldier["role"] = dbSoldier.RoleDescription
			jsonSoldier["startFrameNum"] = dbSoldier.JoinFrame
			jsonSoldier["type"] = "unit"
			jsonSoldier["framesFired"] = make([]interface{}, 0)

			// get soldier states
			var soldierStates []defs.SoldierState
			err = DB.Model(&dbSoldier).Order("capture_frame ASC").Association("SoldierStates").Find(&soldierStates)
			if err != nil {
				return err
			}
			// if no states, skip this entity
			if len(soldierStates) == 0 {
				continue
			}

			// preprocess the soldier states into a map of capture frame to interface
			soldierStatesMap := defs.NewSoldierStatesMap()
			for _, state := range soldierStates {
				var thisPosition []interface{}

				pos := state.Position
				coords, _ := pos.Coordinates()
				posArr := []float64{coords.X, coords.Y, coords.Z}
				thisPosition = append(thisPosition, posArr)
				thisPosition = append(thisPosition, state.Bearing)
				thisPosition = append(thisPosition, state.Lifestate)
				if state.InVehicle {
					thisPosition = append(thisPosition, 1)
				} else {
					thisPosition = append(thisPosition, 0)
				}
				thisPosition = append(thisPosition, state.UnitName)
				if state.IsPlayer {
					thisPosition = append(thisPosition, 1)
				} else {
					thisPosition = append(thisPosition, 0)
				}
				// unused by web
				// thisPosition = append(thisPosition, state.CurrentRole)

				// add to map
				soldierStatesMap.Set(state.CaptureFrame, thisPosition)
			}

			// fmt.Printf("Got soldier states map of length %d in %s\n", soldierStatesMap.Len(), time.Since(txStart))

			// get "positions" arrays
			// start at JoinFrame
			jsonSoldier["positions"] = make([]interface{}, 0)
			for capFrame := dbSoldier.JoinFrame; capFrame <= endMissionEventFrame; capFrame++ {
				// get the soldier frame matching the capture frame
				state, err := soldierStatesMap.GetStateAtFrame(capFrame, endMissionEventFrame)
				if err == nil {
					// add frame to positions
					jsonSoldier["positions"] = append(jsonSoldier["positions"].([]interface{}), state)
				} else {
					// if no state, add last state to positions
					jsonSoldier["positions"] = append(jsonSoldier["positions"].([]interface{}), soldierStatesMap.GetLastState())
					continue
				}
			}

			// fired frames
			var firedEvents []defs.FiredEvent
			err = DB.Model(&dbSoldier).Order("capture_frame ASC").Association("FiredEvents").Find(&firedEvents)
			if err != nil {
				return err
			}

			for _, event := range firedEvents {
				var thisFiredFrame []interface{}
				thisFiredFrame = append(thisFiredFrame, event.CaptureFrame)
				endPos := event.EndPosition
				endPosCoords, _ := endPos.Coordinates()
				endPosArr := []float64{endPosCoords.X, endPosCoords.Y, endPosCoords.Z}
				thisFiredFrame = append(thisFiredFrame, endPosArr)

				jsonSoldier["framesFired"] = append(jsonSoldier["framesFired"].([]interface{}), thisFiredFrame)
			}

			ocapMission["entities"] = append(ocapMission["entities"].([]interface{}), jsonSoldier)
		}

		fmt.Printf("Processed soldier data in %s\n", time.Since(txStart))

		// process vehicle entities
		txStart = time.Now()
		var missionVehicles []defs.Vehicle

		err = DB.Model(&mission).Association("Vehicles").Find(&missionVehicles)
		if err != nil {
			return err
		}

		fmt.Printf("Found %d vehicles in mission in %s\n", len(missionVehicles), time.Since(txStart))

		// process vehicle data and states into json
		txStart = time.Now()
		for _, dbVehicle := range missionVehicles {
			// preprocess vehicle data into an object
			jsonVehicle := make(map[string]interface{})
			jsonVehicle["id"] = dbVehicle.OcapID
			jsonVehicle["name"] = dbVehicle.DisplayName
			jsonVehicle["class"] = dbVehicle.OcapType
			jsonVehicle["startFrameNum"] = dbVehicle.JoinFrame
			jsonVehicle["framesFired"] = make([]interface{}, 0)
			jsonVehicle["positions"] = make([]interface{}, 0)

			// get vehicle states
			var vehicleStates []defs.VehicleState
			err = DB.Model(&dbVehicle).Order("capture_frame ASC").Association("VehicleStates").Find(&vehicleStates)
			if err != nil {
				return err
			}
			// get custom position arrays
			lastState := make([]interface{}, 0)
			frameIndex := 0
			usedCaptureFrames := make(map[uint]bool)
			for _, state := range vehicleStates {
				var thisPosition []interface{}

				// check if a record was already written for this capture frame
				if usedCaptureFrames[state.CaptureFrame] {
					continue
				}

				pos := state.Position
				coords, _ := pos.Coordinates()
				posArr := []float64{coords.X, coords.Y, coords.Z}
				thisPosition = append(thisPosition, posArr)
				thisPosition = append(thisPosition, state.Bearing)
				thisPosition = append(thisPosition, state.IsAlive)
				crewArr := make([]int, 0)
				var crewStr string = state.Crew
				if crewStr != "" {
					crewStrs := strings.Split(crewStr, ",")
					for _, crew := range crewStrs {
						crewInt, err := strconv.Atoi(crew)
						if err != nil {
							return err
						}
						crewArr = append(crewArr, crewInt)
					}
				}
				thisPosition = append(thisPosition, crewArr)

				// the latest web code allows for a vehicle to be in the same position for multiple frames, denoted by the 5th element in the array, while only taking up a single element in the positions array to save space
				// if the position is the same as the last one, we just increment the frame count of the last one
				// select the first 3 elements of the position array to compare
				thisPosition = append(thisPosition, []uint{
					state.CaptureFrame,
					state.CaptureFrame,
				})
				if len(lastState) > 0 && reflect.DeepEqual(thisPosition[:3], lastState[:3]) {
					// if the position is the same, we just increment the frame count of the last one
					if len(lastState[4].([]uint)) != 2 {
						lastState[4] = []interface{}{
							lastState[4].([]uint)[0],
							lastState[4].([]uint)[0] + 1,
						}
					} else {
						lastState[4].([]uint)[1]++
					}
					// and ensure the positions array is up to date
					jsonVehicle["positions"].([]interface{})[frameIndex-1] = lastState
					// and add the capture frame to the used frames map
					usedCaptureFrames[state.CaptureFrame] = true
				} else {
					// we need to account for gaps in the capture frames, where the vehicle's last entry doesn't have [4][1] < captureFrame - 1
					// if the last state is not empty
					// and the last entry's end frame is less than the current state's capture frame - 1
					// then we need to add a new position to the positions array & also update the last state
					// first we need to update the last state's end frame
					if len(lastState) >= 5 &&
						lastState[4].([]uint)[1] < state.CaptureFrame-1 {
						lastState[4].([]uint)[1] = state.CaptureFrame - 1
						jsonVehicle["positions"].([]interface{})[frameIndex-1].([]interface{})[4] = lastState[4]
					}
					// then we add a new position to the positions array
					thisPosition[4] = []uint{state.CaptureFrame, state.CaptureFrame}
					// and add the capture frame to the used frames map
					usedCaptureFrames[state.CaptureFrame] = true
					// and update the last state and frame index
					lastState = thisPosition
					frameIndex++
					jsonVehicle["positions"] = append(jsonVehicle["positions"].([]interface{}), thisPosition)
				}
			}

			ocapMission["entities"] = append(ocapMission["entities"].([]interface{}), jsonVehicle)
		}

		fmt.Printf("Processed vehicle data in %s\n", time.Since(txStart))

		// process events
		txStart = time.Now()

		allEvents := [][]interface{}{}
		generalEvents := []defs.GeneralEvent{}
		hitEvents := []defs.HitEvent{}
		killEvents := []defs.KillEvent{}
		if err = DB.Model(&mission).Order("capture_frame ASC, time ASC").Association("GeneralEvents").Find(&generalEvents); err != nil {
			return err
		}

		if err = DB.Model(&mission).Order("capture_frame ASC, time ASC").Association("HitEvents").Find(&hitEvents); err != nil {
			return err
		}

		if err = DB.Model(&mission).Order("capture_frame ASC, time ASC").Association("KillEvents").Find(&killEvents); err != nil {
			return err
		}

		for _, generalEvent := range generalEvents {
			// preprocess event data into an object
			jsonEvent := make([]interface{}, 3)
			jsonEvent[0] = generalEvent.CaptureFrame
			jsonEvent[1] = generalEvent.Name
			if generalEvent.Name == "endMission" {
				// get json data from ExtraData
				var extraData map[string]interface{}
				if err = json.Unmarshal([]byte(generalEvent.ExtraData), &extraData); err != nil {
					return err
				}
				jsonEvent[2] = make([]string, 2)
				jsonEvent[2].([]string)[0] = extraData["winSide"].(string)
				jsonEvent[2].([]string)[1] = extraData["message"].(string)
			} else {
				jsonEvent[2] = generalEvent.Message
			}

			allEvents = append(allEvents, jsonEvent)
		}

		for _, hitEvent := range hitEvents {
			// preprocess event data into an object
			jsonEvent := make([]interface{}, 5)
			jsonEvent[0] = hitEvent.CaptureFrame
			jsonEvent[1] = "hit"
			// VictimVehicleID or VictimSoldierID, whichever is not empty
			victimID := uint(0)

			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", hitEvent.VictimSoldierID).Pluck(
				"ocap_id", &victimID,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
				jsonEvent[2] = victimID
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", hitEvent.VictimVehicleID).Pluck(
					"ocap_id", &victimID,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
					jsonEvent[2] = victimID
				} else {
					// if neither found, skip this event
					continue
				}
			}

			// causedby info
			causedBy := make([]interface{}, 2)
			causedByID := uint(0)
			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", hitEvent.ShooterSoldierID).Pluck(
				"ocap_id", &causedByID,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", hitEvent.ShooterVehicleID).Pluck(
					"ocap_id", &causedByID,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
				} else {
					// if neither found, skip this event
					continue
				}
			}

			causedByText := hitEvent.EventText
			causedBy[0] = causedByID
			causedBy[1] = causedByText
			jsonEvent[3] = causedBy

			// distance
			jsonEvent[4] = hitEvent.Distance

			allEvents = append(allEvents, jsonEvent)
		}

		for _, killEvent := range killEvents {
			// preprocess event data into an object
			jsonEvent := make([]interface{}, 5)
			jsonEvent[0] = killEvent.CaptureFrame
			jsonEvent[1] = "killed"
			// victimIDVehicle or victimIDSoldier, whichever is not empty
			victimID := uint(0)

			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", killEvent.VictimIDSoldier).Pluck(
				"ocap_id", &victimID,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
				jsonEvent[2] = victimID
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", killEvent.VictimIDVehicle).Pluck(
					"ocap_id", &victimID,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
					jsonEvent[2] = victimID
				} else {
					// if neither found, skip this event
					continue
				}
			}

			// causedby info
			causedBy := make([]interface{}, 2)
			var causedByID uint16
			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", killEvent.KillerIDSoldier).Pluck(
				"ocap_id", &causedByID,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", killEvent.KillerIDVehicle).Pluck(
					"ocap_id", &causedByID,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
				} else {
					// if neither found, skip this event
					continue
				}
			}

			causedByText := killEvent.EventText
			causedBy[0] = causedByID
			causedBy[1] = causedByText
			jsonEvent[3] = causedBy

			// distance
			jsonEvent[4] = killEvent.Distance

			allEvents = append(allEvents, jsonEvent)
		}

		ocapMission["events"] = allEvents

		fmt.Printf("Processed event data in %s\n", time.Since(txStart))

		// now we need to get the mission object in json format
		txStart = time.Now()
		missionJSON, err := json.Marshal(ocapMission)
		// missionJSON, err := json.MarshalIndent(ocapMission, "", "  ")
		if err != nil {
			return err
		}

		// write to gzipped json file
		filename := fmt.Sprintf("mission_%s_%s.json.gz", missionID, ocapMission["missionName"])
		filename = strings.ReplaceAll(filename, " ", "_")
		gzipFile, err := os.Create(filename)
		defer func() {
			err := gzipFile.Close()
			if err != nil {
				panic(err)
			}
		}()
		if err != nil {
			return err
		}

		gzipWriter := gzip.NewWriter(gzipFile)
		defer func() {
			err := gzipWriter.Close()
			if err != nil {
				panic(err)
			}
		}()
		bytes, err := gzipWriter.Write(missionJSON)
		if err != nil {
			return err
		}

		fmt.Printf("Wrote %d bytes for missionId %s in %s\n", bytes, missionID, time.Since(txStart))

		// add sqlite query to add to operations table
		queries = append(queries, fmt.Sprintf(
			`
INSERT INTO operations
(world_name, mission_name, mission_duration, filename, date, tag)
VALUES
('%s', '%s', %d, '%s', '%s', '%s');
			`,
			ocapMission["worldName"],
			ocapMission["missionName"],
			ocapMission["endFrame"],
			strings.Replace(filename, ".gz", "", 1),
			time.Now().Format("2006-01-02"),
			ocapMission["tags"],
		))
		fmt.Println()
	}

	fmt.Println("JSON data written for all missionIds")

	fmt.Println("To insert rows into your OCAP database, open the SQLite file in a proper client & run the following queries:")
	for _, query := range queries {
		fmt.Println(query)
	}

	fmt.Println()
	fmt.Println("Finished processing, press enter to exit.")
	return nil
}

func reduceMission(missionIDs []string) (err error) {
	fmt.Println("Reducing mission IDs: ", missionIDs)

	for _, missionID := range missionIDs {

		// get missionIdInt
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		// get mission data
		txStart := time.Now()
		var mission defs.Mission
		err = DB.Model(&defs.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return fmt.Errorf("error getting mission: %w", err)
		}

		// we want to reduce the number of soldier states by a factor of 5, so that we can still see the general movement of the soldiers, but not have to store every single frame
		// we'll be left with 1 state every 5 frames, plus the first and last states

		// get soldier states to remove
		soldierStatesToDelete := []defs.SoldierState{}
		err = DB.Model(&defs.SoldierState{}).Where(
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
	// get tables to vacuum
	tables := []string{}
	err = DB.Raw(
		`SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&tables).Error
	if err != nil {
		return fmt.Errorf("error getting tables to vacuum: %w", err)
	}

	// run vacuum on each table
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

	// get rows
	frameData := []defs.FrameData{}
	err = DB.Raw(query, 4, 0, 100).Scan(&frameData).Error
	if err != nil {
		fmt.Println(err)
		return
	}
	// create json data
	// jsonData := make(map[string]interface{})

	// for rows.Next() {

	// 	rowData := defs.FrameData{}
	// 	err = DB.ScanRows(rows, &rowData)
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		return
	// 	}

	// 	soldierId := strconv.Itoa(int(rowData.OcapId))
	// 	captureFrame := strconv.Itoa(int(rowData.CaptureFrame))

	// 	// check if soldier exists in jsonData
	// 	if jsonData[soldierId] == nil {
	// 		jsonData[soldierId] = make(map[string]interface{})
	// 	}
	// 	// add frameData to jsonData
	// 	jsonData[soldierId].(map[string]interface{})[captureFrame] = rowData
	// }

	// marshal and write data to file
	// jsonBytes, err := json.MarshalIndent(frameData, "", "  ")
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
	defs.Logger.Info().Msg("Starting up...")
	// connect to DB
	defs.Logger.Info().Msg("Connecting to DB...")
	err = initDB()
	if err != nil {
		panic(err)
	}
	defs.Logger.Info().Msg("DB connect/migrate complete.")
	initExtension()

	// get arguments
	args := os.Args[1:]
	if len(args) > 0 {
		if strings.ToLower(args[0]) == "demo" {
			defs.Logger.Info().Msg("Populating demo data...")
			IsDemoData = true
			demoStart := time.Now()
			populateDemoData()
			defs.Logger.Info().Dur("duration", time.Duration(time.Since(demoStart).Seconds())).Msg("Demo data populated.")
			// wait input
			fmt.Println("Press enter to exit.")
		}
		if strings.ToLower(args[0]) == "setupdb" {
			err = setupDB(DB)
			if err != nil {
				panic(err)
			}
			defs.Logger.Info().Msg("DB setup complete.")
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
			defs.Logger.Info().Msg("Finished migrating backups.")
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
