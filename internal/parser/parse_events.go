package parser

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"

	geom "github.com/peterstace/simplefeatures/geom"
)

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
