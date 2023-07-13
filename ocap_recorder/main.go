package main

/*
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include "extensionCallback.h"
*/
import "C" // This is required to import the C code

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/glebarez/sqlite"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkbhex"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

var (
	// cache soldiers by ocap id
	soldiersCache map[uint16]uint = make(map[uint16]uint)
	// channels for receiving new data and filing to DB
	newSoldierChan   chan []string
	soldierStateChan chan []string
	newVehicleChan   chan []string
)

// queue struct
type queue struct {
	items []interface{}
}

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
	LOCAL_DB_FILE = fmt.Sprintf(`%s\%s.db`, ADDON_FOLDER, EXTENSION)

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

// /////////////////////
// DATABASE STRUCTURES //
// /////////////////////

// key value store for OCAP Group info, later use in frontend?
type OcapInfo struct {
	gorm.Model
	GroupName        string `json:"groupName" gorm:"size:127"` // primary key
	GroupDescription string `json:"groupDescription" gorm:"size:255"`
	GroupWebsite     string `json:"groupURL" gorm:"size:255"`
	GroupLogo        string `json:"groupLogoURL" gorm:"size:255"`
}

// setup status of the database
type DatabaseStatus struct {
	gorm.Model
	SetupTime             time.Time `json:"setupTime"`
	TablesMigrated        bool      `json:"tablesMigrated"`
	TablesMigratedTime    time.Time `json:"tablesMigratedTime"`
	HyperTablesConfigured bool      `json:"hyperTablesConfigured"`
}

type World struct {
	gorm.Model
	Author            string  `json:"author" gorm:"size:64"`
	WorkshopID        string  `json:"workshopID" gorm:"size:64"`
	DisplayName       string  `json:"displayName" gorm:"size:127"`
	WorldName         string  `json:"worldName" gorm:"size:127"`
	WorldNameOriginal string  `json:"worldNameOriginal" gorm:"size:127"`
	WorldSize         float32 `json:"worldSize"`
	Latitude          float32 `json:"latitude"`
	Longitude         float32 `json:"longitude"`
	Missions          []Mission
}

type Mission struct {
	gorm.Model
	MissionName       string    `json:"missionName" gorm:"size:200"`
	BriefingName      string    `json:"briefingName" gorm:"size:200"`
	MissionNameSource string    `json:"missionNameSource" gorm:"size:200"`
	OnLoadName        string    `json:"onLoadName" gorm:"size:200"`
	Author            string    `json:"author" gorm:"size:200"`
	ServerName        string    `json:"serverName" gorm:"size:200"`
	ServerProfile     string    `json:"serverProfile" gorm:"size:200"`
	StartTime         time.Time `json:"missionStart" gorm:"type:timestamptz;index:idx_mission_start"` // time.Time
	WorldName         string    `json:"worldName" gorm:"-"`
	WorldID           uint
	World             World     `gorm:"foreignkey:WorldID"`
	Tag               string    `json:"tag" gorm:"size:127"`
	Soldiers          []Soldier `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Vehicles               []Vehicle               `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerConnects    []EventPlayerConnect    `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerDisconnects []EventPlayerDisconnect `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Soldier struct {
	gorm.Model
	Mission         Mission   `gorm:"foreignkey:MissionID"`
	MissionID       uint      `json:"missionId"`
	JoinTime        time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;"`
	JoinFrame       uint      `json:"joinFrame"`
	OcapID          uint16    `json:"ocapId" gorm:"uniqueIndex:idx_ocap_id"`
	UnitName        string    `json:"unitName" gorm:"size:64"`
	GroupID         string    `json:"groupId" gorm:"size:64"`
	Side            string    `json:"side" gorm:"size:16"`
	IsPlayer        bool      `json:"isPlayer" gorm:"default:false"`
	RoleDescription string    `json:"roleDescription" gorm:"size:64"`
}

// SoldierState inherits from Frame
type SoldierState struct {
	// composite primary key with ID and OCAPID
	Time         time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	SoldierID    uint      `json:"soldierId"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint      `json:"captureFrame"`

	Position     GPSCoordinates `json:"position" gorm:"type:point"`
	ElevationASL float32        `json:"elevationASL"`
	Bearing      uint16         `json:"bearing" gorm:"default:0"`
	Lifestate    uint8          `json:"lifestate" gorm:"default:0"`
	InVehicle    bool           `json:"inVehicle" gorm:"default:false"`
	UnitName     string         `json:"unitName" gorm:"size:64"`
	IsPlayer     bool           `json:"isPlayer" gorm:"default:false"`
	CurrentRole  string         `json:"currentRole" gorm:"size:64"`
}

type Vehicle struct {
	VehicleClass string         `json:"vehicleClass" gorm:"size:64"`
	DisplayName  string         `json:"displayName" gorm:"size:64"`
	Position     GPSCoordinates `json:"position" gorm:"type:point"`
	Bearing      uint16         `json:"bearing"`
	IsAlive      bool           `json:"isAlive"`
	Crew         string         `json:"crew" gorm:"size:255"`
}

// event types
type EventPlayerConnect struct {
	gorm.Model
	Mission      Mission `gorm:"foreignkey:MissionID"`
	MissionID    uint
	CaptureFrame uint32 `json:"captureFrame"`
	EventType    string `json:"eventType" gorm:"size:32"`
	PlayerUID    string `json:"playerUid" gorm:"index:idx_player_uid;size:20"`
	ProfileName  string `json:"playerName" gorm:"size:32"`
}

type EventPlayerDisconnect struct {
	gorm.Model
	Mission      Mission `gorm:"foreignkey:MissionID"`
	MissionID    uint
	CaptureFrame uint32 `json:"captureFrame"`
	EventType    string `json:"eventType" gorm:"size:32"`
	PlayerUID    string `json:"playerUid" gorm:"index:idx_player_uid;size:20"`
	ProfileName  string `json:"playerName" gorm:"size:32"`
}

// GEO POINTS
// https://pkg.go.dev/github.com/StampWallet/backend/internal/database
type GPSCoordinates geom.Point

var ErrInvalidCoordinates = errors.New("invalid coordinates")

func (g *GPSCoordinates) Scan(input interface{}) error {
	gt, err := ewkbhex.Decode(input.(string))
	if err != nil {
		return err
	}
	gp := gt.(*geom.Point)
	gc := GPSCoordinates(*gp)
	*g = gc
	return nil
}

func (g *GPSCoordinates) ToString() string {
	return strconv.FormatFloat(g.Coords().X(), 'f', -1, 64) +
		"," +
		strconv.FormatFloat(g.Coords().Y(), 'f', -1, 64)
}

func GPSCoordinatesFromString(coords string) (GPSCoordinates, error) {
	sp := strings.Split(coords, ",")
	if len(sp) != 2 {
		return GPSCoordinates{}, ErrInvalidCoordinates
	}
	long, err := strconv.ParseFloat(sp[0], 64)
	if err != nil {
		return GPSCoordinates{}, ErrInvalidCoordinates
	}
	lat, err := strconv.ParseFloat(sp[1], 64)
	if err != nil {
		return GPSCoordinates{}, ErrInvalidCoordinates
	}
	return GPSCoordinates(*geom.NewPointFlat(geom.XY, []float64{long, lat})), nil
}

func (g GPSCoordinates) GormDataType() string {
	return "geometry"
}

func (g GPSCoordinates) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	b := geom.Point(g)
	srid := b.SRID()
	var vars []interface{} = []interface{}{fmt.Sprintf(`SRID=%d;POINT(0 0)"`, srid)}
	if !b.Empty() {
		vars = []interface{}{fmt.Sprintf("SRID=%d;POINT(%f %f)", srid, b.X(), b.Y())}
	}
	return clause.Expr{
		SQL:  "ST_PointFromText(?)",
		Vars: vars,
	}
}

func GPSFromCoords(longitude float64, latitude float64, srid int) GPSCoordinates {
	if srid == 0 {
		srid = 4326
	}
	return GPSCoordinates(*geom.NewPointFlat(geom.XY, geom.Coord{longitude, latitude}).SetSRID(srid))
}

///////////////////////
// DATABASE OPS //
///////////////////////

func getLocalDB() (err error) {
	functionName := "getDB"
	// connect to database (SQLite)
	DB, err = gorm.Open(sqlite.Open(LOCAL_DB_FILE), &gorm.Config{
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
		DB_VALID = true
		return nil
	}
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
		DSN: dsn,
		// PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		CreateBatchSize:        2000,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	configValid := err == nil
	if !configValid {
		writeLog(functionName, fmt.Sprintf(`Failed to set up TimescaleDB. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		getLocalDB()
	}
	// test connection
	err = DB.Exec(`SELECT 1`).Error
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

	databaseStatus := DatabaseStatus{}
	// Check if DatabaseStatus table exists
	if !DB.Migrator().HasTable(&DatabaseStatus{}) {
		// Create the table
		err = DB.AutoMigrate(&DatabaseStatus{})
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create database_status table. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
		// Create the first entry
		databaseStatus.SetupTime = time.Now()
		databaseStatus.TablesMigrated = false
		databaseStatus.TablesMigratedTime = time.Unix(0, 0)
		databaseStatus.HyperTablesConfigured = false
		err = DB.Create(&databaseStatus).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create database_status entry. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
	} else {
		// Get the first entry
		err = DB.First(&databaseStatus).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to get database_status entry. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
	}

	// Check if OcapInfo table exists
	if !DB.Migrator().HasTable(&OcapInfo{}) {
		// Create the table
		err = DB.AutoMigrate(&OcapInfo{})
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create ocap_info table. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
		// Create the default settings
		err = DB.Create(&OcapInfo{
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

	if !databaseStatus.TablesMigrated {
		// Migrate the schema
		err = DB.AutoMigrate(
			&World{},
			&Mission{},
			&Soldier{},
			&SoldierState{},
		)
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to migrate DB schema. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}

		// record migration
		databaseStatus.TablesMigrated = true
		databaseStatus.TablesMigratedTime = time.Now()
		err = DB.Save(&databaseStatus).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to update database_status entry. Err: %s`, err), "ERROR")
			DB_VALID = false
			return err
		}
	}

	if !SAVE_LOCAL {
		// if running TimescaleDB, make sure that these tables, which are time-oriented and we want to maximize time based input, are partitioned in order for fast retrieval and compressed after 2 weeks to save disk space
		// https://docs.timescale.com/latest/using-timescaledb/hypertables

		// see if config is already set
		if !databaseStatus.HyperTablesConfigured {
			// if not, set it
			hyperTables := map[string][]string{
				"soldier_states": {
					"time",
					"soldier_id",
					"mission_id",
				},
			}
			err = validateHypertables(hyperTables)
			if err != nil {
				writeLog(functionName, `Failed to validate hypertables.`, "ERROR")
				DB_VALID = false
				return err
			}
			databaseStatus.HyperTablesConfigured = true
			err = DB.Save(&databaseStatus).Error
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Failed to update database_status entry. Err: %s`, err), "ERROR")
				DB_VALID = false
				return err
			}
		}
	}

	writeLog(functionName, "DB initialized", "INFO")
	return nil
}

func validateHypertables(tables map[string][]string) (err error) {
	functionName := "validateHypertables"
	// HYPERTABLES

	// iterate through each provided table
	for table := range tables {

		// if table doesn't exist, create it
		queryCreateHypertable := fmt.Sprintf(`
				SELECT create_hypertable('%s', 'time', migrate_data => true, chunk_time_interval => interval '1 day', if_not_exists => true);
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
					timescaledb.compress_segmentby = 'soldier_id');
			`, table)
		err = DB.Exec(queryCompressHypertable).Error
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
var CurrentWorld World
var CurrentMission Mission

// logNewMission logs a new mission to the database and associates the world it's played on
func logNewMission(data []string) (err error) {
	functionName := ":NEW:MISSION:"

	world := World{}
	mission := Mission{}
	// unmarshal data[0]
	err = json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling world data: %v`, err), "ERROR")
		return err
	}

	// unmarshal data[1]
	err = json.Unmarshal([]byte(data[1]), &mission)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling mission data: %v`, err), "ERROR")
		return err
	}

	mission.StartTime = time.Now()

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

	// set up channels
	if newSoldierChan == nil {
		newSoldierChan = make(chan []string, 2500)
	}
	if soldierStateChan == nil {
		soldierStateChan = make(chan []string, 2500)
	}

	go func() {
		for {
			if !DB_VALID {
				return
			}
			select {
			case data := <-newSoldierChan:
				// workaround for appending cap frame in sqf rather than using it as first element
				frameStr := data[len(data)-1]
				data[len(data)-1] = ""    // Erase element (write zero value)
				data = data[:len(data)-1] // [A B]

				capframe, err := strconv.ParseUint(frameStr, 10, 32)
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
					continue
				}

				// fix received data
				for i, v := range data {
					data[i] = fixEscapeQuotes(trimQuotes(v))
				}
				logNewSoldier(uint(capframe), data)
			case data := <-soldierStateChan:
				frameStr := data[len(data)-1]
				data[len(data)-1] = ""    // Erase element (write zero value)
				data = data[:len(data)-1] // [A B]

				capframe, err := strconv.ParseUint(frameStr, 10, 32)
				if err != nil {
					writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
					continue
				}

				// fix received data
				for i, v := range data {
					data[i] = fixEscapeQuotes(trimQuotes(v))
				}
				logSoldierState(uint(capframe), data[1:])
			}
		}
	}()
	return nil
}

// (UNITS) AND VEHICLES

// logNewSoldier logs a new soldier to the database
func logNewSoldier(capFrame uint, data []string) {
	functionName := ":NEW:SOLDIER:"
	// check if DB is valid
	if !DB_VALID {
		return
	}

	var err error
	var soldier Soldier

	// parse array
	soldier.Mission = CurrentMission
	soldier.JoinTime = time.Now()
	soldier.JoinFrame = capFrame
	ocapId, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return
	}
	soldier.OcapID = uint16(ocapId)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, err = strconv.ParseBool(data[5])
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
		return
	}
	soldier.RoleDescription = data[6]

	// log to database
	err = DB.Create(&soldier).Error
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error creating soldier: %v -- %v`, soldier, err), "ERROR")
		return
	}
	soldiersCache[soldier.OcapID] = soldier.ID
}

// logSoldierState logs a SoldierState state to the database
func logSoldierState(capframe uint, data []string) {
	functionName := ":NEW:SOLDIER:STATE:"
	// check if DB is valid
	if !DB_VALID {
		return
	}

	var SoldierState SoldierState

	// parse data in struct - convert strings when necessary
	ocapId, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return
	}

	// try and find soldier in cache to associate
	if _, ok := soldiersCache[uint16(ocapId)]; !ok {
		SoldierState.SoldierID = soldiersCache[uint16(ocapId)]
	} else {
		writeLog(functionName, fmt.Sprintf(`Error finding soldier in cache, failed to write initial data: %d -- %v`, ocapId, err), "ERROR")
		return
	}
	SoldierState.CaptureFrame = uint(capframe)
	SoldierState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	posArr := strings.Split(pos, ",")
	coordX, _ := strconv.ParseFloat(posArr[0], 64)
	coordY, _ := strconv.ParseFloat(posArr[1], 64)
	coordZ, _ := strconv.ParseFloat(posArr[2], 64)
	SoldierState.Position = GPSFromCoords(coordX, coordY, 3857)
	SoldierState.ElevationASL = float32(coordZ)

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	SoldierState.Bearing = uint16(bearing)
	// lifestate
	lifeState, _ := strconv.Atoi(data[3])
	SoldierState.Lifestate = uint8(lifeState)
	// in vehicle
	SoldierState.InVehicle, _ = strconv.ParseBool(data[4])
	// name
	SoldierState.UnitName = data[5]
	// is player
	isPlayer, err := strconv.ParseBool(data[6])
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
		return
	}
	SoldierState.IsPlayer = isPlayer
	// current role
	SoldierState.CurrentRole = data[7]

	// write
	err = DB.Create(&SoldierState).Error
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error creating soldier state: %v -- %v`, SoldierState, err), "ERROR")
		return
	}
}

func logVehicle(data []string) {
	if !DB_VALID {
		return
	}

	// var vehicle Vehicle

	// // parse data in struct - convert strings when necessary
	// vehicle.Mission = CurrentMission
	// capframe, _ := strconv.Atoi(data[0])
	// vehicle.CaptureFrame = uint(capframe)
	// vehicleId, _ := strconv.ParseUint(data[1], 10, 64)
	// vehicle.OcapID = uint16(vehicleId)
	// vehicle.VehicleClass = data[2]
	// vehicle.DisplayName = data[3]
	// // coordX, _ := strconv.ParseFloat(data[4], 64)
	// // coordY, _ := strconv.ParseFloat(data[5], 64)
	// // coordZ, _ := strconv.ParseFloat(data[6], 64)
	// // vehicle.Position = geom.NewPoint(geom.XYZ).MustSetCoords([]float64{coordX, coordY, coordZ}).SetSRID(3857)
	// bearing, _ := strconv.Atoi(data[7])
	// vehicle.Bearing = uint16(bearing)
	// vehicle.Crew = data[8]

	// // write
	// DB.Create(&vehicle)
}

// function to process events of different kinds
func processEvent(data []string) {
	event := data[1]
	switch event {
	case "connected":
		object := EventPlayerConnect{}
		captureFrame, _ := strconv.Atoi(data[0])
		object.CaptureFrame = uint32(captureFrame)
		object.ProfileName = data[2]
		object.PlayerUID = data[3]
		object.MissionID = CurrentMission.ID

		// write
		DB.Create(&object)
	case "disconnected":
		object := EventPlayerDisconnect{}
		captureFrame, _ := strconv.Atoi(data[0])
		object.CaptureFrame = uint32(captureFrame)
		object.ProfileName = data[2]
		object.PlayerUID = data[3]
		object.MissionID = CurrentMission.ID

		// write
		DB.Create(&object)
	case "vehicle":
		logVehicle(data)
	default:
		writeLog("processEvent", fmt.Sprintf(`Unknown event type: %s`, event), "ERROR")
	}
}

// function to write queued models to database in a transaction
// func writemodelQueue(mut *sync.Mutex) {
// 	functionName := "savemodelQueue"

// 	if !DB_VALID {
// 		return
// 	}

// 	// make a copy of pending models
// 	mut.Lock()
// 	modelsToWrite := make([]interface{}, len(modelQueue))
// 	// copy pending models to modelsToWrite
// 	copy(modelsToWrite, modelQueue)
// 	// clear pending models
// 	modelQueue = nil
// 	mut.Unlock()

// 	// start session
// 	var tx *gorm.DB = DB.Session(&gorm.Session{PrepareStmt: true, SkipDefaultTransaction: true})

// 	// write models
// 	writeLog(functionName, fmt.Sprintf(`Writing %d models to database`, len(modelsToWrite)), "DEBUG")
// 	res := tx.Create(modelsToWrite)

// 	// check for errors
// 	if res.Error != nil {
// 		writeLog(functionName, fmt.Sprintf(`Error writing models to database: %v`, res.Error), "ERROR")
// 		return
// 	}
// }

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
	case ":NEW:SOLDIER:STATE:":
		soldierStateChan <- out
		temp = `[0, "Logging unit"]`

	case ":LOG:VEHICLE:":
		{
			go logVehicle(out)
			temp = `[0, "Logging vehicle"]`
		}
	case ":EVENT:":
		{
			go processEvent(out)
		}
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
		numWorlds       int = 200
		numMissions     int = 150
		missionDuration int = 60 * 30                                          // s * value = seconds
		numUnits        int = numMissions * 100                                // num missions * num unique units
		numSoldiers     int = int(math.Ceil(float64(numUnits) * float64(0.8))) // numUnits / 3
		// numVehicles      int = int(math.Ceil(float64(numUnits) * float64(0.2))) // numUnits / 3
		numSoldierStates int = numSoldiers * missionDuration // numSoldiers * missionDuration (1s frames)

		sides []string = []string{
			"WEST",
			"EAST",
			"GUER",
			"CIV",
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
	)

	// start a goroutine that will output channel lengths every second
	go func() {
		for {
			time.Sleep(1 * time.Second)
			fmt.Printf("PENDING: Soldiers: %d, SoldierStates: %d\n",
				len(newSoldierChan),
				len(soldierStateChan),
			)
		}
	}()

	// write worlds
	missionsStart := time.Now()
	for i := 0; i < numWorlds; i++ {
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
			"latitude":          0.0,
			"longitude":         0.0,
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
			"missionName":       fmt.Sprintf("Demo Mission %d", i),
			"briefingName":      fmt.Sprintf("Demo Briefing %d", i),
			"missionNameSource": fmt.Sprintf("Demo Mission %d", i),
			"onLoadName":        "",
			"author":            "Demo Author",
			"serverName":        "Demo Server",
			"serverProfile":     "Demo Profile",
			"missionStart":      nil, // random time
			"worldName":         fmt.Sprintf("demo_world_%d", i),
			"tag":               "Demo Tag",
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
	}
	missionsEnd := time.Now()
	missionsElapsed := missionsEnd.Sub(missionsStart)
	fmt.Printf("Sent %d missions in %s\n", numMissions, missionsElapsed)

	// write soldiers, now that our missions exist and the channels have been created
	soldiersStart := time.Now()
	for i := 0; i < numSoldiers; i++ {
		// use channels

		// pick random mission
		mission := Mission{}
		DB.Model(&Mission{}).Order("RANDOM()").First(&mission)

		// soldier := Soldier{
		// 	MissionID: mission.ID,
		// 	// jointime is mission.StartTime + random time between 0 and missionDuration (seconds)
		// 	JoinTime: mission.StartTime.Add(time.Duration(rand.Intn(missionDuration)) * time.Second),
		// 	// joinframe is random time between 0 and missionDuration seconds
		// 	JoinFrame: uint(rand.Intn(missionDuration)),
		// 	// OcapID is random number between 1 and numUnits
		// 	OcapID: uint16(rand.Intn(numUnits) + 1),
		// 	// UnitName is random string
		// 	UnitName: fmt.Sprintf("Demo Unit %d", i),
		// 	// GroupID is random string
		// 	GroupID: fmt.Sprintf("Demo Group %d", i),
		// 	// Side is random string from sides
		// 	Side: sides[rand.Intn(len(sides))],
		// 	// isPlayer is random bool
		// 	IsPlayer: rand.Intn(2) == 1,
		// 	// RoleDescription is random string from roles
		// 	RoleDescription: roles[rand.Intn(len(roles))],
		// }

		// these will be sent as an array
		frame := strconv.FormatInt(int64(rand.Intn(missionDuration)), 10)
		soldier := []string{
			frame,
			string(rand.Intn(numUnits) + 1),
			fmt.Sprintf("Demo Unit %d", i),
			fmt.Sprintf("Demo Group %d", i),
			sides[rand.Intn(len(sides))],
			strconv.FormatBool(rand.Intn(2) == 1),
			roles[rand.Intn(len(roles))],
			frame,
		}

		newSoldierChan <- soldier
	}
	soldiersEnd := time.Now()
	soldiersElapsed := soldiersEnd.Sub(soldiersStart)
	fmt.Printf("Sent %d soldiers in %s\n", numSoldiers, soldiersElapsed)

	// write soldier states
	for i := 0; i < numSoldierStates; i++ {
		// get mission start time
		// timeForThisRow := mission.StartTime
		// get mission end time

	}
	fmt.Println("Demo data populated. Press enter to exit.")
	fmt.Scanln()
}

func main() {
	fmt.Println("Running DB connect/migrate to build schema...")
	err := getDB()
	if err != nil {
		panic(err)
	}
	fmt.Println("DB connect/migrate complete.")

	fmt.Println("Populating demo data...")
	populateDemoData()
}
