package parser

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/convert"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/pkg/a3interface"

	geom "github.com/peterstace/simplefeatures/geom"
	"gorm.io/gorm"
)

// parseUintFromFloat parses a string that may be an integer ("32") or float ("32.00") into uint64.
// ArmA 3's SQF has no integer type, so the extension API may serialize numbers as floats.
func parseUintFromFloat(s string) (uint64, error) {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f < 0 || f != float64(uint64(f)) {
		return 0, fmt.Errorf("parseUintFromFloat: %q is not a valid uint64", s)
	}
	return uint64(f), nil
}

// parseIntFromFloat parses a string that may be an integer or float into int64.
func parseIntFromFloat(s string) (int64, error) {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f != float64(int64(f)) {
		return 0, fmt.Errorf("parseIntFromFloat: %q is not a valid int64", s)
	}
	return int64(f), nil
}

// MissionContext holds the current mission and world state
type MissionContext struct {
	mu      sync.RWMutex
	Mission *model.Mission
	World   *model.World
}

// NewMissionContext creates a new MissionContext with default values
func NewMissionContext() *MissionContext {
	return &MissionContext{
		Mission: &model.Mission{MissionName: "No mission loaded"},
		World:   &model.World{WorldName: "No world loaded"},
	}
}

// GetMission returns the current mission
func (mc *MissionContext) GetMission() *model.Mission {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.Mission
}

// GetWorld returns the current world
func (mc *MissionContext) GetWorld() *model.World {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.World
}

// SetMission sets the current mission and world
func (mc *MissionContext) SetMission(mission *model.Mission, world *model.World) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.Mission = mission
	mc.World = world
}

// Dependencies holds all dependencies needed by the parser
type Dependencies struct {
	DB               *gorm.DB
	EntityCache      *cache.EntityCache
	MarkerCache      *cache.MarkerCache
	LogManager       *logging.SlogManager
	ExtensionName    string
	AddonVersion     string
	ExtensionVersion string
}

// Parser provides methods for parsing game data into model structs
type Parser struct {
	deps         Dependencies
	ctx          *MissionContext
	writeLogFunc func(functionName, data, level string)
	backend      storage.Backend
}

// NewParser creates a new parser
func NewParser(deps Dependencies, ctx *MissionContext) *Parser {
	s := &Parser{
		deps: deps,
		ctx:  ctx,
	}
	// Default writeLog function uses the logging manager
	s.writeLogFunc = func(functionName, data, level string) {
		if deps.LogManager != nil {
			deps.LogManager.WriteLog(functionName, data, level)
		}
	}
	return s
}

// GetMissionContext returns the mission context
func (s *Parser) GetMissionContext() *MissionContext {
	return s.ctx
}

// SetBackend sets the storage backend for mission start/end handling
func (s *Parser) SetBackend(b storage.Backend) {
	s.backend = b
}

func (s *Parser) writeLog(functionName, data, level string) {
	s.writeLogFunc(functionName, data, level)
}

// InitMission logs a new mission to the database
func (s *Parser) InitMission(data []string) error {
	functionName := ":NEW:MISSION:"

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	world := model.World{}
	mission := model.Mission{}
	// unmarshal data[0]
	err := json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error unmarshalling world data: %v`, err), "ERROR")
		return err
	}

	// preprocess the world 'location' to geopoint
	worldLocation, err := geo.Coords3857From4326(
		float64(world.Longitude),
		float64(world.Latitude),
	)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting world location to geopoint: %v`, err), "ERROR")
		return err
	}
	world.Location = worldLocation

	// unmarshal data[1]
	// unmarshal to temp object too to extract addons
	missionTemp := map[string]interface{}{}
	if err = json.Unmarshal([]byte(data[1]), &missionTemp); err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error unmarshalling mission data: %v`, err), "ERROR")
		return err
	}

	// add addons
	addons := []model.Addon{}
	for _, addon := range missionTemp["addons"].([]interface{}) {
		thisAddon := model.Addon{
			Name: addon.([]interface{})[0].(string),
		}
		// if addon[1] workshopId is int, convert to string
		if reflect.TypeOf(addon.([]interface{})[1]).Kind() == reflect.Float64 {
			thisAddon.WorkshopID = strconv.Itoa(int(addon.([]interface{})[1].(float64)))
		} else {
			thisAddon.WorkshopID = addon.([]interface{})[1].(string)
		}

		// Only use DB for addon lookup/create if DB is available
		if s.deps.DB != nil {
			// if addon doesn't exist, insert it
			err = s.deps.DB.Where("name = ?", thisAddon.Name).First(&thisAddon).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				s.writeLog(functionName, fmt.Sprintf(`Error checking if addon exists: %v`, err), "ERROR")
				return err
			}
			if thisAddon.ID == 0 {
				// addon does not exist, create it
				if err = s.deps.DB.Create(&thisAddon).Error; err != nil {
					s.writeLog(functionName, fmt.Sprintf(`Error creating addon: %v`, err), "ERROR")
					return err
				}
			}
		}
		addons = append(addons, thisAddon)
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
	mission.AddonVersion = s.deps.AddonVersion
	mission.ExtensionVersion = s.deps.ExtensionVersion

	logger := s.deps.LogManager.Logger()

	// Only use DB for world/mission persistence if DB is available
	if s.deps.DB != nil {
		// get or insert world
		created, err := world.GetOrInsert(s.deps.DB)
		if err != nil {
			logger.Error("Failed to get or insert world", "error", err)
			return err
		}
		if created {
			logger.Debug("New world inserted", "worldName", world.WorldName)
		} else {
			logger.Debug("World already exists", "worldName", world.WorldName)
		}

		// always write new mission
		mission.World = world
		err = s.deps.DB.Create(&mission).Error
		if err != nil {
			logger.Error("Failed to insert new mission", "error", err)
			return err
		} else {
			logger.Debug("New mission inserted", "missionName", mission.MissionName)
		}
	} else {
		// Memory-only mode: just set the world reference
		mission.World = world
		logger.Debug("Memory-only mode: mission context set without DB persistence", "missionName", mission.MissionName)
	}

	// set current world and mission
	s.ctx.SetMission(&mission, &world)

	// Clear marker cache for new mission
	s.deps.MarkerCache.Reset()

	// Start mission in storage backend if configured
	if s.backend != nil {
		coreMission := convert.MissionToCore(&mission)
		coreWorld := convert.WorldToCore(&world)
		if err := s.backend.StartMission(&coreMission, &coreWorld); err != nil {
			logger.Error("Failed to start mission in storage backend", "error", err)
		}
	}

	// write to log
	s.writeLog(functionName, `New mission logged`, "INFO")

	logger.Debug("World data",
		"worldName", world.WorldName,
		"displayName", world.DisplayName)
	logger.Debug("Mission data",
		"missionName", mission.MissionName,
		"briefingName", mission.BriefingName,
		"serverName", mission.ServerName,
		"serverProfile", mission.ServerProfile,
		"onLoadName", mission.OnLoadName,
		"author", mission.Author,
		"tag", mission.Tag)

	// callback to addon to begin sending data
	a3interface.WriteArmaCallback(s.deps.ExtensionName, `:MISSION:OK:`, "OK")
	return nil
}

// ParseSoldier parses soldier data and returns a Soldier model
func (s *Parser) ParseSoldier(data []string) (model.Soldier, error) {
	functionName := ":NEW:SOLDIER:"
	var soldier model.Soldier

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return soldier, err
	}

	// parse array
	soldier.MissionID = s.ctx.GetMission().ID
	soldier.JoinFrame = uint(capframe)

	soldier.JoinTime = time.Now()

	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return soldier, err
	}
	soldier.ObjectID = uint16(ocapID)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, err = strconv.ParseBool(data[5])
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
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
		s.writeLog(functionName, fmt.Sprintf(`Error unmarshalling squadParams: %v`, err), "ERROR")
		return soldier, err
	}

	return soldier, nil
}

// ParseSoldierState parses soldier state data and returns a SoldierState model
func (s *Parser) ParseSoldierState(data []string) (model.SoldierState, error) {
	functionName := ":NEW:SOLDIER:STATE:"
	var soldierState model.SoldierState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	soldierState.MissionID = s.ctx.GetMission().ID

	frameStr := data[8]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return soldierState, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}
	soldierState.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		return soldierState, fmt.Errorf(`error converting ocapId to uint: %v`, err)
	}

	// try and find soldier in DB to associate
	soldier, ok := s.deps.EntityCache.GetSoldier(uint16(ocapID))
	if !ok {
		return soldierState, fmt.Errorf("soldier %d not found in cache", ocapID)
	}

	soldierState.SoldierObjectID = soldier.ObjectID

	soldierState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := geo.Coord3857FromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		s.writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", jsonData, err), "ERROR")
		return soldierState, err
	}
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
		s.writeLog(functionName, fmt.Sprintf(`Error converting isPlayer to bool: %v`, err), "ERROR")
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
		if len(scoresArr) >= 6 {
			scoresInt := make([]uint8, len(scoresArr))
			for i, v := range scoresArr {
				num, _ := strconv.Atoi(v)
				scoresInt[i] = uint8(num)
			}

			soldierState.Scores = model.SoldierScores{
				InfantryKills: scoresInt[0],
				VehicleKills:  scoresInt[1],
				ArmorKills:    scoresInt[2],
				AirKills:      scoresInt[3],
				Deaths:        scoresInt[4],
				TotalScore:    scoresInt[5],
			}
		} else {
			s.writeLog(functionName, fmt.Sprintf("expected 6 score values, got %d: %q", len(scoresArr), scoresStr), "WARN")
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

	// groupID and side (added by addon PR#55, backward compatible)
	if len(data) >= 17 {
		soldierState.GroupID = data[15]
		soldierState.Side = data[16]
	} else {
		// Fall back to initial registration data
		soldierState.GroupID = soldier.GroupID
		soldierState.Side = soldier.Side
	}

	return soldierState, nil
}

// ParseVehicle parses vehicle data and returns a Vehicle model
func (s *Parser) ParseVehicle(data []string) (model.Vehicle, error) {
	functionName := ":NEW:VEHICLE:"
	var vehicle model.Vehicle

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return vehicle, err
	}

	vehicle.JoinTime = time.Now()

	// parse array
	vehicle.MissionID = s.ctx.GetMission().ID
	vehicle.JoinFrame = uint(capframe)
	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting ocapID to uint: %v`, err), "ERROR")
		return vehicle, err
	}
	vehicle.ObjectID = uint16(ocapID)
	vehicle.OcapType = data[2]
	vehicle.DisplayName = data[3]
	vehicle.ClassName = data[4]
	vehicle.Customization = data[5]

	return vehicle, nil
}

// ParseVehicleState parses vehicle state data and returns a VehicleState model
func (s *Parser) ParseVehicleState(data []string) (model.VehicleState, error) {
	functionName := ":NEW:VEHICLE:STATE:"
	var vehicleState model.VehicleState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	vehicleState.MissionID = s.ctx.GetMission().ID

	// get frame
	frameStr := data[5]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting ocapId to uint: %v`, err), "ERROR")
		return vehicleState, err
	}

	// try and find vehicle in DB to associate
	vehicle, ok := s.deps.EntityCache.GetVehicle(uint16(ocapID))
	if !ok {
		return vehicleState, fmt.Errorf("vehicle %d not found in cache", ocapID)
	}
	vehicleState.VehicleObjectID = vehicle.ObjectID

	vehicleState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := geo.Coord3857FromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		s.writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", jsonData, err), "ERROR")
		return vehicleState, err
	}
	vehicleState.Position = point
	vehicleState.ElevationASL = float32(elev)

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	vehicleState.Bearing = uint16(bearing)
	// is alive
	isAlive, _ := strconv.ParseBool(data[3])
	vehicleState.IsAlive = isAlive
	// parse crew, which is a JSON array of ocap ids of soldiers (e.g. "[202,203]")
	vehicleState.Crew = data[4]

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

// ParseProjectileEvent parses projectile event data and returns a ProjectileEvent model
// New SQF array format (indices):
//
//	0:  firedFrame (uint)
//	1:  firedTime (float - diag_tickTime)
//	2:  firerID (uint)
//	3:  vehicleID (uint, -1 if not in vehicle)
//	4:  vehicleRole (string)
//	5:  remoteControllerID (uint)
//	6:  weapon (string - CfgWeapons class name, e.g. "arifle_katiba_f")
//	7:  weaponDisplay (string - display name, e.g. "Katiba 6.5 mm")
//	8:  muzzle (string - CfgWeapons muzzle class name, e.g. "arifle_Katiba_pointer_F")
//	9:  muzzleDisplay (string - muzzle display name, e.g. "Katiba 6.5 mm")
//	10: magazine (string - CfgMagazines class name, e.g. "30Rnd_65x39_caseless_green")
//	11: magazineDisplay (string - magazine display name, e.g. "6.5 mm 30Rnd Caseless Mag")
//	12: ammo (string - CfgAmmo class name, e.g. "B_65x39_Caseless_green")
//	13: fireMode (string)
//	14: positions (array string "[[tickTime,frameNo,\"x,y,z\"],...]")
//	15: initialVelocity (string "x,y,z")
//	16: hitParts (array string)
//	17: sim (string - simulation type, required: "shotBullet", "shotGrenade", "shotRocket", "shotMissile", "shotShell", etc.)
//	18: isSub (bool - is submunition)
//	19: magazineIcon (string - path to magazine icon texture)
func (s *Parser) ParseProjectileEvent(data []string) (model.ProjectileEvent, error) {
	var projectileEvent model.ProjectileEvent
	logger := s.deps.LogManager.Logger()

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	if len(data) < 20 {
		return projectileEvent, fmt.Errorf("insufficient data fields: got %d, need 20", len(data))
	}

	projectileEvent.MissionID = s.ctx.GetMission().ID

	// [0] firedFrame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return projectileEvent, fmt.Errorf("error parsing firedFrame: %v", err)
	}
	projectileEvent.CaptureFrame = uint(capframe)

	// [1] firedTime (diag_tickTime) - convert to Time using current time as base
	// diag_tickTime is seconds since game start, so we just store current time
	projectileEvent.Time = time.Now()

	// [2] firerID
	firerID, err := parseUintFromFloat(data[2])
	if err != nil {
		return projectileEvent, fmt.Errorf("error parsing firerID: %v", err)
	}
	soldierFired, ok := s.deps.EntityCache.GetSoldier(uint16(firerID))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d not found in cache", firerID)
	}
	projectileEvent.FirerObjectID = soldierFired.ObjectID

	// [3] vehicleID (-1 if not in vehicle)
	vehicleID, err := parseIntFromFloat(data[3])
	if err != nil {
		return projectileEvent, fmt.Errorf("error parsing vehicleID: %v", err)
	}
	if vehicleID >= 0 {
		vehicle, ok := s.deps.EntityCache.GetVehicle(uint16(vehicleID))
		if ok {
			projectileEvent.VehicleObjectID = sql.NullInt32{Int32: int32(vehicle.ObjectID), Valid: true}
		}
	}

	// [4] vehicleRole
	projectileEvent.VehicleRole = data[4]

	// [5] remoteControllerID
	remoteControllerID, err := parseUintFromFloat(data[5])
	if err != nil {
		return projectileEvent, fmt.Errorf("error parsing remoteControllerID: %v", err)
	}
	actualFirer, ok := s.deps.EntityCache.GetSoldier(uint16(remoteControllerID))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d (remoteController) not found in cache", remoteControllerID)
	}
	projectileEvent.ActualFirerObjectID = actualFirer.ObjectID

	// [6-13] weapon info
	projectileEvent.Weapon = data[6]
	projectileEvent.WeaponDisplay = data[7]
	projectileEvent.Muzzle = data[8]
	projectileEvent.MuzzleDisplay = data[9]
	projectileEvent.Magazine = data[10]
	projectileEvent.MagazineDisplay = data[11]
	projectileEvent.Ammo = data[12]
	projectileEvent.Mode = data[13]

	// [14] positions - SQF array "[[tickTime,frameNo,\"x,y,z\"],...]"
	var positions [][]interface{}
	if err := json.Unmarshal([]byte(data[14]), &positions); err != nil {
		return projectileEvent, fmt.Errorf("error parsing positions: %v", err)
	}

	positionSequence := []float64{}
	for _, posArr := range positions {
		if len(posArr) < 3 {
			continue
		}

		// posArr[1] = frameNo (float64 from JSON)
		frameNo, ok := posArr[1].(float64)
		if !ok {
			logger.Warn("Invalid frameNo in position", "value", posArr[1])
			continue
		}

		// posArr[2] = "x,y,z" position string
		posStr, ok := posArr[2].(string)
		if !ok {
			logger.Warn("Invalid position string", "value", posArr[2])
			continue
		}

		point, _, err := geo.Coord3857FromString(posStr)
		if err != nil {
			logger.Warn("Error converting position to Point", "error", err, "pos", posStr)
			continue
		}
		coords, _ := point.Coordinates()

		// Store as XYZM where M = frame number (used by projectile markers)
		positionSequence = append(
			positionSequence,
			coords.XY.X,
			coords.XY.Y,
			coords.Z,
			frameNo,
		)
	}

	// create the linestring if we have positions
	if len(positionSequence) >= 8 { // at least 2 points (4 values each)
		posSeq := geom.NewSequence(positionSequence, geom.DimXYZM)
		ls := geom.NewLineString(posSeq)
		projectileEvent.Positions = ls.AsGeometry()
	}

	// [15] initialVelocity
	projectileEvent.InitialVelocity = data[15]

	// [16] hitParts - SQF array "[[entityID,[components],\"x,y,z\",frameNo],...]"
	projectileEvent.HitSoldiers = []model.ProjectileHitsSoldier{}
	projectileEvent.HitVehicles = []model.ProjectileHitsVehicle{}

	var hitParts [][]interface{}
	if err := json.Unmarshal([]byte(data[16]), &hitParts); err != nil {
		logger.Warn("Error parsing hitParts", "error", err, "data", data[16])
	} else {
		for _, eventArr := range hitParts {
			if len(eventArr) < 4 {
				continue
			}

			// [0] hit entity ocap id
			hitEntityID, ok := eventArr[0].(float64)
			if !ok {
				continue
			}

			// [1] hit component(s) - string for HitPart, array for HitExplosion
			hitComponents := []string{}
			switch comp := eventArr[1].(type) {
			case string:
				// Single component (HitPart event)
				hitComponents = append(hitComponents, comp)
			case []interface{}:
				// Multiple components (HitExplosion event)
				for _, v := range comp {
					if s, ok := v.(string); ok {
						hitComponents = append(hitComponents, s)
					}
				}
			}

			// [2] hit position "x,y,z"
			hitPosStr, ok := eventArr[2].(string)
			if !ok {
				continue
			}
			hitPoint, _, err := geo.Coord3857FromString(hitPosStr)
			if err != nil {
				logger.Warn("Error converting hit position", "error", err)
				continue
			}

			// [3] capture frame
			hitFrame, ok := eventArr[3].(float64)
			if !ok {
				continue
			}

			hitComponentsJSON, _ := json.Marshal(hitComponents)

			// Try soldier first, then vehicle
			if hitEntity, ok := s.deps.EntityCache.GetSoldier(uint16(hitEntityID)); ok {
				projectileEvent.HitSoldiers = append(projectileEvent.HitSoldiers,
					model.ProjectileHitsSoldier{
						SoldierObjectID: hitEntity.ObjectID,
						ComponentsHit:   hitComponentsJSON,
						CaptureFrame:    uint(hitFrame),
						Position:        hitPoint,
					})
			} else if hitVehicle, ok := s.deps.EntityCache.GetVehicle(uint16(hitEntityID)); ok {
				projectileEvent.HitVehicles = append(projectileEvent.HitVehicles,
					model.ProjectileHitsVehicle{
						VehicleObjectID: hitVehicle.ObjectID,
						ComponentsHit:   hitComponentsJSON,
						CaptureFrame:    uint(hitFrame),
						Position:        hitPoint,
					})
			} else {
				logger.Warn("Hit entity not found in cache", "hitEntityID", uint16(hitEntityID))
			}
		}
	}

	// [17] sim - simulation type
	projectileEvent.SimulationType = data[17]

	// [18] isSub - is submunition
	isSub, err := strconv.ParseBool(data[18])
	if err == nil {
		projectileEvent.IsSubmunition = isSub
	}

	// [19] magazineIcon - magazine icon path
	projectileEvent.MagazineIcon = data[19]

	return projectileEvent, nil
}

// ParseGeneralEvent parses general event data and returns a GeneralEvent model
func (s *Parser) ParseGeneralEvent(data []string) (model.GeneralEvent, error) {
	functionName := ":EVENT:"
	var thisEvent model.GeneralEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog("processEvent", fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return thisEvent, err
	}

	thisEvent.Time = time.Now()

	thisEvent.Mission = *s.ctx.GetMission()

	// get event type
	thisEvent.CaptureFrame = uint(capframe)

	// get event type
	thisEvent.Name = data[1]

	// get event message
	thisEvent.Message = data[2]

	// get extra event data
	if len(data) > 3 {
		err = json.Unmarshal([]byte(data[3]), &thisEvent.ExtraData)
		if err != nil {
			s.writeLog(functionName, fmt.Sprintf(`Error unmarshalling extra data: %s`, err), "ERROR")
			return thisEvent, err
		}
	}

	return thisEvent, nil
}

// ParseKillEvent parses kill event data and returns a KillEvent model
func (s *Parser) ParseKillEvent(data []string) (model.KillEvent, error) {
	var killEvent model.KillEvent

	// Save weapon array before FixEscapeQuotes — it corrupts SQF array
	// delimiters (e.g. ["","",""] → [",","]) by treating the adjacent
	// quotes as escape sequences rather than array syntax.
	rawWeapon := util.TrimQuotes(data[3])

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	killEvent.Time = time.Now()

	killEvent.CaptureFrame = uint(capframe)
	killEvent.Mission = *s.ctx.GetMission()

	// parse data in array
	victimObjectID, err := parseUintFromFloat(data[1])
	if err != nil {
		return killEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// Set victim ObjectID - check if soldier or vehicle
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(victimObjectID)); ok {
		killEvent.VictimSoldierObjectID = sql.NullInt32{Int32: int32(victimObjectID), Valid: true}
	} else if _, ok := s.deps.EntityCache.GetVehicle(uint16(victimObjectID)); ok {
		killEvent.VictimVehicleObjectID = sql.NullInt32{Int32: int32(victimObjectID), Valid: true}
	} else {
		return killEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimObjectID)
	}

	// parse killer ObjectID
	killerObjectID, err := parseUintFromFloat(data[2])
	if err != nil {
		return killEvent, fmt.Errorf(`error converting killer ocap id to uint: %v`, err)
	}

	// Set killer ObjectID - check if soldier or vehicle
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(killerObjectID)); ok {
		killEvent.KillerSoldierObjectID = sql.NullInt32{Int32: int32(killerObjectID), Valid: true}
	} else if _, ok := s.deps.EntityCache.GetVehicle(uint16(killerObjectID)); ok {
		killEvent.KillerVehicleObjectID = sql.NullInt32{Int32: int32(killerObjectID), Valid: true}
	} else {
		return killEvent, fmt.Errorf(`killer ocap id not found in cache: %d`, killerObjectID)
	}

	// get weapon info - parse SQF array [vehicleName, weaponDisp, magDisp]
	killEvent.WeaponVehicle, killEvent.WeaponName, killEvent.WeaponMagazine = util.ParseSQFStringArray(rawWeapon)
	killEvent.EventText = util.FormatWeaponText(killEvent.WeaponVehicle, killEvent.WeaponName, killEvent.WeaponMagazine)

	// get event distance
	distance, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return killEvent, fmt.Errorf(`error converting distance to float: %v`, err)
	}
	killEvent.Distance = float32(distance)

	return killEvent, nil
}

// ParseChatEvent parses chat event data and returns a ChatEvent model
func (s *Parser) ParseChatEvent(data []string) (model.ChatEvent, error) {
	var chatEvent model.ChatEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	chatEvent.Time = time.Now()

	chatEvent.CaptureFrame = uint(capframe)
	chatEvent.Mission = *s.ctx.GetMission()

	// parse data in array
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		if _, ok := s.deps.EntityCache.GetSoldier(uint16(senderObjectID)); !ok {
			return chatEvent, fmt.Errorf(`sender ocap id not found in cache: %d`, senderObjectID)
		}
		chatEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
	}

	// channel is the 3rd element, compare against map
	channelInt, err := parseIntFromFloat(data[2])
	if err != nil {
		return chatEvent, fmt.Errorf(`error converting channel to int: %v`, err)
	}
	channelName, ok := model.ChatChannels[int(channelInt)]
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

// ParseRadioEvent parses radio event data and returns a RadioEvent model
func (s *Parser) ParseRadioEvent(data []string) (model.RadioEvent, error) {
	var radioEvent model.RadioEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	radioEvent.Time = time.Now()

	radioEvent.CaptureFrame = uint(capframe)
	radioEvent.Mission = *s.ctx.GetMission()

	// parse data in array
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return radioEvent, fmt.Errorf(`error converting sender ocap id to uint: %v`, err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		if _, ok := s.deps.EntityCache.GetSoldier(uint16(senderObjectID)); !ok {
			return radioEvent, fmt.Errorf(`sender ocap id not found in cache: %d`, senderObjectID)
		}
		radioEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
	}

	// radio
	radioEvent.Radio = data[2]
	// radio type (SW or LR)
	radioEvent.RadioType = data[3]
	// transmission type (start/end)
	radioEvent.StartEnd = data[4]
	// channel on radio (1-8) int8
	channelInt, err := parseIntFromFloat(data[5])
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

// ParseFpsEvent parses FPS event data and returns a ServerFpsEvent model
func (s *Parser) ParseFpsEvent(data []string) (model.ServerFpsEvent, error) {
	var fpsEvent model.ServerFpsEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return fpsEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	fpsEvent.CaptureFrame = uint(capframe)
	fpsEvent.Time = time.Now()
	fpsEvent.Mission = *s.ctx.GetMission()

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

// ParseTimeState parses time state data and returns a TimeState model
// Args: [frameNo, systemTimeUTC, missionDateTime, timeMultiplier, missionTime]
func (s *Parser) ParseTimeState(data []string) (model.TimeState, error) {
	var timeState model.TimeState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return timeState, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	timeState.CaptureFrame = uint(capframe)
	timeState.Time = time.Now()
	timeState.Mission = *s.ctx.GetMission()

	// systemTimeUTC - e.g., "2024-01-15T14:30:45.123"
	timeState.SystemTimeUTC = data[1]

	// missionDateTime - e.g., "2035-06-15T06:00:00"
	timeState.MissionDate = data[2]

	// timeMultiplier
	timeMult, err := strconv.ParseFloat(data[3], 64)
	if err != nil {
		return timeState, fmt.Errorf(`error converting timeMultiplier to float: %v`, err)
	}
	timeState.TimeMultiplier = float32(timeMult)

	// missionTime (seconds since start)
	missionTime, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return timeState, fmt.Errorf(`error converting missionTime to float: %v`, err)
	}
	timeState.MissionTime = float32(missionTime)

	return timeState, nil
}

// ParseAce3DeathEvent parses ACE3 death event data and returns an Ace3DeathEvent model
func (s *Parser) ParseAce3DeathEvent(data []string) (model.Ace3DeathEvent, error) {
	var deathEvent model.Ace3DeathEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	deathEvent.Time = time.Now()

	deathEvent.CaptureFrame = uint(capframe)
	deathEvent.Mission = *s.ctx.GetMission()

	// parse data in array
	victimObjectID, err := parseUintFromFloat(data[1])
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// Set victim ObjectID
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(victimObjectID)); !ok {
		return deathEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimObjectID)
	}
	deathEvent.SoldierObjectID = uint16(victimObjectID)

	deathEvent.Reason = data[2]

	// get last damage source id [3]
	lastDamageSourceID, err := parseIntFromFloat(data[3])
	if err != nil {
		return deathEvent, fmt.Errorf(`error converting last damage source id to uint: %v`, err)
	}

	if lastDamageSourceID > -1 {
		lastDamageSource, ok := s.deps.EntityCache.GetSoldier(uint16(lastDamageSourceID))
		if !ok {
			return deathEvent, fmt.Errorf(`last damage source id not found in cache: %d`, lastDamageSourceID)
		}
		deathEvent.LastDamageSourceObjectID = sql.NullInt32{Int32: int32(lastDamageSource.ObjectID), Valid: true}
	}

	return deathEvent, nil
}

// ParseAce3UnconsciousEvent parses ACE3 unconscious event data and returns an Ace3UnconsciousEvent model
func (s *Parser) ParseAce3UnconsciousEvent(data []string) (model.Ace3UnconsciousEvent, error) {
	var unconsciousEvent model.Ace3UnconsciousEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	unconsciousEvent.CaptureFrame = uint(capframe)
	unconsciousEvent.Mission = *s.ctx.GetMission()

	// parse data in array
	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting ocap id to uint: %v`, err)
	}

	// Set soldier ObjectID
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(ocapID)); !ok {
		return unconsciousEvent, fmt.Errorf("soldier %d not found in cache", ocapID)
	}
	unconsciousEvent.SoldierObjectID = uint16(ocapID)

	isUnconscious, err := strconv.ParseBool(data[2])
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting isUnconscious to bool: %v`, err)
	}
	unconsciousEvent.IsUnconscious = isUnconscious

	return unconsciousEvent, nil
}

// ParseMarkerCreate parses marker create data and returns a Marker model
func (s *Parser) ParseMarkerCreate(data []string) (model.Marker, error) {
	functionName := ":NEW:MARKER:"
	var marker model.Marker

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	marker.MissionID = s.ctx.GetMission().ID

	// markerName
	marker.MarkerName = data[0]

	// direction
	dir, err := strconv.ParseFloat(data[1], 32)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing direction: %v", err), "ERROR")
		return marker, err
	}
	marker.Direction = float32(dir)

	// type
	marker.MarkerType = data[2]

	// text
	marker.Text = data[3]

	// frameNo
	frameStr := data[4]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return marker, err
	}
	marker.CaptureFrame = uint(capframe)

	// data[5] is -1, skip

	// ownerId
	ownerId, err := strconv.Atoi(data[6])
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing ownerId: %v", err), "WARN")
		ownerId = -1
	}
	marker.OwnerID = ownerId

	// color
	marker.Color = data[7]

	// size (stored as string "[w,h]")
	marker.Size = data[8]

	// side
	marker.Side = data[9]

	// shape - read first to determine position format
	marker.Shape = data[11]

	// position - parse based on shape
	pos := data[10]
	if marker.Shape == "POLYLINE" {
		polyline, err := geo.ParsePolyline(pos)
		if err != nil {
			s.writeLog(functionName, fmt.Sprintf("Error parsing polyline: %v", err), "ERROR")
			return marker, err
		}
		marker.Polyline = polyline
	} else {
		pos = strings.TrimPrefix(pos, "[")
		pos = strings.TrimSuffix(pos, "]")
		point, _, err := geo.Coord3857FromString(pos)
		if err != nil {
			s.writeLog(functionName, fmt.Sprintf("Error parsing position: %v", err), "ERROR")
			return marker, err
		}
		marker.Position = point
	}

	// alpha
	alpha, err := strconv.ParseFloat(data[12], 32)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing alpha: %v", err), "WARN")
		alpha = 1.0
	}
	marker.Alpha = float32(alpha)

	// brush
	marker.Brush = data[13]

	marker.Time = time.Now()

	marker.IsDeleted = false

	return marker, nil
}

// ParseMarkerMove parses marker move data and returns a MarkerState model
func (s *Parser) ParseMarkerMove(data []string) (model.MarkerState, error) {
	functionName := ":NEW:MARKER:STATE:"
	var markerState model.MarkerState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	markerState.MissionID = s.ctx.GetMission().ID

	// markerName - look up marker ID from cache
	markerName := data[0]
	markerID, ok := s.deps.MarkerCache.Get(markerName)
	if !ok {
		return markerState, fmt.Errorf("marker %s not found in cache", markerName)
	}
	markerState.MarkerID = markerID

	// frameNo
	frameStr := data[1]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return markerState, err
	}
	markerState.CaptureFrame = uint(capframe)

	// position
	pos := data[2]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, _, err := geo.Coord3857FromString(pos)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing position: %v", err), "ERROR")
		return markerState, err
	}
	markerState.Position = point

	// direction
	dir, err := strconv.ParseFloat(data[3], 32)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing direction: %v", err), "WARN")
		dir = 0
	}
	markerState.Direction = float32(dir)

	// alpha
	alpha, err := strconv.ParseFloat(data[4], 32)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing alpha: %v", err), "WARN")
		alpha = 1.0
	}
	markerState.Alpha = float32(alpha)

	markerState.Time = time.Now()

	return markerState, nil
}

// ParseMarkerDelete parses marker delete data and returns the marker name and frame number
func (s *Parser) ParseMarkerDelete(data []string) (string, uint, error) {
	functionName := ":DELETE:MARKER:"

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	markerName := data[0]

	// frameNo
	frameStr := data[1]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf("Error parsing capture frame: %v", err), "ERROR")
		return markerName, 0, err
	}

	return markerName, uint(capframe), nil
}
