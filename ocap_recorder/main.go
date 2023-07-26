package main

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include "extensionCallback.h"
*/
import "C" // This is required to import the C code

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	defs "ocap_recorder/defs"

	_ "golang.org/x/sys/unix"

	"github.com/glebarez/sqlite"
	"github.com/twpayne/go-geom"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var EXTENSION_VERSION string = "0.0.1"
var extensionCallbackFnc C.extensionCallback

var ADDON string = "ocap"
var EXTENSION string = "ocap_recorder"

// file paths
var ADDON_FOLDER string = fmt.Sprintf(
	"%s\\@%s",
	getDir(),
	ADDON,
)
var LOG_FILE string = fmt.Sprintf(
	"%s\\%s.log",
	ADDON_FOLDER,
	EXTENSION,
)
var CONFIG_FILE string = fmt.Sprintf(
	"%s\\%s.cfg.json",
	ADDON_FOLDER,
	EXTENSION,
)
var LOCAL_DB_FILE string = ADDON_FOLDER + "\\ocap_recorder.db"

// global variables
var SAVE_LOCAL bool = false
var DB *gorm.DB
var DB_VALID bool = false
var sqlDB *sql.DB

var (
	// channels for receiving new data and filing to DB
	newSoldierChan      chan []string = make(chan []string, 1000)
	newVehicleChan      chan []string = make(chan []string, 1000)
	newSoldierStateChan chan []string = make(chan []string, 10000)
	newVehicleStateChan chan []string = make(chan []string, 10000)
	newFiredEventChan   chan []string = make(chan []string, 10000)
	newGeneralEventChan chan []string = make(chan []string, 1000)
	newHitEventChan     chan []string = make(chan []string, 2000)
	newKillEventChan    chan []string = make(chan []string, 2000)
	newChatEventChan    chan []string = make(chan []string, 1000)
	newRadioEventChan   chan []string = make(chan []string, 1000)
	newFpsEventChan     chan []string = make(chan []string, 1000)

	// caches of processed models pending DB write
	soldiersToWrite      = defs.SoldiersQueue{}
	soldierStatesToWrite = defs.SoldierStatesQueue{}
	vehiclesToWrite      = defs.VehiclesQueue{}
	vehicleStatesToWrite = defs.VehicleStatesQueue{}
	firedEventsToWrite   = defs.FiredEventsQueue{}
	generalEventsToWrite = defs.GeneralEventsQueue{}
	hitEventsToWrite     = defs.HitEventsQueue{}
	killEventsToWrite    = defs.KillEventsQueue{}
	chatEventsToWrite    = defs.ChatEventsQueue{}
	radioEventsToWrite   = defs.RadioEventsQueue{}
	fpsEventsToWrite     = defs.FpsEventsQueue{}

	// testing
	TEST_DATA         bool = false
	TEST_DATA_TIMEINC      = defs.SafeCounter{}

	// sqlite flow
	PAUSE_INSERTS bool = false

	SESSION_START_TIME  time.Time = time.Now()
	LAST_WRITE_DURATION time.Duration
)

type APIConfig struct {
	ServerURL string `json:"serverUrl"`
	APIKey    string `json:"apiKey"`
}

type DBConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type ConfigJson struct {
	Debug      bool      `json:"debug"`
	DefaultTag string    `json:"defaultTag"`
	LogsDir    string    `json:"logsDir"`
	APIConfig  APIConfig `json:"api"`
	DBConfig   DBConfig  `json:"db"`
}

var ActiveSettings ConfigJson = ConfigJson{}

// configure log output
func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// check if parent folder exists
	// if it doesn't, create it
	if _, err := os.Stat(ADDON_FOLDER); os.IsNotExist(err) {
		os.Mkdir(ADDON_FOLDER, 0755)
	}
	// check if LOG_FILE exists
	// if it does, move it to LOG_FILE.old
	// if it doesn't, create it
	if _, err := os.Stat(LOG_FILE); err == nil {
		os.Rename(LOG_FILE, LOG_FILE+".old")
	}
	f, err := os.OpenFile(LOG_FILE, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	log.SetOutput(f)

	loadConfig()
}

func setLogFile() (err error) {
	LOG_FILE = fmt.Sprintf(
		`%s\%s.%s.log`,
		ActiveSettings.LogsDir,
		EXTENSION,
		time.Now().Format("20060102_150405"),
	)
	log.Println("Log location:", LOG_FILE)
	LOCAL_DB_FILE = fmt.Sprintf(`%s\%s_%s.db`, ADDON_FOLDER, EXTENSION, SESSION_START_TIME.Format("20060102_150405"))

	// resolve path set in ActiveSettings.LogsDir
	// create logs dir if it doesn't exist
	if _, err := os.Stat(ActiveSettings.LogsDir); os.IsNotExist(err) {
		os.Mkdir(ActiveSettings.LogsDir, 0755)
	}
	// log to file
	f, err := os.OpenFile(LOG_FILE, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	log.SetOutput(f)

	return nil
}

func version() {
	functionName := "version"
	writeLog(functionName, fmt.Sprintf(`ocap_recorder version: %s`, EXTENSION_VERSION), "INFO")
}

func getDir() string {
	dir, err := os.Getwd()
	if err != nil {
		writeLog("getDir", fmt.Sprintf(`error getting working directory: %v`, err), "ERROR")
		return ""
	}
	return dir
}

func loadConfig() {
	// load config from file as JSON
	functionName := "loadConfig"

	file, err := os.OpenFile(CONFIG_FILE, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`%s`, err), "ERROR")
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&ActiveSettings)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`%s`, err), "ERROR")
		return
	}

	checkServerStatus()

	writeLog(functionName, `Config loaded`, "INFO")
}

func checkServerStatus() {
	functionName := "checkServerStatus"
	var err error

	// check if server is running by making a healthcheck API request
	// if server is not running, log error and exit
	_, err = http.Get(ActiveSettings.APIConfig.ServerURL + "/healthcheck")
	if err != nil {
		writeLog(functionName, `OCAP Frontend is offline`, "WARN")
	} else {
		writeLog(functionName, `OCAP Frontend is online`, "INFO")
	}
}

func getProgramStatus(
	rawBuffers bool,
	writeQueues bool,
	lastWrite bool,
) (output []string, model defs.OcapPerformance) {
	// returns a slice of strings containing the current program status
	// rawBuffers: include raw buffers in output
	rawBuffersStr := fmt.Sprintf("TO PROCESS: Soldiers: %d | Vehicles: %d | SoldierStates: %d | VehicleStates: %d | FiredEvents: %d",
		len(newSoldierChan),
		len(newVehicleChan),
		len(newSoldierStateChan),
		len(newVehicleStateChan),
		len(newFiredEventChan),
	)
	// writeQueues: include write queues in output
	writeQueuesStr := fmt.Sprintf("AWAITING WRITE: Soldiers: %d | Vehicles: %d | SoldierStates: %d | VehicleStates: %d | FiredEvents: %d",
		soldiersToWrite.Len(),
		vehiclesToWrite.Len(),
		soldierStatesToWrite.Len(),
		vehicleStatesToWrite.Len(),
		firedEventsToWrite.Len(),
	)
	// lastWrite: include last write duration in output
	lastWriteStr := fmt.Sprintf("LAST WRITE TOOK: %s", LAST_WRITE_DURATION)

	if rawBuffers {
		output = append(output, rawBuffersStr)
	}
	if writeQueues {
		output = append(output, writeQueuesStr)
	}
	if lastWrite {
		output = append(output, lastWriteStr)
	}

	buffersObj := defs.BufferLengths{
		Soldiers:        uint16(len(newSoldierChan)),
		Vehicles:        uint16(len(newVehicleChan)),
		SoldierStates:   uint16(len(newSoldierStateChan)),
		VehicleStates:   uint16(len(newVehicleStateChan)),
		FiredEvents:     uint16(len(newFiredEventChan)),
		GeneralEvents:   uint16(len(newGeneralEventChan)),
		HitEvents:       uint16(len(newHitEventChan)),
		KillEvents:      uint16(len(newKillEventChan)),
		ChatEvents:      uint16(len(newChatEventChan)),
		RadioEvents:     uint16(len(newRadioEventChan)),
		ServerFpsEvents: uint16(len(newFpsEventChan)),
	}

	writeQueuesObj := defs.WriteQueueLengths{
		Soldiers:        uint16(soldiersToWrite.Len()),
		Vehicles:        uint16(vehiclesToWrite.Len()),
		SoldierStates:   uint16(soldierStatesToWrite.Len()),
		VehicleStates:   uint16(vehicleStatesToWrite.Len()),
		FiredEvents:     uint16(firedEventsToWrite.Len()),
		GeneralEvents:   uint16(generalEventsToWrite.Len()),
		HitEvents:       uint16(hitEventsToWrite.Len()),
		KillEvents:      uint16(killEventsToWrite.Len()),
		ChatEvents:      uint16(chatEventsToWrite.Len()),
		RadioEvents:     uint16(radioEventsToWrite.Len()),
		ServerFpsEvents: uint16(fpsEventsToWrite.Len()),
	}

	perf := defs.OcapPerformance{
		Time:              time.Now(),
		MissionID:         CurrentMission.ID,
		BufferLengths:     buffersObj,
		WriteQueueLengths: writeQueuesObj,
		// get float32 in ms
		LastWriteDurationMs: float32(LAST_WRITE_DURATION.Milliseconds()),
	}

	return output, perf
}

///////////////////////
// DATABASE OPS //
///////////////////////

func getLocalDB() (err error) {
	functionName := "getDB"
	// connect to database (SQLite)
	DB, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		CreateBatchSize:        2000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		DB_VALID = false
		return err
	} else {
		writeLog(functionName, "Using local SQlite DB", "INFO")

		// set PRAGMAS
		err = DB.Exec("PRAGMA user_version = 1;").Error
		if err != nil {
			writeLog(functionName, "Error setting user_version PRAGMA", "ERROR")
			return err
		}
		err = DB.Exec("PRAGMA journal_mode = MEMORY;").Error
		if err != nil {
			writeLog(functionName, "Error setting journal_mode PRAGMA", "ERROR")
			return err
		}
		err = DB.Exec("PRAGMA synchronous = OFF;").Error
		if err != nil {
			writeLog(functionName, "Error setting synchronous PRAGMA", "ERROR")
			return err
		}
		err = DB.Exec("PRAGMA cache_size = -32000;").Error
		if err != nil {
			writeLog(functionName, "Error setting cache_size PRAGMA", "ERROR")
			return err
		}
		err = DB.Exec("PRAGMA temp_store = MEMORY;").Error
		if err != nil {
			writeLog(functionName, "Error setting temp_store PRAGMA", "ERROR")
			return err
		}

		err = DB.Exec("PRAGMA page_size = 32768;").Error
		if err != nil {
			writeLog(functionName, "Error setting page_size PRAGMA", "ERROR")
			return err
		}

		err = DB.Exec("PRAGMA mmap_size = 30000000000;").Error
		if err != nil {
			writeLog(functionName, "Error setting mmap_size PRAGMA", "ERROR")
			return err
		}

		DB_VALID = true
		return nil
	}
}

func dumpMemoryDBToDisk() (err error) {
	functionName := "dumpMemoryDBToDisk"
	// remove existing file if it exists
	exists, err := os.Stat(LOCAL_DB_FILE)
	if err == nil {
		if exists != nil {
			err = os.Remove(LOCAL_DB_FILE)
			if err != nil {
				writeLog(functionName, "Error removing existing DB file", "ERROR")
				return err
			}
		}
	}

	// dump memory DB to disk
	start := time.Now()
	err = DB.Exec("VACUUM INTO 'file:" + LOCAL_DB_FILE + "';").Error
	if err != nil {
		writeLog(functionName, "Error dumping memory DB to disk", "ERROR")
		return err
	}
	writeLog(functionName, fmt.Sprintf(`Dumped memory DB to disk in %s`, time.Since(start)), "INFO")
	return nil
}

func connectMySql() (err error) {
	// connect to database (MySQL/MariaDB)
	dsn := fmt.Sprintf(`%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True`,
		ActiveSettings.DBConfig.Username,
		ActiveSettings.DBConfig.Password,
		ActiveSettings.DBConfig.Host,
		ActiveSettings.DBConfig.Port,
		ActiveSettings.DBConfig.Database,
	)

	// wrap with gorm
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		CreateBatchSize:        2000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		writeLog("connectMySql", fmt.Sprintf(`Failed to connect to MySQL/MariaDB. Err: %s`, err), "ERROR")
		DB_VALID = false
		return
	} else {
		writeLog("connectMySql", "Connected to MySQL/MariaDB", "INFO")
		DB_VALID = true
		return
	}

}

// getDB connects to the MySql/MariaDB database, and if it fails, it will use a local SQlite DB
func getDB() (err error) {
	functionName := ":INIT:DB:"

	// connect to database (Postgres) using gorm
	dsn := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable`,
		ActiveSettings.DBConfig.Host,
		ActiveSettings.DBConfig.Port,
		ActiveSettings.DBConfig.Username,
		ActiveSettings.DBConfig.Password,
		ActiveSettings.DBConfig.Database,
	)

	DB, err = gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{
		// PrepareStmt:            true,
		SkipDefaultTransaction: true,
		CreateBatchSize:        10000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	configValid := err == nil
	if !configValid {
		writeLog(functionName, fmt.Sprintf(`Failed to set up Postgres. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		getLocalDB()
	}
	// test connection
	sqlDB, err = DB.DB()
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error getting sql database, trying ping. Err: %s`, err), "ERROR")
	}
	err = sqlDB.Ping()
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Failed to connect to Postgres. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		err = getLocalDB()
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to connect to SQLite. Err: %s`, err), "ERROR")
			DB_VALID = false
		} else {
			DB_VALID = true
		}
	} else {
		writeLog(functionName, "Connected to Postgres", "INFO")
		SAVE_LOCAL = false
		DB_VALID = true
	}

	if !DB_VALID {
		writeLog(functionName, "DB not valid. Not saving!", "ERROR")
		return fmt.Errorf("db not valid. not saving")
	}

	if !SAVE_LOCAL {
		// Ensure PostGIS extension is installed
		err = DB.Exec(`
		CREATE EXTENSION IF NOT EXISTS postgis;
		`).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create PostGIS extension. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		} else {
			writeLog(functionName, "PostGIS extension created", "INFO")
		}
	}

	// Check if OcapInfo table exists
	if !DB.Migrator().HasTable(&defs.OcapInfo{}) {
		// Create the table
		err = DB.AutoMigrate(&defs.OcapInfo{})
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create ocap_info table. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
		// Create the default settings
		err = DB.Create(&defs.OcapInfo{
			GroupName:        "OCAP",
			GroupDescription: "OCAP",
			GroupLogo:        "https://i.imgur.com/0Q4z0ZP.png",
			GroupWebsite:     "https://ocap.arma3.com",
		}).Error

		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create ocap_info entry. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
	}

	/////////////////////////////
	// Migrate the schema
	/////////////////////////////

	toMigrate := make([]interface{}, 0)
	// system models
	toMigrate = append(toMigrate, &defs.OcapInfo{})
	// aar models
	toMigrate = append(toMigrate, &defs.AfterActionReview{})
	toMigrate = append(toMigrate, &defs.World{})
	toMigrate = append(toMigrate, &defs.Mission{})
	toMigrate = append(toMigrate, &defs.Soldier{})
	toMigrate = append(toMigrate, &defs.SoldierState{})
	toMigrate = append(toMigrate, &defs.Vehicle{})
	toMigrate = append(toMigrate, &defs.VehicleState{})
	toMigrate = append(toMigrate, &defs.FiredEvent{})
	toMigrate = append(toMigrate, &defs.GeneralEvent{})
	toMigrate = append(toMigrate, &defs.HitEvent{})
	toMigrate = append(toMigrate, &defs.KillEvent{})
	toMigrate = append(toMigrate, &defs.ChatEvent{})
	toMigrate = append(toMigrate, &defs.RadioEvent{})
	toMigrate = append(toMigrate, &defs.ServerFpsEvent{})
	toMigrate = append(toMigrate, &defs.OcapPerformance{})

	err = DB.AutoMigrate(toMigrate...)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Failed to migrate DB schema. Err: %s`, err), "ERROR")
		DB_VALID = false
		return err
	}

	if !SAVE_LOCAL {
		// if running TimescaleDB (Postgres), configure
		sqlDB, err = DB.DB()
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to get DB.DB(). Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}

		sqlDB.SetMaxOpenConns(30)
	}

	writeLog(
		functionName,
		fmt.Sprintf("DB Valid: %v", DB_VALID),
		"INFO",
	)

	// init caches
	TEST_DATA_TIMEINC.Set(0)

	startAsyncProcessors()
	startDBWriters()

	// goroutine to, every x seconds, pause insert execution and dump memory sqlite db to disk
	go func() {
		for {
			if !DB_VALID || !SAVE_LOCAL {
				return
			}

			time.Sleep(3 * time.Minute)

			// pause insert execution
			PAUSE_INSERTS = true

			// dump memory sqlite db to disk
			err = dumpMemoryDBToDisk()
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Error dumping memory db to disk: %v`, err), "ERROR")
			}

			// resume insert execution
			PAUSE_INSERTS = false
		}
	}()

	// start a goroutine that will output channel lengths every second
	go func() {
		// get status file writer with full control of file
		statusFile, err := os.OpenFile(ADDON_FOLDER+"/status.txt", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Error opening status file: %v`, err), "ERROR")
		}
		defer statusFile.Close()
		for {
			time.Sleep(3000 * time.Millisecond)
			// clear the file contents and then write status
			statusFile.Truncate(0)
			statusFile.Seek(0, 0)
			statusStr, model := getProgramStatus(true, true, true)
			for _, line := range statusStr {
				statusFile.WriteString(line + "\n")
			}

			// write model to db
			err = DB.Create(&model).Error
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Error writing ocap perfromance to db: %v`, err), "ERROR")
			}
		}
	}()

	// log post goroutine creation
	writeLog(functionName, "Goroutines started successfully", "INFO")

	// only if everything worked should we send a callback letting the addon know we're ready
	if DB_VALID {
		writeLog(":DB:OK:", "DB ready", "INFO")
	}

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
		} else {
			writeLog(functionName, fmt.Sprintf(`Created hypertable for %s`, table), "INFO")
		}

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
		} else {
			writeLog(functionName, fmt.Sprintf(`Enabled hypertable compression for %s`, table), "INFO")
		}

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
		} else {
			writeLog(functionName, fmt.Sprintf(`Set compress_after for %s`, table), "INFO")
		}
	}
	return nil
}

// WORLDS AND MISSIONS
var CurrentWorld defs.World
var CurrentMission defs.Mission

// logNewMission logs a new mission to the database and associates the world it's played on
func logNewMission(data []string) (err error) {
	functionName := ":NEW:MISSION:"

	setLogFile()

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

	writeLog(functionName, fmt.Sprintf(`World data: %v`, world), "DEBUG")

	// preprocess the world 'location' to geopoint
	worldLocation := defs.GPSFromCoords(
		float64(world.Longitude),
		float64(world.Latitude),
		4326,
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

	writeLog(functionName, fmt.Sprintf(`Mission data: %v`, missionTemp), "DEBUG")

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
			} else {
				addons = append(addons, thisAddon)
			}
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

	writeLog(functionName, fmt.Sprintf(`Mission: %v`, mission), "DEBUG")

	// check if world exists
	err = DB.Where("world_name = ?", world.WorldName).First(&world).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		writeLog(functionName, fmt.Sprintf(`Error checking if world exists: %v`, err), "ERROR")
		return err
	} else if err == gorm.ErrRecordNotFound {
		// world does not exist, create it
		err = DB.Create(&world).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Error creating world: %v`, err), "ERROR")
			return err
		}
	}

	// always write new mission
	mission.World = world
	err = DB.Create(&mission).Error
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error creating mission: %v`, err), "ERROR")
		return err
	}

	// write to log
	writeLog(functionName, fmt.Sprintf(`New mission logged: %s`, mission.MissionName), "INFO")

	// set current world and mission
	CurrentWorld = world
	CurrentMission = mission

	writeLog(`:MISSION:OK:`, `[0, "OK"]`, "INFO")
	return nil
}

// (UNITS) AND VEHICLES

// logNewSoldier logs a new soldier to the database
func logNewSoldier(data []string) (soldier defs.Soldier, err error) {
	functionName := ":NEW:SOLDIER:"
	// check if DB is valid
	if !DB_VALID {
		return
	}

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

	ocapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return soldier, err
	}
	soldier.OcapID = uint16(ocapId)
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

	return soldier, nil
}

// logSoldierState logs a SoldierState state to the database
func logSoldierState(data []string) (soldierState defs.SoldierState, err error) {
	functionName := ":NEW:SOLDIER:STATE:"
	// check if DB is valid
	if !DB_VALID {
		return soldierState, nil
	}

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
	ocapId, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		return soldierState, fmt.Errorf(`error converting ocapId to uint: %v`, err)
	}

	// try and find soldier in DB to associate
	soldier := defs.Soldier{}
	err = DB.Model(&defs.Soldier{}).Order(
		"join_time DESC",
	).Where(
		&defs.Soldier{
			OcapID:    uint16(ocapId),
			MissionID: CurrentMission.ID,
		}).First(&soldier).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound && capframe < 10 {
			return defs.SoldierState{}, errTooEarlyForStateAssociation
		}
		json, _ := json.Marshal(data)
		return soldierState, fmt.Errorf("error finding soldier in DB:\n%s\n%v\nMissionID: %d", json, err, CurrentMission.ID)
	}
	soldierState.Soldier = soldier

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
	point, elev, err := defs.GPSFromString(pos, 3857, SAVE_LOCAL)
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

	soldierState.VehicleRole = data[12]

	return soldierState, nil
}

// log a new vehicle
func logNewVehicle(data []string) (vehicle defs.Vehicle, err error) {
	functionName := ":NEW:VEHICLE:"
	// check if DB is valid
	if !DB_VALID {
		return vehicle, nil
	}

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
	ocapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return vehicle, err
	}
	vehicle.OcapID = uint16(ocapId)
	vehicle.OcapType = data[2]
	vehicle.DisplayName = data[3]
	vehicle.ClassName = data[4]
	vehicle.Customization = data[5]

	return vehicle, nil
}

func logVehicleState(data []string) (vehicleState defs.VehicleState, err error) {
	functionName := ":NEW:VEHICLE:STATE:"
	// check if DB is valid
	if !DB_VALID {
		return vehicleState, nil
	}

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
	ocapId, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return vehicleState, err
	}

	// try and find vehicle in DB to associate
	vehicle := defs.Vehicle{}
	err = DB.Model(&defs.Vehicle{}).Order(
		"join_time DESC",
	).Where(
		&defs.Vehicle{
			OcapID:    uint16(ocapId),
			MissionID: CurrentMission.ID,
		}).First(&vehicle).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound && capframe < 10 {
			return defs.VehicleState{}, errTooEarlyForStateAssociation
		}
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error finding vehicle in DB:\n%s\n%v", json, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.Vehicle = vehicle

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
	point, elev, err := defs.GPSFromString(pos, 3857, SAVE_LOCAL)
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

	return vehicleState, nil
}

// FIRED EVENTS
func logFiredEvent(data []string) (firedEvent defs.FiredEvent, err error) {
	functionName := ":FIRED:"
	// check if DB is valid
	if !DB_VALID {
		return firedEvent, nil
	}

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
	ocapId, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return firedEvent, err
	}

	// try and find soldier in DB to associate
	soldierId := uint(0)
	err = DB.Model(&defs.Soldier{}).Select("id").Order(
		"join_time DESC",
	).Where(
		&defs.Soldier{
			OcapID:    uint16(ocapId),
			MissionID: CurrentMission.ID,
		}).Limit(1).Scan(&soldierId).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error finding soldier in DB:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	} else if err == gorm.ErrRecordNotFound {
		if capframe < 10 {
			return firedEvent, errTooEarlyForStateAssociation
		}
		// soldier not found, return
		return firedEvent, nil
	}
	firedEvent.SoldierID = soldierId

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
	endpoint, endelev, err := defs.GPSFromString(endpos, 3857, SAVE_LOCAL)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.EndPosition = endpoint
	firedEvent.EndElevationASL = float32(endelev)

	// parse BULLET START POS from an arma string
	startpos := data[3]
	startpoint, startelev, err := defs.GPSFromString(startpos, 3857, SAVE_LOCAL)
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

	// check if DB is valid
	if !DB_VALID {
		return thisEvent, nil
	}

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

	thisEvent.Mission = CurrentMission

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
	// check if DB is valid
	if !DB_VALID {
		return hitEvent, nil
	}

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
	hitEvent.Mission = CurrentMission

	// timestamp will always be appended as the last element of data, in unixnano format as a string
	timestampStr := data[len(data)-1]
	timestampInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting timestamp to int: %v`, err)
	}
	hitEvent.Time = time.Unix(0, timestampInt)

	// parse data in array
	victimOcapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	victimSoldier := defs.Soldier{}
	// first, look in soldiers
	err = DB.Model(&defs.Soldier{}).Where(
		&defs.Soldier{
			OcapID:    uint16(victimOcapId),
			MissionID: CurrentMission.ID,
		}).First(&victimSoldier).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return hitEvent, fmt.Errorf(`error finding victim in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
	}
	if err == nil {
		hitEvent.VictimSoldier = victimSoldier
	} else if err == gorm.ErrRecordNotFound {
		// if not found, look in vehicles
		victimVehicle := defs.Vehicle{}
		err = DB.Model(&defs.Vehicle{}).Where(
			&defs.Vehicle{
				OcapID:    uint16(victimOcapId),
				MissionID: CurrentMission.ID,
			}).First(&victimVehicle).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return hitEvent, fmt.Errorf(`error finding victim in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.HitEvent{}, errTooEarlyForStateAssociation
			}
			return hitEvent, fmt.Errorf(`victim ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
		} else {
			hitEvent.VictimVehicle = victimVehicle
		}
	}

	// now look for the shooter
	shooterOcapId, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting shooter ocap id to uint: %v`, err)
	}

	// try and find shooter in DB to associate
	// first, look in soldiers
	shooterSoldier := defs.Soldier{}
	err = DB.Model(&defs.Soldier{}).Where(
		&defs.Soldier{
			OcapID:    uint16(shooterOcapId),
			MissionID: CurrentMission.ID,
		}).First(&shooterSoldier).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return hitEvent, fmt.Errorf(`error finding shooter in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, shooterOcapId, err)
	}
	if err == nil {
		hitEvent.ShooterSoldier = shooterSoldier
	} else if err == gorm.ErrRecordNotFound {
		// if not found, look in vehicles
		shooterVehicle := defs.Vehicle{}
		err = DB.Model(&defs.Vehicle{}).Where(
			&defs.Vehicle{
				OcapID:    uint16(shooterOcapId),
				MissionID: CurrentMission.ID,
			}).First(&shooterVehicle).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return hitEvent, fmt.Errorf(`error finding shooter in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, shooterOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.HitEvent{}, errTooEarlyForStateAssociation
			}
			return hitEvent, fmt.Errorf(`shooter ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, shooterOcapId, err)
		} else {
			hitEvent.ShooterVehicle = shooterVehicle
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
	// check if DB is valid
	if !DB_VALID {
		return killEvent, nil
	}

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
	killEvent.Mission = CurrentMission

	// parse data in array
	victimOcapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	// first, look in soldiers
	victimSoldier := defs.Soldier{}
	err = DB.Model(&defs.Soldier{}).Where(
		&defs.Soldier{
			OcapID:    uint16(victimOcapId),
			MissionID: CurrentMission.ID,
		}).First(&victimSoldier).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return killEvent, fmt.Errorf(`error finding victim in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
	}
	if err == nil {
		killEvent.VictimSoldier = victimSoldier
	} else if err == gorm.ErrRecordNotFound {
		// if not found, look in vehicles
		victimVehicle := defs.Vehicle{}
		err = DB.Model(&defs.Vehicle{}).Where(
			&defs.Vehicle{
				OcapID:  uint16(victimOcapId),
				Mission: CurrentMission,
			}).First(&victimVehicle).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return killEvent, fmt.Errorf(`error finding victim in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.KillEvent{}, errTooEarlyForStateAssociation
			}
			return killEvent, fmt.Errorf(`victim ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, victimOcapId, err)
		} else {
			killEvent.VictimVehicle = victimVehicle
		}
	}

	// now look for the killer
	killerOcapId, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting killer ocap id to uint: %v`, err)
	}

	// try and find killer in DB to associate
	// first, look in soldiers
	killerSoldier := defs.Soldier{}
	err = DB.Model(&defs.Soldier{}).Where(
		&defs.Soldier{
			OcapID:    uint16(killerOcapId),
			MissionID: CurrentMission.ID,
		}).First(&killerSoldier).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return killEvent, fmt.Errorf(`error finding killer in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, killerOcapId, err)
	} else if err == nil {
		killEvent.KillerSoldier = killerSoldier
	} else if err == gorm.ErrRecordNotFound {
		// if not found, look in vehicles
		killerVehicle := defs.Vehicle{}
		err = DB.Model(&defs.Vehicle{}).Where(
			&defs.Vehicle{
				OcapID:  uint16(killerOcapId),
				Mission: CurrentMission,
			}).First(&killerVehicle).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return killEvent, fmt.Errorf(`error finding killer in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, killerOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.KillEvent{}, errTooEarlyForStateAssociation
			}
			return killEvent, fmt.Errorf(`killer ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, killerOcapId, err)
		} else {
			killEvent.KillerVehicle = killerVehicle
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
	// check if DB is valid
	if !DB_VALID {
		return chatEvent, nil
	}

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
	chatEvent.Mission = CurrentMission

	// parse data in array
	senderOcapId, err := strconv.ParseInt(data[1], 10, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// try and find sender solder in DB to associate if not -1
	if senderOcapId != -1 {
		senderSoldier := defs.Soldier{}
		err = DB.Model(&defs.Soldier{}).Where(
			&defs.Soldier{
				OcapID:    uint16(senderOcapId),
				MissionID: CurrentMission.ID,
			}).First(&senderSoldier).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return chatEvent, fmt.Errorf(`error finding sender in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, senderOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.ChatEvent{}, errTooEarlyForStateAssociation
			}
			return chatEvent, fmt.Errorf(`sender ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, senderOcapId, err)
		} else if err == nil {
			chatEvent.Soldier = senderSoldier
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
		chatEvent.Channel = "System"
	}

	// next is from (formatted as the game message)
	chatEvent.FromName = data[3]

	// next is actual name
	chatEvent.SenderName = data[4]

	// next is message
	chatEvent.Message = data[5]

	// next is playerUID
	chatEvent.PlayerUid = data[6]

	return chatEvent, nil
}

// radio events
func logRadioEvent(data []string) (radioEvent defs.RadioEvent, err error) {
	// check if DB is valid
	if !DB_VALID {
		return radioEvent, nil
	}

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
	radioEvent.Mission = CurrentMission

	// parse data in array
	senderOcapId, err := strconv.ParseInt(data[1], 10, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// try and find sender solder in DB to associate if not -1
	if senderOcapId != -1 {
		senderSoldier := defs.Soldier{}
		err = DB.Model(&defs.Soldier{}).Where(
			&defs.Soldier{
				OcapID:    uint16(senderOcapId),
				MissionID: CurrentMission.ID,
			}).First(&senderSoldier).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return radioEvent, fmt.Errorf(`error finding sender in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, senderOcapId, err)
		} else if err == gorm.ErrRecordNotFound {
			if capframe < 10 {
				return defs.RadioEvent{}, errTooEarlyForStateAssociation
			}
			return radioEvent, fmt.Errorf(`sender ocap id not found in db: mission_id = %d AND ocap_id = %d - err: %s`, CurrentMission.ID, senderOcapId, err)
		} else if err == nil {
			radioEvent.Soldier = senderSoldier
		}
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
	// check if DB is valid
	if !DB_VALID {
		return fpsEvent, nil
	}

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
	fpsEvent.Mission = CurrentMission

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

///////////////////////
// GOROUTINES //
///////////////////////

func startAsyncProcessors() {
	functionName := ":DATA:PROCESSOR:"

	// these goroutines will receive data from buffered channels & process them into defined models, then push them into queues for writing

	go func() {
		// process channel data
		for v := range newSoldierChan {
			if !DB_VALID {
				return
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
		for v := range newVehicleChan {
			if !DB_VALID {
				return
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
		for v := range newSoldierStateChan {
			if !DB_VALID {
				return
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
		for v := range newVehicleStateChan {
			if !DB_VALID {
				return
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
		for v := range newFiredEventChan {
			if !DB_VALID {
				return
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

	go func() {
		// process channel data
		for v := range newGeneralEventChan {
			if !DB_VALID {
				return
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
		for v := range newHitEventChan {
			if !DB_VALID {
				return
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
		for v := range newKillEventChan {
			if !DB_VALID {
				return
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
		for v := range newChatEventChan {
			if !DB_VALID {
				return
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
		for v := range newRadioEventChan {
			if !DB_VALID {
				return
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
		for v := range newFpsEventChan {
			if !DB_VALID {
				return
			}

			obj, err := logFpsEvent(v)
			if err == nil {
				fpsEventsToWrite.Push([]defs.ServerFpsEvent{obj})
			} else {
				writeLog(functionName, fmt.Sprintf(`Failed to log fps event. Err: %s`, err), "ERROR")
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
			if !DB_VALID {
				return
			}

			if PAUSE_INSERTS {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			var (
				writeStart time.Time = time.Now()
			)

			// write new soldiers
			if !soldiersToWrite.Empty() {
				tx := DB.Begin()
				soldiersToWrite.Lock()
				err := tx.Create(&soldiersToWrite.Queue).Error
				soldiersToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldiers: %v`, err), "ERROR")
					tx.Rollback()
				}
				soldiersToWrite.Clear()
			}

			// write soldier states
			if !soldierStatesToWrite.Empty() {
				tx := DB.Begin()
				soldierStatesToWrite.Lock()
				err := tx.Create(&soldierStatesToWrite.Queue).Error
				soldierStatesToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldier states: %v`, err), "ERROR")
					tx.Rollback()
				}
				soldierStatesToWrite.Clear()
			}

			// write new vehicles
			if !vehiclesToWrite.Empty() {
				tx := DB.Begin()
				vehiclesToWrite.Lock()
				err := tx.Create(&vehiclesToWrite.Queue).Error
				vehiclesToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicles: %v`, err), "ERROR")
					tx.Rollback()
				}
				vehiclesToWrite.Clear()
			}

			// write vehicle states
			if !vehicleStatesToWrite.Empty() {
				tx := DB.Begin()
				vehicleStatesToWrite.Lock()
				err := tx.Create(&vehicleStatesToWrite.Queue).Error
				vehicleStatesToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicle states: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					writeLog(functionName, stmt, "ERROR")
					tx.Rollback()
				}
				vehicleStatesToWrite.Clear()
			}

			// write fired events
			if !firedEventsToWrite.Empty() {
				tx := DB.Begin()
				firedEventsToWrite.Lock()
				err := tx.Create(&firedEventsToWrite.Queue).Error
				firedEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating fired events: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					writeLog(functionName, stmt, "ERROR")
					tx.Rollback()
				}
				firedEventsToWrite.Clear()
			}

			// write general events
			if !generalEventsToWrite.Empty() {
				tx := DB.Begin()
				generalEventsToWrite.Lock()
				err := tx.Create(&generalEventsToWrite.Queue).Error
				generalEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating general events: %v`, err), "ERROR")
					tx.Rollback()
				}
				generalEventsToWrite.Clear()
			}

			// write hit events
			if !hitEventsToWrite.Empty() {
				tx := DB.Begin()
				hitEventsToWrite.Lock()
				err := tx.Create(&hitEventsToWrite.Queue).Error
				hitEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating hit events: %v`, err), "ERROR")
					tx.Rollback()
				}
				hitEventsToWrite.Clear()
			}

			// write kill events
			if !killEventsToWrite.Empty() {
				tx := DB.Begin()
				killEventsToWrite.Lock()
				err := tx.Create(&killEventsToWrite.Queue).Error
				killEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating killed events: %v`, err), "ERROR")
					tx.Rollback()
				}
				killEventsToWrite.Clear()
			}

			// write chat events
			if !chatEventsToWrite.Empty() {
				tx := DB.Begin()
				chatEventsToWrite.Lock()
				err := tx.Create(&chatEventsToWrite.Queue).Error
				chatEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating chat events: %v`, err), "ERROR")
					tx.Rollback()
				}
				chatEventsToWrite.Clear()
			}

			// write radio events
			if !radioEventsToWrite.Empty() {
				tx := DB.Begin()
				radioEventsToWrite.Lock()
				err := tx.Create(&radioEventsToWrite.Queue).Error
				radioEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating radio events: %v`, err), "ERROR")
					tx.Rollback()
				}
				radioEventsToWrite.Clear()
			}

			// write serverfps events
			if !fpsEventsToWrite.Empty() {
				tx := DB.Begin()
				fpsEventsToWrite.Lock()
				err := tx.Create(&fpsEventsToWrite.Queue).Error
				fpsEventsToWrite.Unlock()
				tx.Commit()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating serverfps events: %v`, err), "ERROR")
					tx.Rollback()
				}
				fpsEventsToWrite.Clear()
			}

			LAST_WRITE_DURATION = time.Since(writeStart)

			// sleep
			time.Sleep(750 * time.Millisecond)

		}
	}()
}

///////////////////////
// EXPORTED FUNCTIONS //
///////////////////////

func runExtensionCallback(name *C.char, function *C.char, data *C.char) C.int {
	return C.runExtensionCallback(extensionCallbackFnc, name, function, data)
}

//export goRVExtensionVersion
func goRVExtensionVersion(output *C.char, outputsize C.size_t) {
	result := C.CString(EXTENSION_VERSION)
	defer C.free(unsafe.Pointer(result))
	var size = C.strlen(result) + 1
	if size > outputsize {
		size = outputsize
	}
	C.memmove(unsafe.Pointer(output), unsafe.Pointer(result), size)
}

//export goRVExtensionArgs
func goRVExtensionArgs(output *C.char, outputsize C.size_t, input *C.char, argv **C.char, argc C.int) {
	var offset = unsafe.Sizeof(uintptr(0))
	var out []string
	for index := C.int(0); index < argc; index++ {
		out = append(out, C.GoString(*argv))
		argv = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(argv)) + offset))
	}

	// temp := fmt.Sprintf("Function: %s nb params: %d params: %s!", C.GoString(input), argc, out)
	temp := fmt.Sprintf(`["Function: %s", "nb params: %d"]`, C.GoString(input), argc)

	timestamp := time.Now()

	switch C.GoString(input) {
	case ":INIT:DB:":
		go getDB()
	case ":NEW:MISSION:":
		go logNewMission(out)
	case ":NEW:SOLDIER:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newSoldierChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":NEW:SOLDIER:STATE:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newSoldierStateChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":NEW:VEHICLE:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newVehicleChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":NEW:VEHICLE:STATE:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newVehicleStateChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":FIRED:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newFiredEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":EVENT:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newGeneralEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":HIT:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newHitEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":KILL:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newKillEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":CHAT:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newChatEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":RADIO:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newRadioEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	case ":FPS:":
		go func(data []string, timestamp string) {
			data = append(data, timestamp)
			newFpsEventChan <- data
		}(out, fmt.Sprintf("%d", timestamp.UnixNano()))
	default:
		temp = fmt.Sprintf(`["%s"]`, "Unknown Function")
	}

	// Return a result to Arma
	result := C.CString(temp)
	defer C.free(unsafe.Pointer(result))
	var size = C.strlen(result) + 1
	if size > outputsize {
		size = outputsize
	}

	C.memmove(unsafe.Pointer(output), unsafe.Pointer(result), size)
}

func callBackExample() {
	name := C.CString("arma")
	defer C.free(unsafe.Pointer(name))
	function := C.CString("funcToExecute")
	defer C.free(unsafe.Pointer(function))
	// Make a callback to Arma
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Second)
		param := C.CString(fmt.Sprintf("Loop: %d", i))
		defer C.free(unsafe.Pointer(param))
		runExtensionCallback(name, function, param)
	}
}

func getTimestamp() string {
	// get the current unix timestamp in nanoseconds
	// return time.Now().Local().Unix()
	return time.Now().Format("2006-01-02 15:04:05")
}

func trimQuotes(s string) string {
	// trim the start and end quotes from a string
	return strings.Trim(s, `"`)
}

func fixEscapeQuotes(s string) string {
	// fix the escape quotes in a string
	return strings.Replace(s, `""`, `"`, -1)
}

func writeLog(functionName string, data string, level string) {
	// get calling function & line
	_, file, line, _ := runtime.Caller(1)

	if ActiveSettings.Debug && level == "DEBUG" {
		log.Printf(`%s:%d %s [%s] %s`, path.Base(file), line, functionName, level, data)
	} else if level != "DEBUG" {
		log.Printf(`%s:%d %s [%s] %s`, path.Base(file), line, functionName, level, data)
	}

	if extensionCallbackFnc != nil {
		// replace double quotes with 2 double quotes
		escapedData := strings.Replace(data, `"`, `""`, -1)
		// do the same for single quotes
		escapedData = strings.Replace(escapedData, `'`, `'`, -1)
		// replace brackets w parentheses
		escapedData = strings.Replace(escapedData, `[`, `(`, -1)
		escapedData = strings.Replace(escapedData, `]`, `)`, -1)
		a3Message := fmt.Sprintf(`["%s", "%s"]`, escapedData, level)

		statusName := C.CString(EXTENSION)
		defer C.free(unsafe.Pointer(statusName))
		statusFunction := C.CString(functionName)
		defer C.free(unsafe.Pointer(statusFunction))
		statusParam := C.CString(a3Message)
		defer C.free(unsafe.Pointer(statusParam))
		runExtensionCallback(statusName, statusFunction, statusParam)
	}
}

//export goRVExtension
func goRVExtension(output *C.char, outputsize C.size_t, input *C.char) {

	var temp string

	// logLine("goRVExtension", fmt.Sprintf(`["Input: %s",  "DEBUG"]`, C.GoString(input)), true)

	switch C.GoString(input) {
	case "version":
		temp = EXTENSION_VERSION
	case "getDir":
		temp = getDir()

	default:
		temp = fmt.Sprintf(`["%s"]`, "Unknown Function")
	}

	result := C.CString(temp)
	defer C.free(unsafe.Pointer(result))
	var size = C.strlen(result) + 1
	if size > outputsize {
		size = outputsize
	}

	C.memmove(unsafe.Pointer(output), unsafe.Pointer(result), size)
	// return
}

//export goRVExtensionRegisterCallback
func goRVExtensionRegisterCallback(fnc C.extensionCallback) {
	extensionCallbackFnc = fnc
}

func populateDemoData() {
	if !DB_VALID {
		return
	}

	// declare test size counts
	var (
		numMissions              int = 1
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
		// ["missionName", missionName],
		// ["briefingName", briefingName],
		// ["missionNameSource", missionNameSource],
		// ["onLoadName", getMissionConfigValue ["onLoadName", ""]],
		// ["author", getMissionConfigValue ["author", ""]],
		// ["serverName", serverName],
		// ["serverProfile", profileName],
		// ["missionStart", "0"],
		// ["worldName", toLower worldName],
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
		}
		missionDataJSON, err := json.Marshal(missionData)
		if err != nil {
			fmt.Println(err)
		}
		data[1] = string(missionDataJSON)

		err = logNewMission(data)
		if err != nil {
			fmt.Println(err)
		}

		time.Sleep(500 * time.Millisecond)
	}
	missionsEnd := time.Now()
	missionsElapsed := missionsEnd.Sub(missionsStart)
	fmt.Printf("Sent %d missions in %s\n", numMissions, missionsElapsed)

	// for each mission
	missions := []defs.Mission{}
	DB.Model(&defs.Mission{}).Find(&missions)

	waitGroup = sync.WaitGroup{}
	for _, mission := range missions {
		CurrentMission = mission

		fmt.Printf("Populating mission with ID %d\n", mission.ID)

		// write soldiers, now that our missions exist and the channels have been created
		idCounter := 1

		for i := 0; i <= numSoldiers; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				// these will be sent as an array
				// frame := strconv.FormatInt(int64(rand.Intn(missionDuration)), 10)
				soldierId := strconv.FormatInt(int64(thisId), 10)
				soldier := []string{
					soldierId,                                          // join frame
					soldierId,                                          // ocapid
					fmt.Sprintf("Demo Unit %d", i),                     // unit name
					fmt.Sprintf("Demo Group %d", i),                    // group id
					sides[rand.Intn(len(sides))],                       // side
					strconv.FormatBool(rand.Intn(2) == 1),              // isplayer
					roleDescriptions[rand.Intn(len(roleDescriptions))], // roleDescription
					"B_Soldier_F",                                      // unit classname
					"Rifleman",                                         // unit display name
					strconv.FormatInt(int64(rand.Intn(1000000000)), 10), // random player uid
				}

				// add timestamp to end of array
				soldier = append(soldier, fmt.Sprintf("%d", time.Now().UnixNano()))
				newSoldierChan <- soldier

				// sleep to ensure soldier is written
				time.Sleep(3000 * time.Millisecond)

				// write soldier states
				var randomPos [3]float64 = [3]float64{rand.Float64() * 30720, rand.Float64() * 30720, rand.Float64() * 30720}
				var randomDir float64 = rand.Float64() * 360
				var dirMoveOffset float64 = rand.Float64() * 360
				var randomLifestate int = rand.Intn(3)
				var currentRole string = roles[rand.Intn(len(roles))]
				for i := 0; i <= missionDuration; i++ {
					// sleep 500 ms to make sure there's enough time between processing so time doesn't match exactly
					time.Sleep(50 * time.Millisecond)

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
						soldierId,
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
					}

					// add timestamp to end of array
					soldierState = append(soldierState, fmt.Sprintf("%d", time.Now().UnixNano()))
					newSoldierStateChan <- soldierState
				}
				waitGroup.Done()
			}(idCounter)
			idCounter++
		}

		// write vehicles
		for i := 0; i <= numVehicles; i++ {
			waitGroup.Add(1)
			go func(thisId int) {
				vehicleId := strconv.FormatInt(int64(thisId), 10)
				vehicle := []string{
					// join frame
					vehicleId,
					// ocapid
					vehicleId,
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
				newVehicleChan <- vehicle

				// sleep to ensure vehicle is written
				time.Sleep(3000 * time.Millisecond)

				// send vehicle states
				for i := 0; i <= missionDuration; i++ {
					// sleep 100 ms
					time.Sleep(100 * time.Millisecond)
					vehicleState := []string{
						// ocap id
						vehicleId,
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
					}

					// add timestamp to end of array
					vehicleState = append(vehicleState, fmt.Sprintf("%d", time.Now().UnixNano()))
					newVehicleStateChan <- vehicleState
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
			time.Sleep(100 * time.Millisecond)
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
				newFiredEventChan <- firedEvent
				wg2.Done()
			}()
		}
		wg2.Wait()
	}

	// pause until newFiredEventChan is empty
	for len(newFiredEventChan) > 0 {
		time.Sleep(1000 * time.Millisecond)
	}

}

func getOcapRecording(missionIds []string) (err error) {
	fmt.Println("Getting JSON for mission IDs: ", missionIds)

	queries := []string{}

	for _, missionId := range missionIds {

		// get missionIdInt
		missionIdInt, err := strconv.Atoi(missionId)
		if err != nil {
			return err
		}

		// var result string
		var txStart time.Time

		// get mission data
		txStart = time.Now()
		var mission defs.Mission
		ocapMission := make(map[string]interface{})
		err = DB.Model(&defs.Mission{}).Where("id = ?", missionIdInt).First(&mission).Error
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

				pos := geom.Point(state.Position)
				posArr := []float64{pos.Coords().X(), pos.Coords().Y(), pos.Z()}
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
				endPos := geom.Point(event.EndPosition)
				endPosArr := []float64{endPos.Coords().X(), endPos.Coords().Y(), endPos.Z()}
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

				pos := geom.Point(state.Position)
				posArr := []float64{pos.Coords().X(), pos.Coords().Y(), pos.Z()}
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
			// victimIDVehicle or victimIDSoldier, whichever is not empty
			victimId := uint(0)

			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", hitEvent.VictimIDSoldier).Pluck(
				"ocap_id", &victimId,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
				jsonEvent[2] = victimId
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", hitEvent.VictimIDVehicle).Pluck(
					"ocap_id", &victimId,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
					jsonEvent[2] = victimId
				} else {
					// if neither found, skip this event
					continue
				}
			}

			// causedby info
			causedBy := make([]interface{}, 2)
			causedById := uint(0)
			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", hitEvent.ShooterIDSoldier).Pluck(
				"ocap_id", &causedById,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", hitEvent.ShooterIDVehicle).Pluck(
					"ocap_id", &causedById,
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
			causedBy[0] = causedById
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
			victimId := uint(0)

			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", killEvent.VictimIDSoldier).Pluck(
				"ocap_id", &victimId,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
				jsonEvent[2] = victimId
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", killEvent.VictimIDVehicle).Pluck(
					"ocap_id", &victimId,
				).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return err
				} else if err == nil {
					// if vehicle ocap_id found, use it
					jsonEvent[2] = victimId
				} else {
					// if neither found, skip this event
					continue
				}
			}

			// causedby info
			causedBy := make([]interface{}, 2)
			var causedById uint16
			// get soldier ocap_id
			err = DB.Model(&defs.Soldier{}).Where("id = ?", killEvent.KillerIDSoldier).Pluck(
				"ocap_id", &causedById,
			).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			} else if err == nil {
				// if soldier ocap_id found, use it
			} else {
				// get vehicle ocap_id
				err = DB.Model(&defs.Vehicle{}).Where("id = ?", killEvent.KillerIDVehicle).Pluck(
					"ocap_id", &causedById,
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
			causedBy[0] = causedById
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
		// missionJSON, err := json.Marshal(ocapMission)
		missionJSON, err := json.MarshalIndent(ocapMission, "", "  ")
		if err != nil {
			return err
		}

		// write to gzipped json file
		filename := fmt.Sprintf("mission_%s_%s.json.gz", missionId, ocapMission["missionName"])
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

		fmt.Printf("Wrote %d bytes for missionId %s in %s\n", bytes, missionId, time.Since(txStart))

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

func reduceMission(missionIds []string) (err error) {
	fmt.Println("Reducing mission IDs: ", missionIds)

	for _, missionId := range missionIds {

		// get missionIdInt
		missionIdInt, err := strconv.Atoi(missionId)
		if err != nil {
			return err
		}

		// get mission data
		txStart := time.Now()
		var mission defs.Mission
		err = DB.Model(&defs.Mission{}).Where("id = ?", missionIdInt).First(&mission).Error
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
			fmt.Println("No soldier states to delete for missionId ", missionId, ", checked in ", time.Since(txStart))
			continue
		}

		err = DB.Delete(&soldierStatesToDelete).Error
		if err != nil {
			return fmt.Errorf("error deleting soldier states: %w", err)
		}

		fmt.Println("Deleted ", len(soldierStatesToDelete), " soldier states from missionId ", missionId, " in ", time.Since(txStart))
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
	err = ioutil.WriteFile("test.json", jsonBytes, 0644)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	return nil
}

func main() {
	fmt.Println("Running DB connect/migrate to build schema...")
	err := getDB()
	if err != nil {
		panic(err)
	}
	fmt.Println("DB connect/migrate complete.")

	// get arguments
	args := os.Args[1:]
	if len(args) > 0 {
		if strings.ToLower(args[0]) == "demo" {
			fmt.Println("Populating demo data...")
			TEST_DATA = true
			demoStart := time.Now()
			populateDemoData()
			fmt.Printf("Demo data populated in %s\n", time.Since(demoStart))
			// wait input
			fmt.Println("Demo data populated. Press enter to exit.")
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
