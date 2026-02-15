package parser

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"

	geom "github.com/peterstace/simplefeatures/geom"
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

// Parser provides pure []string -> model struct conversion.
// It has zero external dependencies beyond a logger.
type Parser struct {
	logger  *slog.Logger
	mission atomic.Pointer[model.Mission]

	// Static config set at creation time
	addonVersion     string
	extensionVersion string
}

// NewParser creates a new parser with only a logger dependency
func NewParser(logger *slog.Logger, addonVersion, extensionVersion string) *Parser {
	p := &Parser{
		logger:           logger,
		addonVersion:     addonVersion,
		extensionVersion: extensionVersion,
	}
	return p
}

// SetMission sets the current mission for MissionID lookups
func (p *Parser) SetMission(m *model.Mission) {
	p.mission.Store(m)
}

func (p *Parser) getMissionID() uint {
	m := p.mission.Load()
	if m == nil {
		return 0
	}
	return m.ID
}

// ParseMission parses mission and world data from raw args.
// Returns parsed mission + world. NO DB operations, NO cache resets, NO callbacks.
func (p *Parser) ParseMission(data []string) (model.Mission, model.World, error) {
	var mission model.Mission
	var world model.World

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// unmarshal data[0] -> world
	err := json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		return mission, world, fmt.Errorf("error unmarshalling world data: %w", err)
	}

	// preprocess the world 'location' to geopoint
	worldLocation, err := geo.Coords3857From4326(
		float64(world.Longitude),
		float64(world.Latitude),
	)
	if err != nil {
		return mission, world, fmt.Errorf("error converting world location to geopoint: %w", err)
	}
	world.Location = worldLocation

	// unmarshal data[1] -> mission (via temp map for addons extraction)
	missionTemp := map[string]any{}
	if err = json.Unmarshal([]byte(data[1]), &missionTemp); err != nil {
		return mission, world, fmt.Errorf("error unmarshalling mission data: %w", err)
	}

	// extract addons (without DB lookup - just parse)
	addons := []model.Addon{}
	for _, addon := range missionTemp["addons"].([]any) {
		thisAddon := model.Addon{
			Name: addon.([]any)[0].(string),
		}
		if reflect.TypeOf(addon.([]any)[1]).Kind() == reflect.Float64 {
			thisAddon.WorkshopID = strconv.Itoa(int(addon.([]any)[1].(float64)))
		} else {
			thisAddon.WorkshopID = addon.([]any)[1].(string)
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
	playableSlotsJSON := missionTemp["playableSlots"].([]any)
	mission.PlayableSlots.East = uint8(playableSlotsJSON[0].(float64))
	mission.PlayableSlots.West = uint8(playableSlotsJSON[1].(float64))
	mission.PlayableSlots.Independent = uint8(playableSlotsJSON[2].(float64))
	mission.PlayableSlots.Civilian = uint8(playableSlotsJSON[3].(float64))
	mission.PlayableSlots.Logic = uint8(playableSlotsJSON[4].(float64))

	// sideFriendly
	sideFriendlyJSON := missionTemp["sideFriendly"].([]any)
	mission.SideFriendly.EastWest = sideFriendlyJSON[0].(bool)
	mission.SideFriendly.EastIndependent = sideFriendlyJSON[1].(bool)
	mission.SideFriendly.WestIndependent = sideFriendlyJSON[2].(bool)

	// received at extension init and saved to local memory
	mission.AddonVersion = p.addonVersion
	mission.ExtensionVersion = p.extensionVersion

	p.logger.Debug("Parsed mission data",
		"missionName", mission.MissionName,
		"worldName", world.WorldName)

	return mission, world, nil
}

// ParseSoldier parses soldier data and returns a Soldier model
func (p *Parser) ParseSoldier(data []string) (model.Soldier, error) {
	var soldier model.Soldier

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return soldier, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	soldier.MissionID = p.getMissionID()
	soldier.JoinFrame = uint(capframe)
	soldier.JoinTime = time.Now()

	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return soldier, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	soldier.ObjectID = uint16(ocapID)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, err = strconv.ParseBool(data[5])
	if err != nil {
		return soldier, fmt.Errorf("error converting isPlayer to bool: %w", err)
	}
	soldier.RoleDescription = data[6]
	soldier.ClassName = data[7]
	soldier.DisplayName = data[8]
	soldier.PlayerUID = data[9]

	// marshal squadparams
	err = json.Unmarshal([]byte(data[10]), &soldier.SquadParams)
	if err != nil {
		return soldier, fmt.Errorf("error unmarshalling squadParams: %w", err)
	}

	return soldier, nil
}

// ParseSoldierState parses soldier state data and returns a SoldierState model.
// Sets SoldierObjectID directly from the parsed ocapID (no cache lookup).
// If groupID/side fields are not in the data (len < 17), they are left empty
// for the worker layer to fill from cache.
func (p *Parser) ParseSoldierState(data []string) (model.SoldierState, error) {
	var soldierState model.SoldierState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	soldierState.MissionID = p.getMissionID()

	frameStr := data[8]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return soldierState, fmt.Errorf("error converting capture frame to int: %w", err)
	}
	soldierState.CaptureFrame = uint(capframe)

	// parse ocapID and set directly (worker validates against cache)
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		return soldierState, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	soldierState.SoldierObjectID = uint16(ocapID)

	soldierState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := geo.Coord3857FromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		p.logger.Error("Error converting position to Point", "data", string(jsonData), "error", err)
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
		return soldierState, fmt.Errorf("error converting isPlayer to bool: %w", err)
	}
	soldierState.IsPlayer = isPlayer
	// current role
	soldierState.CurrentRole = data[7]

	// parse ace3/medical data
	hasStableVitals, _ := strconv.ParseBool(data[9])
	soldierState.HasStableVitals = hasStableVitals
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
			p.logger.Warn("Unexpected score count", "expected", 6, "got", len(scoresArr), "data", scoresStr)
		}
	}

	// seat in vehicle
	soldierState.VehicleRole = data[12]

	// InVehicleObjectID, if -1 not in a vehicle
	inVehicleID, _ := strconv.Atoi(data[13])
	if inVehicleID == -1 {
		soldierState.InVehicleObjectID = sql.NullInt32{Int32: 0, Valid: false}
	} else {
		soldierState.InVehicleObjectID = sql.NullInt32{Int32: int32(inVehicleID), Valid: true}
	}

	// stance
	soldierState.Stance = data[14]

	// groupID and side (added by addon PR#55, backward compatible)
	// If not present, leave empty - worker fills from cached soldier
	if len(data) >= 17 {
		soldierState.GroupID = data[15]
		soldierState.Side = data[16]
	}

	return soldierState, nil
}

// ParseVehicle parses vehicle data and returns a Vehicle model
func (p *Parser) ParseVehicle(data []string) (model.Vehicle, error) {
	var vehicle model.Vehicle

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return vehicle, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	vehicle.JoinTime = time.Now()

	vehicle.MissionID = p.getMissionID()
	vehicle.JoinFrame = uint(capframe)
	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return vehicle, fmt.Errorf("error converting ocapID to uint: %w", err)
	}
	vehicle.ObjectID = uint16(ocapID)
	vehicle.OcapType = data[2]
	vehicle.DisplayName = data[3]
	vehicle.ClassName = data[4]
	vehicle.Customization = data[5]

	return vehicle, nil
}

// ParseVehicleState parses vehicle state data and returns a VehicleState model.
// Sets VehicleObjectID directly from the parsed ocapID (no cache lookup).
func (p *Parser) ParseVehicleState(data []string) (model.VehicleState, error) {
	var vehicleState model.VehicleState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	vehicleState.MissionID = p.getMissionID()

	// get frame
	capframe, err := strconv.ParseFloat(data[5], 64)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting capture frame to int: %w", err)
	}
	vehicleState.CaptureFrame = uint(capframe)

	// parse ocapID and set directly (worker validates against cache)
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	vehicleState.VehicleObjectID = uint16(ocapID)

	vehicleState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := geo.Coord3857FromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		p.logger.Error("Error converting position to Point", "data", string(jsonData), "error", err)
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
	// parse crew
	vehicleState.Crew = data[4]

	// fuel
	fuel, err := strconv.ParseFloat(data[6], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting fuel to float: %w", err)
	}
	vehicleState.Fuel = float32(fuel)

	// damage
	damage, err := strconv.ParseFloat(data[7], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting damage to float: %w", err)
	}
	vehicleState.Damage = float32(damage)

	// isEngineOn
	isEngineOn, err := strconv.ParseBool(data[8])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting isEngineOn to bool: %w", err)
	}
	vehicleState.EngineOn = isEngineOn

	// locked
	locked, err := strconv.ParseBool(data[9])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting locked to bool: %w", err)
	}
	vehicleState.Locked = locked

	vehicleState.Side = data[10]
	vehicleState.VectorDir = data[11]
	vehicleState.VectorUp = data[12]

	turretAzimuth, err := strconv.ParseFloat(data[13], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting turretAzimuth to float: %w", err)
	}
	vehicleState.TurretAzimuth = float32(turretAzimuth)

	turretElevation, err := strconv.ParseFloat(data[14], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting turretElevation to float: %w", err)
	}
	vehicleState.TurretElevation = float32(turretElevation)

	return vehicleState, nil
}

// ParseProjectileEvent parses projectile event data and returns a ParsedProjectileEvent.
// FirerObjectID, VehicleObjectID, and ActualFirerObjectID are set directly from parsed IDs.
// Hit parts are returned as RawHitPart for the worker to classify as soldier/vehicle.
func (p *Parser) ParseProjectileEvent(data []string) (ParsedProjectileEvent, error) {
	var result ParsedProjectileEvent
	var projectileEvent model.ProjectileEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	if len(data) < 20 {
		return result, fmt.Errorf("insufficient data fields: got %d, need 20", len(data))
	}

	projectileEvent.MissionID = p.getMissionID()

	// [0] firedFrame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing firedFrame: %v", err)
	}
	projectileEvent.CaptureFrame = uint(capframe)

	// [1] firedTime
	projectileEvent.Time = time.Now()

	// [2] firerID - set directly
	firerID, err := parseUintFromFloat(data[2])
	if err != nil {
		return result, fmt.Errorf("error parsing firerID: %v", err)
	}
	projectileEvent.FirerObjectID = uint16(firerID)

	// [3] vehicleID (-1 if not in vehicle)
	vehicleID, err := parseIntFromFloat(data[3])
	if err != nil {
		return result, fmt.Errorf("error parsing vehicleID: %v", err)
	}
	if vehicleID >= 0 {
		projectileEvent.VehicleObjectID = sql.NullInt32{Int32: int32(vehicleID), Valid: true}
	}

	// [4] vehicleRole
	projectileEvent.VehicleRole = data[4]

	// [5] remoteControllerID - set directly
	remoteControllerID, err := parseUintFromFloat(data[5])
	if err != nil {
		return result, fmt.Errorf("error parsing remoteControllerID: %v", err)
	}
	projectileEvent.ActualFirerObjectID = uint16(remoteControllerID)

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
	var positions [][]any
	if err := json.Unmarshal([]byte(data[14]), &positions); err != nil {
		return result, fmt.Errorf("error parsing positions: %v", err)
	}

	positionSequence := []float64{}
	for _, posArr := range positions {
		if len(posArr) < 3 {
			continue
		}

		frameNo, ok := posArr[1].(float64)
		if !ok {
			p.logger.Warn("Invalid frameNo in position", "value", posArr[1])
			continue
		}

		posStr, ok := posArr[2].(string)
		if !ok {
			p.logger.Warn("Invalid position string", "value", posArr[2])
			continue
		}

		point, _, err := geo.Coord3857FromString(posStr)
		if err != nil {
			p.logger.Warn("Error converting position to Point", "error", err, "pos", posStr)
			continue
		}
		coords, _ := point.Coordinates()

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

	// [16] hitParts - parse into RawHitPart for worker classification
	var hitParts [][]any
	if err := json.Unmarshal([]byte(data[16]), &hitParts); err != nil {
		p.logger.Warn("Error parsing hitParts", "error", err, "data", data[16])
	} else {
		for _, eventArr := range hitParts {
			if len(eventArr) < 4 {
				continue
			}

			hitEntityID, ok := eventArr[0].(float64)
			if !ok {
				continue
			}

			hitComponents := []string{}
			switch comp := eventArr[1].(type) {
			case string:
				hitComponents = append(hitComponents, comp)
			case []any:
				for _, v := range comp {
					if s, ok := v.(string); ok {
						hitComponents = append(hitComponents, s)
					}
				}
			}

			hitPosStr, ok := eventArr[2].(string)
			if !ok {
				continue
			}
			hitPoint, _, err := geo.Coord3857FromString(hitPosStr)
			if err != nil {
				p.logger.Warn("Error converting hit position", "error", err)
				continue
			}

			hitFrame, ok := eventArr[3].(float64)
			if !ok {
				continue
			}

			hitComponentsJSON, _ := json.Marshal(hitComponents)

			result.HitParts = append(result.HitParts, RawHitPart{
				EntityID:      uint16(hitEntityID),
				ComponentsHit: hitComponentsJSON,
				CaptureFrame:  uint(hitFrame),
				Position:      hitPoint,
			})
		}
	}

	// [17] sim - simulation type
	projectileEvent.SimulationType = data[17]

	// [18] isSub - is submunition
	isSub, err := strconv.ParseBool(data[18])
	if err == nil {
		projectileEvent.IsSubmunition = isSub
	}

	// [19] magazineIcon
	projectileEvent.MagazineIcon = data[19]

	// Initialize empty hit slices on the event
	projectileEvent.HitSoldiers = []model.ProjectileHitsSoldier{}
	projectileEvent.HitVehicles = []model.ProjectileHitsVehicle{}

	result.Event = projectileEvent
	return result, nil
}

// ParseGeneralEvent parses general event data and returns a GeneralEvent model
func (p *Parser) ParseGeneralEvent(data []string) (model.GeneralEvent, error) {
	var thisEvent model.GeneralEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return thisEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	thisEvent.Time = time.Now()
	thisEvent.MissionID = p.getMissionID()
	thisEvent.CaptureFrame = uint(capframe)
	thisEvent.Name = data[1]
	thisEvent.Message = data[2]

	// get extra event data
	if len(data) > 3 {
		err = json.Unmarshal([]byte(data[3]), &thisEvent.ExtraData)
		if err != nil {
			return thisEvent, fmt.Errorf("error unmarshalling extra data: %w", err)
		}
	}

	return thisEvent, nil
}

// ParseKillEvent parses kill event data and returns a ParsedKillEvent.
// Raw victim/killer IDs are returned for the worker to classify as soldier vs vehicle.
func (p *Parser) ParseKillEvent(data []string) (ParsedKillEvent, error) {
	var result ParsedKillEvent

	// Save weapon array before FixEscapeQuotes
	rawWeapon := util.TrimQuotes(data[3])

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	result.Event.Time = time.Now()
	result.Event.CaptureFrame = uint(capframe)
	result.Event.MissionID = p.getMissionID()

	// parse victim ObjectID
	victimObjectID, err := parseUintFromFloat(data[1])
	if err != nil {
		return result, fmt.Errorf("error converting victim ocap id to uint: %w", err)
	}
	result.VictimID = uint16(victimObjectID)

	// parse killer ObjectID
	killerObjectID, err := parseUintFromFloat(data[2])
	if err != nil {
		return result, fmt.Errorf("error converting killer ocap id to uint: %w", err)
	}
	result.KillerID = uint16(killerObjectID)

	// get weapon info
	result.Event.WeaponVehicle, result.Event.WeaponName, result.Event.WeaponMagazine = util.ParseSQFStringArray(rawWeapon)
	result.Event.EventText = util.FormatWeaponText(result.Event.WeaponVehicle, result.Event.WeaponName, result.Event.WeaponMagazine)

	// get event distance
	distance, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return result, fmt.Errorf("error converting distance to float: %w", err)
	}
	result.Event.Distance = float32(distance)

	return result, nil
}

// ParseChatEvent parses chat event data and returns a ChatEvent model.
// SoldierObjectID is set directly (no cache validation - worker validates).
func (p *Parser) ParseChatEvent(data []string) (model.ChatEvent, error) {
	var chatEvent model.ChatEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return chatEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	chatEvent.Time = time.Now()
	chatEvent.CaptureFrame = uint(capframe)
	chatEvent.MissionID = p.getMissionID()

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		chatEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
	}

	// channel
	channelInt, err := parseIntFromFloat(data[2])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting channel to int: %w", err)
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

	chatEvent.FromName = data[3]
	chatEvent.SenderName = data[4]
	chatEvent.Message = data[5]
	chatEvent.PlayerUID = data[6]

	return chatEvent, nil
}

// ParseRadioEvent parses radio event data and returns a RadioEvent model.
// SoldierObjectID is set directly (no cache validation - worker validates).
func (p *Parser) ParseRadioEvent(data []string) (model.RadioEvent, error) {
	var radioEvent model.RadioEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return radioEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	radioEvent.Time = time.Now()
	radioEvent.CaptureFrame = uint(capframe)
	radioEvent.MissionID = p.getMissionID()

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		radioEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
	}

	radioEvent.Radio = data[2]
	radioEvent.RadioType = data[3]
	radioEvent.StartEnd = data[4]

	channelInt, err := parseIntFromFloat(data[5])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting channel to int: %w", err)
	}
	radioEvent.Channel = int8(channelInt)

	isAddtl, err := strconv.ParseBool(data[6])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting isAddtl to bool: %w", err)
	}
	radioEvent.IsAdditional = isAddtl

	freq, err := strconv.ParseFloat(data[7], 64)
	if err != nil {
		return radioEvent, fmt.Errorf("error converting freq to float: %w", err)
	}
	radioEvent.Frequency = float32(freq)

	radioEvent.Code = data[8]

	return radioEvent, nil
}

// ParseFpsEvent parses FPS event data and returns a ServerFpsEvent model
func (p *Parser) ParseFpsEvent(data []string) (model.ServerFpsEvent, error) {
	var fpsEvent model.ServerFpsEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return fpsEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	fpsEvent.CaptureFrame = uint(capframe)
	fpsEvent.Time = time.Now()
	fpsEvent.MissionID = p.getMissionID()

	fps, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return fpsEvent, fmt.Errorf("error converting fps to float: %w", err)
	}
	fpsEvent.FpsAverage = float32(fps)

	fpsMin, err := strconv.ParseFloat(data[2], 64)
	if err != nil {
		return fpsEvent, fmt.Errorf("error converting fpsMin to float: %w", err)
	}
	fpsEvent.FpsMin = float32(fpsMin)

	return fpsEvent, nil
}

// ParseTimeState parses time state data and returns a TimeState model
// Args: [frameNo, systemTimeUTC, missionDateTime, timeMultiplier, missionTime]
func (p *Parser) ParseTimeState(data []string) (model.TimeState, error) {
	var timeState model.TimeState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return timeState, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	timeState.CaptureFrame = uint(capframe)
	timeState.Time = time.Now()
	timeState.MissionID = p.getMissionID()

	timeState.SystemTimeUTC = data[1]
	timeState.MissionDate = data[2]

	timeMult, err := strconv.ParseFloat(data[3], 64)
	if err != nil {
		return timeState, fmt.Errorf("error converting timeMultiplier to float: %w", err)
	}
	timeState.TimeMultiplier = float32(timeMult)

	missionTime, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return timeState, fmt.Errorf("error converting missionTime to float: %w", err)
	}
	timeState.MissionTime = float32(missionTime)

	return timeState, nil
}

// ParseAce3DeathEvent parses ACE3 death event data and returns an Ace3DeathEvent model.
// SoldierObjectID and LastDamageSourceObjectID are set directly (no cache validation).
func (p *Parser) ParseAce3DeathEvent(data []string) (model.Ace3DeathEvent, error) {
	var deathEvent model.Ace3DeathEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return deathEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	deathEvent.Time = time.Now()
	deathEvent.CaptureFrame = uint(capframe)
	deathEvent.MissionID = p.getMissionID()

	// parse victim ObjectID - set directly
	victimObjectID, err := parseUintFromFloat(data[1])
	if err != nil {
		return deathEvent, fmt.Errorf("error converting victim ocap id to uint: %w", err)
	}
	deathEvent.SoldierObjectID = uint16(victimObjectID)

	deathEvent.Reason = data[2]

	// get last damage source id [3]
	lastDamageSourceID, err := parseIntFromFloat(data[3])
	if err != nil {
		return deathEvent, fmt.Errorf("error converting last damage source id to uint: %w", err)
	}

	if lastDamageSourceID > -1 {
		deathEvent.LastDamageSourceObjectID = sql.NullInt32{Int32: int32(lastDamageSourceID), Valid: true}
	}

	return deathEvent, nil
}

// ParseAce3UnconsciousEvent parses ACE3 unconscious event data and returns an Ace3UnconsciousEvent model.
// SoldierObjectID is set directly (no cache validation - worker validates).
func (p *Parser) ParseAce3UnconsciousEvent(data []string) (model.Ace3UnconsciousEvent, error) {
	var unconsciousEvent model.Ace3UnconsciousEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	unconsciousEvent.CaptureFrame = uint(capframe)
	unconsciousEvent.MissionID = p.getMissionID()

	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting ocap id to uint: %w", err)
	}
	unconsciousEvent.SoldierObjectID = uint16(ocapID)

	isUnconscious, err := strconv.ParseBool(data[2])
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting isUnconscious to bool: %w", err)
	}
	unconsciousEvent.IsUnconscious = isUnconscious

	return unconsciousEvent, nil
}

// ParseMarkerCreate parses marker create data and returns a Marker model
func (p *Parser) ParseMarkerCreate(data []string) (model.Marker, error) {
	var marker model.Marker

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	marker.MissionID = p.getMissionID()

	// markerName
	marker.MarkerName = data[0]

	// direction
	dir, err := strconv.ParseFloat(data[1], 32)
	if err != nil {
		return marker, fmt.Errorf("error parsing direction: %w", err)
	}
	marker.Direction = float32(dir)

	// type
	marker.MarkerType = data[2]

	// text
	marker.Text = data[3]

	// frameNo
	capframe, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return marker, fmt.Errorf("error parsing capture frame: %w", err)
	}
	marker.CaptureFrame = uint(capframe)

	// data[5] is -1, skip

	// ownerId
	ownerId, err := strconv.Atoi(data[6])
	if err != nil {
		p.logger.Warn("Error parsing ownerId", "error", err)
		ownerId = -1
	}
	marker.OwnerID = ownerId

	// color
	marker.Color = data[7]

	// size
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
			return marker, fmt.Errorf("error parsing polyline: %w", err)
		}
		marker.Polyline = polyline
	} else {
		pos = strings.TrimPrefix(pos, "[")
		pos = strings.TrimSuffix(pos, "]")
		point, _, err := geo.Coord3857FromString(pos)
		if err != nil {
			return marker, fmt.Errorf("error parsing position: %w", err)
		}
		marker.Position = point
	}

	// alpha
	alpha, err := strconv.ParseFloat(data[12], 32)
	if err != nil {
		p.logger.Warn("Error parsing alpha", "error", err)
		alpha = 1.0
	}
	marker.Alpha = float32(alpha)

	// brush
	marker.Brush = data[13]

	marker.Time = time.Now()
	marker.IsDeleted = false

	return marker, nil
}

// ParseMarkerMove parses marker move data and returns a ParsedMarkerMove.
// The MarkerName is returned for the worker to resolve to a MarkerID via MarkerCache.
func (p *Parser) ParseMarkerMove(data []string) (ParsedMarkerMove, error) {
	var result ParsedMarkerMove

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	result.State.MissionID = p.getMissionID()

	// markerName - return for worker to resolve
	result.MarkerName = data[0]

	// frameNo
	capframe, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing capture frame: %w", err)
	}
	result.State.CaptureFrame = uint(capframe)

	// position
	pos := data[2]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, _, err := geo.Coord3857FromString(pos)
	if err != nil {
		return result, fmt.Errorf("error parsing position: %w", err)
	}
	result.State.Position = point

	// direction
	dir, err := strconv.ParseFloat(data[3], 32)
	if err != nil {
		p.logger.Warn("Error parsing direction", "error", err)
		dir = 0
	}
	result.State.Direction = float32(dir)

	// alpha
	alpha, err := strconv.ParseFloat(data[4], 32)
	if err != nil {
		p.logger.Warn("Error parsing alpha", "error", err)
		alpha = 1.0
	}
	result.State.Alpha = float32(alpha)

	result.State.Time = time.Now()

	return result, nil
}

// ParseMarkerDelete parses marker delete data and returns the marker name and frame number
func (p *Parser) ParseMarkerDelete(data []string) (string, uint, error) {
	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	markerName := data[0]

	capframe, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return markerName, 0, fmt.Errorf("error parsing capture frame: %w", err)
	}

	return markerName, uint(capframe), nil
}
