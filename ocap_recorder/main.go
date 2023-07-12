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
	"math/rand"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	// mutex for model queue
	mut sync.Mutex
	// model queue to be accessed like ModelQueue.Push() and ModelQueue.Pop()
	ModelQueue = &queue{}
)

// queue struct
type queue struct {
	items []interface{}
}

// push to queue
func (q *queue) Push(item interface{}) {
	mut.Lock()
	q.items = append(q.items, item)
	mut.Unlock()
}

// pop from queue
func (q *queue) Pop() interface{} {
	mut.Lock()
	item := q.items[0]
	q.items = q.items[1:len(q.items)]
	mut.Unlock()
	return item
}

// get length of queue
func (q *queue) Len() int {
	return len(q.items)
}

// return and reset queue
func (q *queue) Reset() []interface{} {
	mut.Lock()
	items := q.items
	q.items = nil
	mut.Unlock()
	return items
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
	MissionName            string    `json:"missionName" gorm:"size:200"`
	BriefingName           string    `json:"briefingName" gorm:"size:200"`
	MissionNameSource      string    `json:"missionNameSource" gorm:"size:200"`
	OnLoadName             string    `json:"onLoadName" gorm:"size:200"`
	Author                 string    `json:"author" gorm:"size:200"`
	ServerName             string    `json:"serverName" gorm:"size:200"`
	ServerProfile          string    `json:"serverProfile" gorm:"size:200"`
	MissionStart           time.Time `json:"missionStart" gorm:"type:timestamptz;index:idx_mission_start"` // time.Time
	WorldName              string    `json:"worldName" gorm:"-"`
	WorldID                uint
	World                  World                   `gorm:"foreignkey:WorldID"`
	Tag                    string                  `json:"tag" gorm:"size:127"`
	Soldiers               []Soldier               `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Vehicles               []Vehicle               `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	EventPlayerConnects    []EventPlayerConnect    `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	EventPlayerDisconnects []EventPlayerDisconnect `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type FrameLog struct {
	gorm.Model
	Time         time.Time `json:"rowTime" gorm:"type:timestamptz;NOT NULL;"`
	Mission      Mission   `gorm:"foreignkey:MissionID"`
	MissionID    uint      `json:"missionId" gorm:"index:,composite:mission_frame,priority:2; index:,composite:mission_frame_ocapid,priority:3"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:,composite:mission_frame,priority:1; index:,composite:mission_frame_ocapid,priority:2"`
	OcapID       uint16    `json:"ocapId" gorm:"index:idx_ocap_id; index:,composite:mission_frame_ocapid,priority:1"`
}

// Soldier inherits from Frame
type Soldier struct {
	FrameLog
	UnitName        string         `json:"unitName" gorm:"size:64"`
	GroupID         string         `json:"groupId" gorm:"size:64"`
	Side            string         `json:"side" gorm:"size:16"`
	IsPlayer        bool           `json:"isPlayer" gorm:"default:false"`
	RoleDescription string         `json:"roleDescription" gorm:"size:64"`
	CurrentRole     string         `json:"currentRole" gorm:"size:64"`
	Position        GPSCoordinates `json:"position" gorm:"type:GEOMETRY"`
	Bearing         uint16         `json:"bearing" gorm:"default:0"`
	Lifestate       uint8          `json:"lifestate" gorm:"default:0"`
	InVehicle       bool           `json:"inVehicle" gorm:"default:false"`
}

type Vehicle struct {
	FrameLog
	VehicleClass string         `json:"vehicleClass" gorm:"size:64"`
	DisplayName  string         `json:"displayName" gorm:"size:64"`
	Position     GPSCoordinates `json:"position" gorm:"type:GEOMETRY"`
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
	var vars []interface{} = []interface{}{"SRID=4326;POINT(0 0)"}
	if !b.Empty() {
		vars = []interface{}{fmt.Sprintf("SRID=4326;POINT(%f %f)", b.X(), b.Y())}
	}
	return clause.Expr{
		SQL:  "ST_PointFromText(?)",
		Vars: vars,
	}
}

func FromCoords(longitude float64, latitude float64) GPSCoordinates {
	return GPSCoordinates(*geom.NewPointFlat(geom.XY, geom.Coord{longitude, latitude}))
}

///////////////////////
// DATABASE OPS //
///////////////////////

func getLocalDB() {
	functionName := "getDB"
	var err error
	DB, err = gorm.Open(sqlite.Open(LOCAL_DB_FILE), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Failed to use local SQlite DB. Not saving! Err: %s`, err), "ERROR")
		DB_VALID = false
		return
	} else {
		writeLog(functionName, "Using local SQlite DB", "INFO")
		DB_VALID = true
		return
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
		PrepareStmt: true,
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
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Failed to connect to TimescaleDB. Err: %s`, err), "ERROR")
		SAVE_LOCAL = true
		getLocalDB()
	} else {
		writeLog(functionName, "Connected to TimescaleDB", "INFO")
		SAVE_LOCAL = false
		DB_VALID = true
	}

	if !DB_VALID {
		writeLog(functionName, "DB not valid. Not saving!", "ERROR")
		return errors.New("DB not valid. Not saving")
	}

	if SAVE_LOCAL {
		// Ensure PostGIS and TimescaleDB extensions are installed
		err = DB.Exec(`
		CREATE EXTENSION IF NOT EXISTS postgis;
		`).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create PostGIS extension. Err: %s`, err), "ERROR")
			return err
		}
		err = DB.Exec(`
		CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;
		`).Error
		if err != nil {
			writeLog(functionName, fmt.Sprintf(`Failed to create TimescaleDB extension. Err: %s`, err), "ERROR")
			return err
		}

		writeLog(functionName, "DB extensions created", "INFO")
	}

	// Migrate the schema
	err = DB.AutoMigrate(&Mission{}, &World{}, &Soldier{}, &Vehicle{}, &EventPlayerConnect{}, &EventPlayerDisconnect{})
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Failed to migrate DB schema. Err: %s`, err), "ERROR")
		return err
	}

	if !SAVE_LOCAL {
		// Set up TimescaleDB hypertable
		hyperTables := []string{
			"soldiers",
			"vehicles",
			"events_player_connects",
			"events_player_disconnects",
		}
		for _, table := range hyperTables {
			query := fmt.Sprintf(`
				SELECT create_hypertable('%s', 'time', migrate_data => true);
			`, table)
			err = DB.Commit().Exec(query).Error
			if err != nil {
				writeLog(functionName, fmt.Sprintf(`Failed to create hypertable for %s. Err: %s`, table, err), "ERROR")
				return err
			}
		}
	}

	writeLog(functionName, "DB initialized", "INFO")

	// start loop to, every 10 seconds, write pending models to DB
	go func() {
		for {
			time.Sleep(10 * time.Second)
			// go writemodelQueue(&mut)
		}
	}()

	return nil
}

// WORLDS AND MISSIONS
var CurrentWorld World
var CurrentMission Mission

// logNewMission logs a new mission to the database and associates the world it's played on
func logNewMission(data []string) (err error) {
	functionName := "logNewMission"

	// unmarshal data[0]
	err = json.Unmarshal([]byte(data[0]), &CurrentWorld)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling world data: %v`, err), "ERROR")
		return err
	}

	// unmarshal data[1]
	err = json.Unmarshal([]byte(data[1]), &CurrentMission)
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error unmarshalling mission data: %v`, err), "ERROR")
		return err
	}

	CurrentMission.MissionStart = time.Now()

	// start transaction
	var tx *gorm.DB = DB.Begin()

	// check if world exists
	tx.Where("world_name = ?", CurrentWorld.WorldName).First(&CurrentWorld)
	if CurrentWorld.ID == 0 {
		// world does not exist, create it
		tx.Create(&CurrentWorld)
	}

	// always write new mission
	CurrentMission.World = CurrentWorld
	tx.Create(&CurrentMission)

	// commit transaction
	err = tx.Commit().Error
	if err != nil {
		writeLog(functionName, fmt.Sprintf(`Error committing transaction: %v`, err), "ERROR")
		return err
	}

	// write to log
	writeLog(functionName, fmt.Sprintf(`New mission logged: %s`, CurrentMission.MissionName), "INFO")
	return nil
}

// SOLDIERS (UNITS) AND VEHICLES

// logSoldier logs a soldier state to the database
func logSoldier(data []string) {

	// check if DB is valid
	if !DB_VALID {
		return
	}

	var soldier Soldier

	// parse data in struct - convert strings when necessary
	soldier.Mission = CurrentMission
	capframe, _ := strconv.Atoi(data[0])
	soldier.CaptureFrame = uint(capframe)
	unitId, _ := strconv.ParseUint(data[1], 10, 64)
	soldier.OcapID = uint16(unitId)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, _ = strconv.ParseBool(data[5])
	soldier.RoleDescription = data[6]
	soldier.CurrentRole = data[7]
	// coordX, _ := strconv.ParseFloat(data[8], 64)
	// coordY, _ := strconv.ParseFloat(data[9], 64)
	// coordZ, _ := strconv.ParseFloat(data[10], 64)
	// soldier.Position = geom.NewPoint(geom.XYZ).MustSetCoords([]float64{coordX, coordY, coordZ}).SetSRID(3857)
	bearing, _ := strconv.Atoi(data[11])
	soldier.Bearing = uint16(bearing)
	lifeState, _ := strconv.Atoi(data[12])
	soldier.Lifestate = uint8(lifeState)
	soldier.InVehicle, _ = strconv.ParseBool(data[13])

	// write
	DB.Create(&soldier)
}

func logVehicle(data []string) {
	if !DB_VALID {
		return
	}

	var vehicle Vehicle

	// parse data in struct - convert strings when necessary
	vehicle.Mission = CurrentMission
	capframe, _ := strconv.Atoi(data[0])
	vehicle.CaptureFrame = uint(capframe)
	vehicleId, _ := strconv.ParseUint(data[1], 10, 64)
	vehicle.OcapID = uint16(vehicleId)
	vehicle.VehicleClass = data[2]
	vehicle.DisplayName = data[3]
	// coordX, _ := strconv.ParseFloat(data[4], 64)
	// coordY, _ := strconv.ParseFloat(data[5], 64)
	// coordZ, _ := strconv.ParseFloat(data[6], 64)
	// vehicle.Position = geom.NewPoint(geom.XYZ).MustSetCoords([]float64{coordX, coordY, coordZ}).SetSRID(3857)
	bearing, _ := strconv.Atoi(data[7])
	vehicle.Bearing = uint16(bearing)
	vehicle.Crew = data[8]

	// write
	DB.Create(&vehicle)
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
		{ // callExtension ["logAttendance", [_hash] call CBA_fnc_encodeJSON]];
			err := getDB()
			if err != nil {
				temp = fmt.Sprintf(
					`[1, "Error getting DB: %s"]`,
					strings.Replace(err.Error(), `"`, `""`, -1),
				)
			} else {
				temp = `[0, "DB initialized"]`
			}
		}
	case "logNewMission":
		{
			err := logNewMission(out)
			if err != nil {
				temp = fmt.Sprintf(
					`[1, "%s"]`,
					strings.Replace(err.Error(), `"`, `""`, -1),
				)
			} else {
				temp = `[0, "Mission logged"]`
			}
		}
	case ":LOG:SOLDIER:":
		{
			go logSoldier(out)
			temp = `[0, "Logging unit"]`
		}

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

func main() {
	fmt.Println("Running DB connect/migrate to build schema...")
	getDB()
	fmt.Println("DB connect/migrate complete.")
	// write 10000 demo records by generating random data of matching types
	// start a transaction

	// create 70 worlds
	worlds := []World{}
	for j := 0; j < 70; j++ {
		worlds = append(worlds, World{
			WorldName: fmt.Sprintf("World %d", j),
		})
	}
	DB.Create(&worlds)

	// create 100 missions with avg recording rows for soldier and vehicle

	for i := 0; i < 100; i++ {
		// create a new mission

		missionStart := time.Now().Add(-time.Duration(rand.Intn(60*60*24*365)) * time.Second)
		mission := Mission{
			MissionName:       fmt.Sprintf("Mission %d", rand.Int()),
			BriefingName:      fmt.Sprintf("Briefing %d", rand.Int()),
			MissionNameSource: fmt.Sprintf("Mission %d", rand.Int()),
			OnLoadName:        fmt.Sprintf("OnLoad %d", rand.Int()),
			Author:            fmt.Sprintf("Author %d", rand.Int()),
			ServerName:        fmt.Sprintf("Server %d", rand.Int()),
			ServerProfile:     fmt.Sprintf("ServerProfile %d", rand.Int()),
			MissionStart:      missionStart,
			Tag:               fmt.Sprintf("Tag %d", rand.Int()),
			EventPlayerConnects: []EventPlayerConnect{
				{
					CaptureFrame: uint32(rand.Int()),
					EventType:    "connected",
					ProfileName:  fmt.Sprintf("Soldier %d", rand.Intn(80*(60*60))),
					PlayerUID:    fmt.Sprintf("PlayerUID %d", rand.Intn(80*(60*60))),
				},
				{
					CaptureFrame: uint32(rand.Int()),
					ProfileName:  fmt.Sprintf("Soldier %d", rand.Intn(80*(60*60))),
					EventType:    "connected",
					PlayerUID:    fmt.Sprintf("PlayerUID %d", rand.Intn(80*(60*60))),
				},
			},
			EventPlayerDisconnects: []EventPlayerDisconnect{
				{
					CaptureFrame: uint32(rand.Int()),
					ProfileName:  fmt.Sprintf("Soldier %d", rand.Intn(80*(60*60))),
					EventType:    "disconnected",
					PlayerUID:    fmt.Sprintf("PlayerUID %d", rand.Intn(80*(60*60))),
				},
				{
					CaptureFrame: uint32(rand.Int()),
					ProfileName:  fmt.Sprintf("Soldier %d", rand.Intn(80*(60*60))),
					EventType:    "disconnected",
					PlayerUID:    fmt.Sprintf("PlayerUID %d", rand.Intn(80*(60*60))),
				},
			},
		}

		// get a random world
		DB.Find(&worlds)
		mission.World = worlds[rand.Intn(len(worlds))]

		DB.Create(&mission)

		var demoRowCount int = 450
		var numSeconds int = 60 * 60
		// 450 per second for 1 hour

		// run half and half
		for k := 0; k < numSeconds; k++ {
			frame := uint(k)

			var wg sync.WaitGroup
			var mut sync.Mutex

			soldiers, vehicles := []Soldier{}, []Vehicle{}

			for j := 0; j < demoRowCount; j++ {
				wg.Add(1)

				// spawn parallel thread for each row
				// return the soldier and vehicle to respective channels

				go func(frame uint, mission Mission, missionStart time.Time, i int, j int) {

					// fmt.Println("Processing mission", i, "of 100", "::", "Frame", frame, "of", numSeconds, "::", "Row", j, "of", demoRowCount)

					missionStart = missionStart.Add(time.Second * 1)
					frameLogSoldier := FrameLog{
						Time:         missionStart,
						CaptureFrame: frame,
						OcapID:       uint16(j),
						MissionID:    mission.ID,
					}

					poss, err := GPSCoordinatesFromString(fmt.Sprintf("%d,%d", rand.Int(), rand.Int()))
					if err != nil {
						panic(err)
					}
					soldier := Soldier{
						FrameLog: frameLogSoldier,
						UnitName: fmt.Sprintf("Soldier %d", rand.Int()),
						Position: poss,
					}

					frameLogVehicle := FrameLog{
						Time:         missionStart,
						CaptureFrame: frame,
						OcapID:       uint16(j + demoRowCount),
						MissionID:    mission.ID,
					}

					posv, err := GPSCoordinatesFromString(fmt.Sprintf("%d,%d", rand.Int(), rand.Int()))
					if err != nil {
						panic(err)
					}
					vehicle := Vehicle{
						FrameLog:     frameLogVehicle,
						VehicleClass: fmt.Sprintf("Vehicle %d", rand.Int()),
						DisplayName:  fmt.Sprintf("Vehicle %d", rand.Int()),
						Position:     posv,
					}

					// write to var w mutex
					mut.Lock()
					soldiers = append(soldiers, soldier)
					vehicles = append(vehicles, vehicle)
					mut.Unlock()

					// signal that this goroutine is done
					wg.Done()

				}(frame, mission, missionStart, i, j)
			}

			// wait for all processing goroutines to complete
			wg.Wait()

			// report counts
			fmt.Println("Soldiers:", len(soldiers))
			fmt.Println("Vehicles:", len(vehicles))

			// // start a transaction
			// fmt.Println("Starting transaction...")
			// tx := DB.Begin()

			// tx.Model(&mission).Association("Soldiers").Append(&soldiers)
			// tx.Model(&mission).Association("Vehicles").Append(&vehicles)

			// // commit transaction
			// fmt.Println("Committing transaction...")
			// tx.Commit()

			// try writing instantly instead
			fmt.Println("Writing to database...")

			// make progress channels
			soldierProgress, vehicleProgress := make(chan uint, len(soldiers)), make(chan uint, len(vehicles))

			batchDone := false

			// report progress from channels, based on total number of rows vs pending goroutines
			// run this logger in a goroutine but stop it when all goroutines are done
			go func() {
				for {
					select {
					case <-soldierProgress:
						fmt.Printf("Soldier progress: %d / %d - ID %d", len(soldierProgress), len(soldiers), <-soldierProgress)
						fmt.Print(" :: ")
						fmt.Printf("Vehicle progress: %d / %d\n", len(vehicleProgress), len(vehicles))
					case <-vehicleProgress:
						fmt.Printf("Soldier progress: %d / %d", len(soldierProgress), len(soldiers))
						fmt.Print(" :: ")
						fmt.Printf("Vehicle progress: %d / %d - ID %d\n", len(vehicleProgress), len(vehicles), <-vehicleProgress)
					}
					if batchDone {
						close(soldierProgress)
						close(vehicleProgress)
						break
					}
				}
			}()

			outsideStartT := time.Now()
			var endT time.Time

			wgSoldier := sync.WaitGroup{}
			startTSoldier := time.Now()
			wgSoldier.Add(1)
			go func() {
				pendingSoldier := len(soldiers)
				for _, soldier := range soldiers {
					wgSoldier.Add(1)
					go func(soldier Soldier, pendingSoldier int) {
						DB.Create(&soldier)
						soldierProgress <- soldier.ID
						wgSoldier.Done()
					}(soldier, pendingSoldier)
				}
				wgSoldier.Done()
			}()
			// wait for all processing goroutines to complete
			wgSoldier.Wait()

			endT = time.Now()
			fmt.Println("Took", endT.Sub(startTSoldier), "to write", len(soldiers), "soldiers")

			wgVehicle := sync.WaitGroup{}
			startTVehicle := time.Now()
			wgVehicle.Add(1)
			go func() {
				pendingVehicle := len(vehicles)
				for _, vehicle := range vehicles {
					wgVehicle.Add(1)
					go func(vehicle Vehicle, pendingVehicle int) {
						DB.Create(&vehicle)
						vehicleProgress <- vehicle.ID
						wgVehicle.Done()
					}(vehicle, pendingVehicle)
				}
				wgVehicle.Done()
			}()

			// wait for all processing goroutines to complete
			wgVehicle.Wait()
			batchDone = true

			endT = time.Now()
			fmt.Println("Took", endT.Sub(startTVehicle), "to write", len(vehicles), "vehicles")

			// wait for all processing goroutines to complete
			endT = time.Now()
			fmt.Println("Took", endT.Sub(outsideStartT), "to write", len(soldiers)+len(vehicles), "total")

			fmt.Println("Finished frame", frame, "of", numSeconds)
			// wait 1 second
			// time.Sleep(time.Millisecond * 200)
		}

	}

	fmt.Println("Demo data populated. Press enter to exit.")
	fmt.Scanln()
}
