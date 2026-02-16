package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseProjectileEvent parses projectile event data into a ProjectileEvent.
// Hit parts are returned as HitPart for the worker to classify as soldier/vehicle.
func (p *Parser) ParseProjectileEvent(data []string) (ProjectileEvent, error) {
	var result ProjectileEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	if len(data) < 20 {
		return result, fmt.Errorf("insufficient data fields: got %d, need 20", len(data))
	}

	// [0] firedFrame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing firedFrame: %v", err)
	}
	result.CaptureFrame = uint(capframe)

	// [2] firerID - set directly
	firerID, err := parseUintFromFloat(data[2])
	if err != nil {
		return result, fmt.Errorf("error parsing firerID: %v", err)
	}
	result.FirerObjectID = uint16(firerID)

	// [3] vehicleID (-1 if not in vehicle)
	vehicleID, err := parseIntFromFloat(data[3])
	if err != nil {
		return result, fmt.Errorf("error parsing vehicleID: %v", err)
	}
	if vehicleID >= 0 {
		ptr := uint16(vehicleID)
		result.VehicleObjectID = &ptr
	}

	// [6-13] weapon info
	result.WeaponDisplay = data[7]
	result.MuzzleDisplay = data[9]
	result.MagazineDisplay = data[11]

	// [14] positions - SQF array "[[tickTime,frameNo,"x,y,z"],...]"
	var positions [][]any
	if err := json.Unmarshal([]byte(data[14]), &positions); err != nil {
		return result, fmt.Errorf("error parsing positions: %v", err)
	}

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

		pos3d, err := geo.Position3DFromString(posStr)
		if err != nil {
			p.logger.Warn("Error converting position to Point", "error", err, "pos", posStr)
			continue
		}

		result.Trajectory = append(result.Trajectory, core.TrajectoryPoint{
			Position: pos3d,
			Frame:    uint(frameNo),
		})
	}

	// [16] hitParts - parse into HitPart for worker classification
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
			hitPos3d, err := geo.Position3DFromString(hitPosStr)
			if err != nil {
				p.logger.Warn("Error converting hit position", "error", err)
				continue
			}

			hitFrame, ok := eventArr[3].(float64)
			if !ok {
				continue
			}

			result.HitParts = append(result.HitParts, HitPart{
				EntityID:      uint16(hitEntityID),
				ComponentsHit: hitComponents,
				CaptureFrame:  uint(hitFrame),
				Position:      hitPos3d,
			})
		}
	}

	// [17] sim - simulation type
	result.SimulationType = data[17]

	// [19] magazineIcon
	result.MagazineIcon = data[19]

	return result, nil
}

// ParseGeneralEvent parses general event data and returns a core GeneralEvent
func (p *Parser) ParseGeneralEvent(data []string) (core.GeneralEvent, error) {
	var thisEvent core.GeneralEvent

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

// ParseKillEvent parses kill event data into a KillEvent.
// Raw victim/killer IDs are returned for the worker to classify as soldier vs vehicle.
func (p *Parser) ParseKillEvent(data []string) (KillEvent, error) {
	var result KillEvent

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

	result.Time = time.Now()
	result.CaptureFrame = uint(capframe)

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
	result.WeaponVehicle, result.WeaponName, result.WeaponMagazine = util.ParseSQFStringArray(rawWeapon)
	result.EventText = util.FormatWeaponText(result.WeaponVehicle, result.WeaponName, result.WeaponMagazine)

	// get event distance
	distance, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return result, fmt.Errorf("error converting distance to float: %w", err)
	}
	result.Distance = float32(distance)

	return result, nil
}

// ParseFpsEvent parses FPS event data and returns a core ServerFpsEvent
func (p *Parser) ParseFpsEvent(data []string) (core.ServerFpsEvent, error) {
	var fpsEvent core.ServerFpsEvent

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

// ParseTimeState parses time state data and returns a core TimeState
// Args: [frameNo, systemTimeUTC, missionDateTime, timeMultiplier, missionTime]
func (p *Parser) ParseTimeState(data []string) (core.TimeState, error) {
	var timeState core.TimeState

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
