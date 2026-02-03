package handlers

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

// Dependencies holds all dependencies needed by handlers
type Dependencies struct {
	DB               *gorm.DB
	EntityCache      *cache.EntityCache
	MarkerCache      *cache.MarkerCache
	LogManager       *logging.SlogManager
	ExtensionName    string
	AddonVersion     string
	ExtensionVersion string
}

// Service provides handler methods for processing game data
type Service struct {
	deps         Dependencies
	ctx          *MissionContext
	writeLogFunc func(functionName, data, level string)
	backend      storage.Backend
}

// NewService creates a new handler service
func NewService(deps Dependencies, ctx *MissionContext) *Service {
	s := &Service{
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
func (s *Service) GetMissionContext() *MissionContext {
	return s.ctx
}

// SetBackend sets the storage backend for mission start/end handling
func (s *Service) SetBackend(b storage.Backend) {
	s.backend = b
}

func (s *Service) writeLog(functionName, data, level string) {
	s.writeLogFunc(functionName, data, level)
}

// LogNewMission logs a new mission to the database
func (s *Service) LogNewMission(data []string) error {
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

// LogNewSoldier parses soldier data and returns a Soldier model
func (s *Service) LogNewSoldier(data []string) (model.Soldier, error) {
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

	ocapID, err := strconv.ParseUint(data[1], 10, 64)
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

// LogSoldierState parses soldier state data and returns a SoldierState model
func (s *Service) LogSoldierState(data []string) (model.SoldierState, error) {
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
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
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

// LogNewVehicle parses vehicle data and returns a Vehicle model
func (s *Service) LogNewVehicle(data []string) (model.Vehicle, error) {
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
	ocapID, err := strconv.ParseUint(data[1], 10, 64)
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

// LogVehicleState parses vehicle state data and returns a VehicleState model
func (s *Service) LogVehicleState(data []string) (model.VehicleState, error) {
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
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
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

// LogFiredEvent parses fired event data and returns a FiredEvent model
func (s *Service) LogFiredEvent(data []string) (model.FiredEvent, error) {
	functionName := ":FIRED:"
	var firedEvent model.FiredEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	firedEvent.MissionID = s.ctx.GetMission().ID

	// get frame
	frameStr := data[1]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting capture frame to int: %s`, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.CaptureFrame = uint(capframe)

	// parse data in array
	ocapID, err := strconv.ParseUint(data[0], 10, 64)
	if err != nil {
		s.writeLog(functionName, fmt.Sprintf(`Error converting ocapID to uint: %v`, err), "ERROR")
		return firedEvent, err
	}

	// try and find soldier in DB to associate
	soldier, ok := s.deps.EntityCache.GetSoldier(uint16(ocapID))
	if !ok {
		return firedEvent, fmt.Errorf("soldier %d not found in cache", ocapID)
	}
	firedEvent.SoldierObjectID = soldier.ObjectID

	firedEvent.Time = time.Now()

	// parse BULLET END POS from an arma string
	endpos := data[2]
	endpoint, endelev, err := geo.Coord3857FromString(endpos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		s.writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", jsonData, err), "ERROR")
		return firedEvent, err
	}
	firedEvent.EndPosition = endpoint
	firedEvent.EndElevationASL = float32(endelev)

	// parse BULLET START POS from an arma string
	startpos := data[3]
	startpoint, startelev, err := geo.Coord3857FromString(startpos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		s.writeLog(functionName, fmt.Sprintf("Error converting position to Point:\n%s\n%v", jsonData, err), "ERROR")
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

// LogProjectileEvent parses projectile event data and returns a ProjectileEvent model
func (s *Service) LogProjectileEvent(data []string) (model.ProjectileEvent, error) {
	var projectileEvent model.ProjectileEvent
	logger := s.deps.LogManager.Logger()

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	projectileEvent.MissionID = s.ctx.GetMission().ID

	logger.Debug("Projectile data", "data", data[0])

	var rawJsonData map[string]interface{}
	err := json.Unmarshal([]byte(data[0]), &rawJsonData)
	if err != nil {
		return projectileEvent, fmt.Errorf(`error unmarshalling json data: %v`, err)
	}

	logger.Debug("Processing time and frame")
	firedTime := rawJsonData["firedTime"].(string)
	firedTimeInt, err := strconv.ParseInt(firedTime, 10, 64)
	if err != nil {
		return projectileEvent, fmt.Errorf(`error converting firedTime to int: %v`, err)
	}
	projectileEvent.Time = time.Unix(0, firedTimeInt)
	projectileEvent.CaptureFrame = uint(rawJsonData["firedFrame"].(float64))

	logger.Debug("Processing soldierFired")
	soldierFired, ok := s.deps.EntityCache.GetSoldier(uint16(rawJsonData["firerID"].(float64)))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d not found in cache", uint16(rawJsonData["firerID"].(float64)))
	}
	projectileEvent.FirerObjectID = soldierFired.ObjectID

	logger.Debug("Processing actualFirer")
	actualFirer, ok := s.deps.EntityCache.GetSoldier(uint16(rawJsonData["remoteControllerID"].(float64)))
	if !ok {
		return projectileEvent, fmt.Errorf("soldier %d not found in cache", uint16(rawJsonData["remoteControllerID"].(float64)))
	}
	projectileEvent.ActualFirerObjectID = actualFirer.ObjectID

	logger.Debug("Processing vehicleID")
	vehicleID := rawJsonData["vehicleID"].(float64)
	vehicle, ok := s.deps.EntityCache.GetVehicle(uint16(vehicleID))
	if ok {
		projectileEvent.VehicleObjectID = sql.NullInt32{
			Int32: int32(vehicle.ObjectID),
			Valid: true,
		}
	} else {
		projectileEvent.VehicleObjectID = sql.NullInt32{
			Int32: 0,
			Valid: false,
		}
	}

	// for Positions parsing, we need to create a Linestring with XYZM dimensions
	logger.Debug("Projectile positions", "positions", rawJsonData["positions"])
	positionSequence := []float64{}
	for _, v := range rawJsonData["positions"].([]interface{}) {
		posArr := v.([]interface{})

		logger.Debug("Projectile posArr", "posArr", posArr)

		// process time as posArr[0]
		unixTimeNano := posArr[0].(string)
		unixTimeNanoFloat, err := strconv.ParseFloat(unixTimeNano, 64)
		if err != nil {
			jsonData, _ := json.Marshal(posArr)
			logger.Error("Error converting timestamp to float64", "error", err, "json", string(jsonData))
			return projectileEvent, err
		}

		// process actual position xyz as posArr[2]
		pos := posArr[2].(string)
		point, _, err := geo.Coord3857FromString(pos)
		if err != nil {
			jsonData, _ := json.Marshal(posArr)
			logger.Error("Error converting position to Point", "error", err, "json", string(jsonData))
			return projectileEvent, err
		}
		coords, _ := point.Coordinates()

		positionSequence = append(
			positionSequence,
			coords.XY.X,
			coords.XY.Y,
			coords.Z,
			unixTimeNanoFloat,
		)
	}

	// create the linestring
	posSeq := geom.NewSequence(positionSequence, geom.DimXYZM)
	ls, err := geom.NewLineString(posSeq)
	if err != nil {
		jsonData, _ := json.Marshal(posSeq)
		logger.Error("Error creating linestring", "error", err, "json", string(jsonData))
		return projectileEvent, err
	}

	logger.Debug("Created linestring",
		"sequence", posSeq,
		"linestring", ls,
		"wkt", ls.AsText(),
		"wkb", ls.AsBinary())

	projectileEvent.Positions = ls.AsGeometry()

	logger.Debug("Processing hit events")
	// hit events
	projectileEvent.HitSoldiers = []model.ProjectileHitsSoldier{}
	projectileEvent.HitVehicles = []model.ProjectileHitsVehicle{}
	for _, event := range rawJsonData["hitParts"].([]interface{}) {
		eventArr := event.([]interface{})

		logger.Debug("Processing hit event", "eventArr", eventArr)

		// [1] is []string containing hit components
		hitComponents := []string{}
		for _, v := range eventArr[1].([]interface{}) {
			hitComponents = append(hitComponents, v.(string))
		}

		// [2] is string with positionASL
		hitPos := eventArr[2].(string)
		hitPoint, _, err := geo.Coord3857FromString(hitPos)
		if err != nil {
			jsonData, _ := json.Marshal(eventArr)
			logger.Error("Error converting hit position to Point", "error", err, "json", string(jsonData))
			return projectileEvent, err
		}

		// [3] is uint capture frame
		hitFrame := eventArr[3].(float64)

		// marshal hit components to json array
		hitComponentsJSON, err := json.Marshal(hitComponents)
		if err != nil {
			logger.Error("Error marshalling hit components to json", "error", err)
			return projectileEvent, err
		}

		logger.Debug("Processed hit components", "hitComponents", hitComponents)

		// [0] is the hit entity ocap id
		hitEntityID := eventArr[0].(float64)
		hitEntity, ok := s.deps.EntityCache.GetSoldier(uint16(hitEntityID))
		if ok {
			projectileEvent.HitSoldiers = append(
				projectileEvent.HitSoldiers,
				model.ProjectileHitsSoldier{
					SoldierObjectID: hitEntity.ObjectID,
					ComponentsHit: hitComponentsJSON,
					CaptureFrame:  uint(hitFrame),
					Position:      hitPoint,
				},
			)
		} else {
			hitVehicle, ok := s.deps.EntityCache.GetVehicle(uint16(hitEntityID))
			if ok {
				projectileEvent.HitVehicles = append(
					projectileEvent.HitVehicles,
					model.ProjectileHitsVehicle{
						VehicleObjectID: hitVehicle.ObjectID,
						ComponentsHit: hitComponentsJSON,
						CaptureFrame:  uint(hitFrame),
						Position:      hitPoint,
					},
				)
			} else {
				logger.Warn("Hit entity not found in cache", "hitEntityID", uint16(hitEntityID))
			}
		}
	}

	logger.Debug("Processing other properties")

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

// LogGeneralEvent parses general event data and returns a GeneralEvent model
func (s *Service) LogGeneralEvent(data []string) (model.GeneralEvent, error) {
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

// LogHitEvent parses hit event data and returns a HitEvent model
func (s *Service) LogHitEvent(data []string) (model.HitEvent, error) {
	var hitEvent model.HitEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	frameStr := data[0]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting capture frame to int: %s`, err)
	}

	hitEvent.CaptureFrame = uint(capframe)
	hitEvent.Mission = *s.ctx.GetMission()

	hitEvent.Time = time.Now()

	// parse data in array
	victimObjectID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting victim ocap id to uint: %v`, err)
	}

	// Set victim ObjectID - check if soldier or vehicle
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(victimObjectID)); ok {
		hitEvent.VictimSoldierObjectID = sql.NullInt32{Int32: int32(victimObjectID), Valid: true}
	} else if _, ok := s.deps.EntityCache.GetVehicle(uint16(victimObjectID)); ok {
		hitEvent.VictimVehicleObjectID = sql.NullInt32{Int32: int32(victimObjectID), Valid: true}
	} else {
		return hitEvent, fmt.Errorf(`victim ocap id not found in cache: %d`, victimObjectID)
	}

	// parse shooter ObjectID
	shooterObjectID, err := strconv.ParseUint(data[2], 10, 64)
	if err != nil {
		return hitEvent, fmt.Errorf(`error converting shooter ocap id to uint: %v`, err)
	}

	// Set shooter ObjectID - check if soldier or vehicle
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(shooterObjectID)); ok {
		hitEvent.ShooterSoldierObjectID = sql.NullInt32{Int32: int32(shooterObjectID), Valid: true}
	} else if _, ok := s.deps.EntityCache.GetVehicle(uint16(shooterObjectID)); ok {
		hitEvent.ShooterVehicleObjectID = sql.NullInt32{Int32: int32(shooterObjectID), Valid: true}
	} else {
		return hitEvent, fmt.Errorf(`shooter ocap id not found in cache: %d`, shooterObjectID)
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

// LogKillEvent parses kill event data and returns a KillEvent model
func (s *Service) LogKillEvent(data []string) (model.KillEvent, error) {
	var killEvent model.KillEvent

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
	victimObjectID, err := strconv.ParseUint(data[1], 10, 64)
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
	killerObjectID, err := strconv.ParseUint(data[2], 10, 64)
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

// LogChatEvent parses chat event data and returns a ChatEvent model
func (s *Service) LogChatEvent(data []string) (model.ChatEvent, error) {
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
	senderObjectID, err := strconv.ParseInt(data[1], 10, 64)
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
	channelInt, err := strconv.ParseInt(data[2], 10, 64)
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

// LogRadioEvent parses radio event data and returns a RadioEvent model
func (s *Service) LogRadioEvent(data []string) (model.RadioEvent, error) {
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
	senderObjectID, err := strconv.ParseInt(data[1], 10, 64)
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

// LogFpsEvent parses FPS event data and returns a ServerFpsEvent model
func (s *Service) LogFpsEvent(data []string) (model.ServerFpsEvent, error) {
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

// LogTimeState parses time state data and returns a TimeState model
// Args: [frameNo, systemTimeUTC, missionDateTime, timeMultiplier, missionTime]
func (s *Service) LogTimeState(data []string) (model.TimeState, error) {
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

// LogAce3DeathEvent parses ACE3 death event data and returns an Ace3DeathEvent model
func (s *Service) LogAce3DeathEvent(data []string) (model.Ace3DeathEvent, error) {
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
	victimObjectID, err := strconv.ParseUint(data[1], 10, 64)
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
	lastDamageSourceID, err := strconv.ParseInt(data[3], 10, 64)
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

// LogAce3UnconsciousEvent parses ACE3 unconscious event data and returns an Ace3UnconsciousEvent model
func (s *Service) LogAce3UnconsciousEvent(data []string) (model.Ace3UnconsciousEvent, error) {
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
	ocapID, err := strconv.ParseUint(data[1], 10, 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting ocap id to uint: %v`, err)
	}

	// Set soldier ObjectID
	if _, ok := s.deps.EntityCache.GetSoldier(uint16(ocapID)); !ok {
		return unconsciousEvent, fmt.Errorf("soldier %d not found in cache", ocapID)
	}
	unconsciousEvent.SoldierObjectID = uint16(ocapID)

	isAwake, err := strconv.ParseBool(data[2])
	if err != nil {
		return unconsciousEvent, fmt.Errorf(`error converting isAwake to bool: %v`, err)
	}
	unconsciousEvent.IsAwake = isAwake

	return unconsciousEvent, nil
}

// LogMarkerCreate parses marker create data and returns a Marker model
func (s *Service) LogMarkerCreate(data []string) (model.Marker, error) {
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

// LogMarkerMove parses marker move data and returns a MarkerState model
func (s *Service) LogMarkerMove(data []string) (model.MarkerState, error) {
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

// LogMarkerDelete parses marker delete data and returns the marker name and frame number
func (s *Service) LogMarkerDelete(data []string) (string, uint, error) {
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
