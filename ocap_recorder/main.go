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

	"github.com/glebarez/sqlite"
	"github.com/twpayne/go-geom"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var EXTENSION_VERSION string = "0.0.1"
var extensionCallbackFnc C.extensionCallback

var ADDON string = "OCAP"
var EXTENSION string = "ocap_recorder"

// file paths
var ADDON_FOLDER string = getDir() + "\\@" + ADDON
var LOG_FILE string = ADDON_FOLDER + "\\" + EXTENSION + ".log"
var CONFIG_FILE string = ADDON_FOLDER + "\\config.json"
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
	newSoldierStateChan chan []string = make(chan []string, 20000)
	newVehicleStateChan chan []string = make(chan []string, 20000)
	newFiredEventChan   chan []string = make(chan []string, 30000)
	newGeneralEventChan chan []string = make(chan []string, 1000)
	newHitEventChan     chan []string = make(chan []string, 2000)
	newKillEventChan    chan []string = make(chan []string, 2000)

	// caches of processed models pending DB write
	soldiersToWrite      = defs.SoldiersQueue{}
	soldierStatesToWrite = defs.SoldierStatesQueue{}
	vehiclesToWrite      = defs.VehiclesQueue{}
	vehicleStatesToWrite = defs.VehicleStatesQueue{}
	firedEventsToWrite   = defs.FiredEventsQueue{}
	generalEventsToWrite = defs.GeneralEventsQueue{}
	hitEventsToWrite     = defs.HitEventsQueue{}
	killEventsToWrite    = defs.KillEventsQueue{}

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

var activeSettings ConfigJson = ConfigJson{}

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

	LOG_FILE = fmt.Sprintf(`%s\%s.log`, activeSettings.LogsDir, EXTENSION)
	log.Println("Log location specified in config as", LOG_FILE)
	LOCAL_DB_FILE = fmt.Sprintf(`%s\%s_%s.db`, ADDON_FOLDER, EXTENSION, SESSION_START_TIME.Format("20060102_150405"))

	// resolve path set in activeSettings.LogsDir
	// create logs dir if it doesn't exist
	if _, err := os.Stat(activeSettings.LogsDir); os.IsNotExist(err) {
		os.Mkdir(activeSettings.LogsDir, 0755)
	}
	// log to file
	f, err = os.OpenFile(LOG_FILE, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
}

func version() {
	functionName := "version"
	writeLog(functionName, fmt.Sprintf(`ocap_recorder version: %s`, EXTENSION_VERSION), "INFO")
}

func getDir() string {
	dir, err := os.Getwd()
	if err != nil {
		writeLog("getDir", fmt.Sprintf(`Error getting working directory: %v`, err), "ERROR")
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
	err = decoder.Decode(&activeSettings)
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
	_, err = http.Get(activeSettings.APIConfig.ServerURL + "/healthcheck")
	if err != nil {
		writeLog(functionName, "OCAP Frontend is offline", "WARN")
	} else {
		writeLog(functionName, "OCAP Frontend is online", "INFO")
	}
}

func getProgramStatus(
	rawBuffers bool,
	writeQueues bool,
	lastWrite bool,
) (output []string) {
	// returns a slice of strings containing the current program status
	// rawBuffers: include raw buffers in output
	rawBuffersStr := fmt.Sprintf("TO PROCESS: Soldiers: %d | Vehicles: %d | SoldierStates: %d, , VehicleStates: %d | FiredEvents: %d",
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

	return output
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
		activeSettings.DBConfig.Username,
		activeSettings.DBConfig.Password,
		activeSettings.DBConfig.Host,
		activeSettings.DBConfig.Port,
		activeSettings.DBConfig.Database,
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
	functionName := "getDB"

	// connect to database (Postgres) using gorm
	dsn := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable`,
		activeSettings.DBConfig.Host,
		activeSettings.DBConfig.Port,
		activeSettings.DBConfig.Username,
		activeSettings.DBConfig.Password,
		activeSettings.DBConfig.Database,
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
		writeLog(functionName, fmt.Sprintf(`Failed to set up TimescaleDB. Err: %s`, err), "ERROR")
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
		writeLog(functionName, fmt.Sprintf(`Failed to connect to TimescaleDB. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		err = getLocalDB()
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to connect to SQLite. Err: %s`, err), "ERROR")
			DB_VALID = false
		} else {
			DB_VALID = true
		}
	} else {
		writeLog(functionName, "Connected to TimescaleDB", "INFO")
		SAVE_LOCAL = false
		DB_VALID = true
	}
	connectionValid := err == nil
	if !connectionValid {
		writeLog(functionName, fmt.Sprintf(`Failed to connect to TimescaleDB. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		err = getLocalDB()
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to connect to SQLite. Err: %s`, err), "ERROR")
			DB_VALID = false
		} else {
			DB_VALID = true
		}
	} else {
		writeLog(functionName, "Connected to TimescaleDB", "INFO")
		SAVE_LOCAL = false
		DB_VALID = true
	}

	if !DB_VALID {
		writeLog(functionName, "DB not valid. Not saving!", "ERROR")
		return errors.New("DB not valid. Not saving")
	}

	if !SAVE_LOCAL {
		// Ensure PostGIS and TimescaleDB extensions are installed
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
		err = DB.Exec(`
		CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;
		`).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create TimescaleDB extension. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		} else {
			writeLog(functionName, "TimescaleDB extension created", "INFO")
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
	toMigrate = append(toMigrate, &defs.Vehicle{})

	conditionalMigrate := map[string]interface{}{
		"soldier_states": &defs.SoldierState{},
		"vehicle_states": &defs.VehicleState{},
		"fired_events":   &defs.FiredEvent{},
		"general_events": &defs.GeneralEvent{},
		"hit_events":     &defs.HitEvent{},
		"kill_events":    &defs.KillEvent{},
	}
	var existingHypertablesNames []string
	if !SAVE_LOCAL {
		// check first to see what hypertables exist that have compression, because we will get an error

		err = DB.Table("timescaledb_information.hypertables").Select(
			"hypertable_name",
		).Where("compression_enabled = TRUE").Scan(&existingHypertablesNames).Error

		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to get existing hypertables. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}

		for k, v := range conditionalMigrate {
			if !contains(existingHypertablesNames, k) {
				toMigrate = append(toMigrate, v)
			}
		}
	} else {
		toMigrate = append(toMigrate, &defs.SoldierState{})
		toMigrate = append(toMigrate, &defs.VehicleState{})
		toMigrate = append(toMigrate, &defs.FiredEvent{})
	}

	// fmt.Printf("toMigrate: %s\n", toMigrate)

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

		// if running TimescaleDB, make sure that these tables, which are time-oriented and we want to maximize time based input, are partitioned in order for fast retrieval and compressed after 2 weeks to save disk space
		// https://docs.timescale.com/latest/using-timescaledb/hypertables

		hyperTables := map[string][]string{
			"soldier_states": {
				"time",
				"mission_id",
				"soldier_id",
				"capture_frame",
			},
			"vehicle_states": {
				"time",
				"mission_id",
				"vehicle_id",
				"capture_frame",
			},
			"fired_events": {
				"time",
				"mission_id",
				"soldier_id",
				"capture_frame",
			},
		}
		for k := range conditionalMigrate {
			if contains(existingHypertablesNames, k) {
				delete(hyperTables, k)
			}
		}
		// err = validateHypertables(hyperTables)
		if err != nil {
			writeLog(functionName, `Failed to validate hypertables.`, "ERROR")
			DB_VALID = false
			return err
		}
	}

	writeLog(functionName, "DB initialized", "INFO")

	// init caches
	TEST_DATA_TIMEINC.Set(0)

	// CHANNEL LISTENER TO SAVE DATA
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
				tx         *gorm.DB  = DB.Begin()
				writeStart time.Time = time.Now()
			)

			// write new soldiers
			if !soldiersToWrite.Empty() {
				soldiersToWrite.Lock()
				err = tx.Create(&soldiersToWrite.Queue).Error
				soldiersToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldiers: %v`, err), "ERROR")
				}
				soldiersToWrite.Clear()
			}

			// write soldier states
			if !soldierStatesToWrite.Empty() {
				soldierStatesToWrite.Lock()
				err = tx.Create(&soldierStatesToWrite.Queue).Error
				soldierStatesToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating soldier states: %v`, err), "ERROR")
				}
				soldierStatesToWrite.Clear()
			}

			// write new vehicles
			if !vehiclesToWrite.Empty() {
				vehiclesToWrite.Lock()
				err = tx.Create(&vehiclesToWrite.Queue).Error
				vehiclesToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicles: %v`, err), "ERROR")
				}
				vehiclesToWrite.Clear()
			}

			// write vehicle states
			if !vehicleStatesToWrite.Empty() {
				vehicleStatesToWrite.Lock()
				err = tx.Create(&vehicleStatesToWrite.Queue).Error
				vehicleStatesToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating vehicle states: %v`, err), "ERROR")
				}
				vehicleStatesToWrite.Clear()
			}

			// write fired events
			if !firedEventsToWrite.Empty() {
				firedEventsToWrite.Lock()
				err = tx.Create(&firedEventsToWrite.Queue).Error
				firedEventsToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating fired events: %v`, err), "ERROR")
				}
				firedEventsToWrite.Clear()
			}

			// write general events
			if !generalEventsToWrite.Empty() {
				generalEventsToWrite.Lock()
				err = tx.Create(&generalEventsToWrite.Queue).Error
				generalEventsToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating general events: %v`, err), "ERROR")
				}
				generalEventsToWrite.Clear()
			}

			// write hit events
			if !hitEventsToWrite.Empty() {
				hitEventsToWrite.Lock()
				err = tx.Create(&hitEventsToWrite.Queue).Error
				hitEventsToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating hit events: %v`, err), "ERROR")
				}
				hitEventsToWrite.Clear()
			}

			// write kill events
			if !killEventsToWrite.Empty() {
				killEventsToWrite.Lock()
				err = tx.Create(&killEventsToWrite.Queue).Error
				killEventsToWrite.Unlock()
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error creating killed events: %v`, err), "ERROR")
				}
				killEventsToWrite.Clear()
			}

			// commit transaction
			err = tx.Commit().Error
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Error committing transaction: %v`, err), "ERROR")
				tx.Rollback()
			}
			LAST_WRITE_DURATION = time.Since(writeStart)

			// sleep
			time.Sleep(1000 * time.Millisecond)

		}
	}()

	// goroutine to, every 10 seconds, pause insert execution and dump memory sqlite db to disk
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

	world := defs.World{}
	mission := defs.Mission{}
	// unmarshal data[0]
	err = json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling world data: %v`, err), "ERROR")
		return err
	}

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
	if err = json.Unmarshal([]byte(data[1]), &mission); err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling mission data: %v`, err), "ERROR")
		return err
	}
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
		} else {
			// addon exists, append it
			addons = append(addons, thisAddon)
		}
	}
	mission.Addons = addons
	mission.StartTime = time.Now()
	captureInt, err := strconv.ParseFloat(missionTemp["captureDelay"].(string), 32)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting captureDelay to int: %v`, err), "ERROR")
		return err
	}
	mission.CaptureDelay = float32(captureInt)

	// check if world exists
	err = DB.Where("world_name = ?", world.WorldName).First(&world).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeLog(functionName, fmt.Sprintf(`Error checking if world exists: %v`, err), "ERROR")
		return err
	}
	if world.ID == 0 {
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
	soldier.JoinTime = time.Now()
	soldier.JoinFrame = uint(capframe)

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
	// player uid
	soldier.PlayerUID = data[7]

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
		writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return soldierState, err
	}
	soldierState.CaptureFrame = uint(capframe)

	// parse data in array
	ocapId, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return soldierState, err
	}

	// try and find soldier in DB to associate
	soldierId := uint(0)
	err = DB.Model(&defs.Soldier{}).Select("id").Order("join_time DESC").Where("ocap_id = ?", uint16(ocapId)).First(&soldierId).Error
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error finding soldier in DB:\n%s\n%v", json, err), "ERROR")
		return soldierState, err
	}
	soldierState.SoldierID = soldierId

	// random value within 5 seconds of now
	soldierState.Time = time.Now().Add(-time.Duration(rand.Intn(10)) * time.Second)

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

	// parse array
	vehicle.MissionID = CurrentMission.ID
	vehicle.JoinTime = time.Now()
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
	frameStr := data[len(data)-1]
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
	vehicleId := uint(0)
	err = DB.Model(&defs.Vehicle{}).Select("id").Order("join_time DESC").Where("ocap_id = ?", uint16(ocapId)).First(&vehicleId).Error
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error finding vehicle in DB:\n%s\n%v", json, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.VehicleID = vehicleId

	vehicleState.Time = time.Now().Add(-time.Duration(rand.Intn(10)) * time.Second)

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
	err = DB.Model(&defs.Soldier{}).Order("join_time DESC").Where("ocap_id = ?", uint16(ocapId)).Pluck("id", &soldierId).Error
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error finding soldier in DB:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.SoldierID = soldierId

	firedEvent.Time = time.Now()

	// parse BULLET START POS from an arma string
	startpos := data[2]
	startpos = strings.TrimPrefix(startpos, "[")
	startpos = strings.TrimSuffix(startpos, "]")
	startpoint, startelev, err := defs.GPSFromString(startpos, 3857, SAVE_LOCAL)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.StartPosition = startpoint
	firedEvent.StartElevationASL = float32(startelev)

	// parse BULLET END POS from an arma string
	endpos := data[3]
	endpos = strings.TrimPrefix(endpos, "[")
	endpos = strings.TrimSuffix(endpos, "]")
	endpoint, endelev, err := defs.GPSFromString(endpos, 3857, SAVE_LOCAL)
	if err != nil {
		json, _ := json.Marshal(data)
		writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", json, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.EndPosition = endpoint
	firedEvent.EndElevationASL = float32(endelev)

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

	thisEvent.Time = time.Now()
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
	hitEvent.Time = time.Now()

	// parse data in array
	victimOcapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	victimId := uint(0)
	// first, look in soldiers
	err = DB.Where(&defs.Soldier{
		OcapID:  uint16(victimOcapId),
		Mission: CurrentMission,
	}).Pluck("id", &victimId).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return hitEvent, fmt.Errorf(`error finding victim in db: %v`, err)
	}
	if victimId != 0 {
		hitEvent.VictimIDSoldier = victimId
	}

	// if not found, look in vehicles
	if victimId == 0 {
		err = DB.Where(&defs.Vehicle{
			OcapID:  uint16(victimOcapId),
			Mission: CurrentMission,
		}).Pluck("id", &victimId).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return hitEvent, fmt.Errorf(`error finding victim in db: %v`, err)
		}
		if victimId != 0 {
			hitEvent.VictimIDVehicle = victimId
		}
	}

	// if victim not found, log it
	if victimId == 0 {
		return hitEvent, fmt.Errorf("victim ocap id not found in db: %v", err)
	}

	// now look for the shooter
	shooterOcapId, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting shooter ocap id to uint: %v`, err)
	}

	// try and find shooter in DB to associate
	shooterId := uint(0)
	// first, look in soldiers
	err = DB.Where(&defs.Soldier{
		OcapID:  uint16(shooterOcapId),
		Mission: CurrentMission,
	}).Pluck("id", &shooterId).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return hitEvent, fmt.Errorf(`error finding shooter in db: %v`, err)
	}
	if shooterId != 0 {
		hitEvent.ShooterIDSoldier = shooterId
	}

	// if not found, look in vehicles
	if shooterId == 0 {
		err = DB.Where(&defs.Vehicle{
			OcapID:  uint16(shooterOcapId),
			Mission: CurrentMission,
		}).Pluck("id", &shooterId).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return hitEvent, fmt.Errorf(`error finding shooter in db: %v`, err)
		}
		if shooterId != 0 {
			hitEvent.ShooterIDVehicle = shooterId
		}
	}

	// if shooter not found, log it
	if shooterId == 0 {
		return hitEvent, fmt.Errorf("shooter ocap id not found in db: %v", err)
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

	killEvent.CaptureFrame = uint(capframe)
	killEvent.Mission = CurrentMission
	killEvent.Time = time.Now()

	// parse data in array
	victimOcapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// try and find victim in DB to associate
	victimId := uint(0)
	// first, look in soldiers
	err = DB.Where(&defs.Soldier{
		OcapID:  uint16(victimOcapId),
		Mission: CurrentMission,
	}).Pluck("id", &victimId).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return killEvent, fmt.Errorf(`error finding victim in db: %v`, err)
	}
	if victimId != 0 {
		killEvent.VictimIDSoldier = victimId
	}

	// if not found, look in vehicles
	if victimId == 0 {
		err = DB.Where(&defs.Vehicle{
			OcapID:  uint16(victimOcapId),
			Mission: CurrentMission,
		}).Pluck("id", &victimId).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return killEvent, fmt.Errorf(`error finding victim in db: %v`, err)
		}
		if victimId != 0 {
			killEvent.VictimIDVehicle = victimId
		}
	}

	// if victim not found, log it
	if victimId == 0 {
		return killEvent, fmt.Errorf("victim ocap id not found in db: %v", err)
	}

	// now look for the killer
	killerOcapId, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting killer ocap id to uint: %v`, err)
	}

	// try and find killer in DB to associate
	killerId := uint(0)
	// first, look in soldiers
	err = DB.Where(&defs.Soldier{
		OcapID:  uint16(killerOcapId),
		Mission: CurrentMission,
	}).Pluck("id", &killerId).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return killEvent, fmt.Errorf(`error finding killer in db: %v`, err)
	}
	if killerId != 0 {

		killEvent.KillerIDSoldier = killerId
	}

	// if not found, look in vehicles
	if killerId == 0 {
		err = DB.Where(&defs.Vehicle{
			OcapID:  uint16(killerOcapId),
			Mission: CurrentMission,
		}).Pluck("id", &killerId).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return killEvent, fmt.Errorf(`error finding killer in db: %v`, err)
		}
		if killerId != 0 {
			killEvent.KillerIDVehicle = killerId
		}
	}

	// if killer not found, log it
	if killerId == 0 {
		return killEvent, fmt.Errorf("killer ocap id not found in db: %v", err)
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
	temp := fmt.Sprintf("Function: %s nb params: %d", C.GoString(input), argc)

	switch C.GoString(input) {
	case "getDB":
		// callExtension ["logAttendance", [_hash] call CBA_fnc_encodeJSON]];
		err := getDB()
		if err != nil {
			temp = fmt.Sprintf(
				`[1, "Error getting DB: %s"]`,
				strings.Replace(err.Error(), `"`, `""`, -1),
			)
		} else {
			temp = `[0, "DB initialized"]`
		}
	case ":NEW:MISSION:":
		err := logNewMission(out)
		if err != nil {
			temp = fmt.Sprintf(
				`[1, "%s"]`,
				strings.Replace(err.Error(), `"`, `""`, -1),
			)
		} else {
			temp = `[0, "Mission logged"]`
		}
	case ":NEW:SOLDIER:":
		newSoldierChan <- out
		temp = `[0, "Logging unit"]`
	case ":NEW:SOLDIER:STATE:":
		newSoldierStateChan <- out
		temp = `[0, "Logging unit state"]`
	case ":NEW:VEHICLE:":
		newVehicleChan <- out
		temp = `[0, "Logging vehicle"]`
	case ":NEW:VEHICLE:STATE:":
		newVehicleStateChan <- out
		temp = `[0, "Logging vehicle state"]`
	case ":FIRED:":
		newFiredEventChan <- out
	case ":EVENT:":
		newGeneralEventChan <- out
	case ":HIT:":
		newHitEventChan <- out
	case ":KILL:":
		newKillEventChan <- out
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

	if activeSettings.Debug && level == "DEBUG" {
		log.Printf(`%s:%d:%s [%s] %s`, path.Base(file), line, functionName, level, data)
	} else if level != "DEBUG" {
		log.Printf(`%s:%d:%s [%s] %s`, path.Base(file), line, functionName, level, data)
	}

	if extensionCallbackFnc != nil {
		// replace double quotes with 2 double quotes
		escapedData := strings.Replace(data, `"`, `""`, -1)
		// do the same for single quotes
		escapedData = strings.Replace(escapedData, `'`, `'`, -1)
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

	// start a goroutine that will output channel lengths every second
	stopMonitorChan := make(chan bool)
	go func() {
		for {
			// if signal received on channel, break
			select {
			case <-stopMonitorChan:
				return
			default:
				time.Sleep(500 * time.Millisecond)
				for _, line := range getProgramStatus(true, true, true) {
					fmt.Println(line)
				}
			}
		}
	}()

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
			"onLoadName":                   "",
			"author":                       "Demo Author",
			"serverName":                   "Demo Server",
			"serverProfile":                "Demo Profile",
			"missionStart":                 nil, // random time
			"worldName":                    fmt.Sprintf("demo_world_%d", i),
			"tag":                          "Demo Tag",
			"captureDelay":                 "1.0",
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
					soldierId,                                           // join frame
					soldierId,                                           // ocapid
					fmt.Sprintf("Demo Unit %d", i),                      // unit name
					fmt.Sprintf("Demo Group %d", i),                     // group id
					sides[rand.Intn(len(sides))],                        // side
					strconv.FormatBool(rand.Intn(2) == 1),               // isplayer
					roleDescriptions[rand.Intn(len(roleDescriptions))],  // roleDescription
					strconv.FormatInt(int64(rand.Intn(1000000000)), 10), // random player uid
				}

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
					}

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
				newVehicleChan <- vehicle

				// sleep to ensure vehicle is written
				time.Sleep(3000 * time.Millisecond)

				// send vehicle states
				for i := 0; i <= missionDuration; i++ {
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
					}

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
					fmt.Sprintf("[%f,%f,%f]", randomStartPos[0], randomStartPos[1], randomStartPos[2]),
					// random end pos within 200m of start pos
					fmt.Sprintf("[%f,%f,%f]", randomEndPos[0], randomEndPos[1], randomEndPos[2]),
					// random weapon
					weapons[rand.Intn(len(weapons))],
					// random magazine
					magazines[rand.Intn(len(magazines))],
					// random firemode
					firemodes[rand.Intn(len(firemodes))],
				}

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

	stopMonitorChan <- true

	fmt.Println("Demo data populated. Press enter to exit.")
	fmt.Scanln()
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

		// get end frame by getting the last soldier state
		var endFrame int
		err = DB.Model(&defs.SoldierState{}).Where("mission_id = ?", missionIdInt).Order("capture_frame DESC").Limit(1).Pluck("capture_frame", &endFrame).Error
		if err != nil {
			return err
		}
		ocapMission["endFrame"] = endFrame

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
			jsonSoldier["positions"] = make([]interface{}, 0)
			jsonSoldier["framesFired"] = make([]interface{}, 0)

			// get soldier states
			var soldierStates []defs.SoldierState
			err = DB.Model(&dbSoldier).Order("capture_frame ASC").Association("SoldierStates").Find(&soldierStates)
			if err != nil {
				return err
			}
			// get "positions" arrays
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

				// add frame to positions
				jsonSoldier["positions"] = append(jsonSoldier["positions"].([]interface{}), thisPosition)
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
				thisPosition = append(thisPosition, make([]uint, 2))
				if len(lastState) > 0 && reflect.DeepEqual(thisPosition[:3], lastState[:3]) {
					// if the position is the same, we just increment the frame count of the last one
					lastState[4].([]uint)[1]++
					// and ensure the positions array is up to date
					jsonVehicle["positions"].([]interface{})[frameIndex].([]interface{})[4] = lastState[4]
					// and add the capture frame to the used frames map
					usedCaptureFrames[state.CaptureFrame] = true
				} else {
					if
					// we need to account for gaps in the capture frames, where the vehicle's last entry doesn't have [4][1] < captureFrame - 1
					// if the last state is not empty
					len(lastState) > 0 &&
						// and the last entry's end frame is less than the current state's capture frame - 1
						lastState[4].([]uint)[1] < state.CaptureFrame-1 {
						// then we need to add a new position to the positions array & also update the last state
						// first we need to update the last state's end frame
						lastState[4].([]uint)[1] = state.CaptureFrame - 1
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

		// now we need to get the mission object in json format
		txStart = time.Now()
		missionJSON, err := json.Marshal(ocapMission)
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
		if args[0] == "demo" {
			fmt.Println("Populating demo data...")
			TEST_DATA = true
			demoStart := time.Now()
			populateDemoData()
			fmt.Printf("Demo data populated in %s\n", time.Since(demoStart))
			// wait input
			fmt.Println("Demo data populated. Press enter to exit.")
		}
		if args[0] == "getjson" {
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
	} else {
		fmt.Println("No arguments provided.")
	}
	fmt.Scanln()
}
