package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/pkg/core"
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

// ParseTelemetryEvent parses a unified telemetry snapshot sent every ~10 seconds.
// Replaces the old :FPS: and :METRIC: commands.
//
// ArmA's callExtension flattens the top-level SQF array into 7 separate string args:
//
//	data[0] — Frame number (OCAP_recorder_captureFrameNo). Plain integer string.
//	           Example: "134"
//
//	data[1] — Server FPS: [diag_fps, diag_fpsmin].
//	           Example: "[43.3604,4.36681]"
//
//	data[2] — Per-side entity counts. Array[4] in order [east, west, independent, civilian].
//	           Each side: [[server_local], [remote]].
//	           Each locality: [units_total, units_alive, units_dead, groups_total,
//	                           vehicles_total, vehicles_weaponholder].
//	           Note: units_total ≈ units_alive (allUnits typically only contains living units;
//	           they may diverge with ACE unconscious units).
//	           Example: "[[[1,1,0,1,0,0],[0,0,0,0,0,0]],[[10,10,0,8,0,0],[0,0,0,0,0,0]],...]"
//
//	data[3] — Global entity counts: [units_alive, units_dead, groups_total, vehicles_total,
//	           vehicles_weaponholder, players_alive, players_dead, players_connected].
//	           Example: "[22,12,15,28,0,1,0,1]"
//
//	data[4] — Running scripts: [spawn, execVM, exec, execFSM, pfh].
//	           Indices 0–3 from diag_activeScripts; index 4 is CBA per-frame handler count.
//	           Example: "[28,4,0,4,2]"
//
//	data[5] — Weather snapshot (12 floats): [fog, overcast, rain, humidity, waves,
//	           windDir (0–360°), windStr, gusts, lightnings, moonIntensity,
//	           moonPhase (0=new, 0.5=full, 1=new), sunOrMoon (0=night, 1=day)].
//	           Example: "[0.2,0.25,0,0,0.1,160.864,0.25,0.175,0.003153,0.441581,0.421672,1]"
//
//	data[6] — Player network data. Variable-length array, one entry per connected human player
//	           (excludes headless clients). Each entry: [playerUID, playerName, avgPing,
//	           avgBandwidth, desync]. Empty when no players: "[]".
//	           Note: avgBandwidth may use scientific notation (e.g. 1.67772e+07).
//	           Example: "[["76561198000074241","info",100,28,0]]"
func (p *Parser) ParseTelemetryEvent(data []string) (core.TelemetryEvent, error) {
	var result core.TelemetryEvent

	if len(data) < 7 {
		return result, fmt.Errorf("telemetry: expected 7 args, got %d", len(data))
	}

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	result.Time = time.Now()

	// [0] captureFrameNo — plain integer string
	frameNo, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("telemetry: parse frameNo: %w", err)
	}
	result.CaptureFrame = uint(frameNo)

	// [1] Server FPS: [diag_fps, diag_fpsmin]
	var fps [2]float64
	if err := json.Unmarshal([]byte(data[1]), &fps); err != nil {
		return result, fmt.Errorf("telemetry: parse fps: %w", err)
	}
	result.FpsAverage = float32(fps[0])
	result.FpsMin = float32(fps[1])

	// [2] Per-side entity counts: [east, west, independent, civilian]
	//     Each side: [[server_local], [remote]]
	//     Each locality: [units_total, units_alive, units_dead, groups_total, vehicles_total, vehicles_weaponholder]
	var sides [4][2][6]float64
	if err := json.Unmarshal([]byte(data[2]), &sides); err != nil {
		return result, fmt.Errorf("telemetry: parse side data: %w", err)
	}
	for i, side := range sides {
		result.SideEntityCounts[i] = core.SideEntityCount{
			Local: core.EntityLocality{
				UnitsTotal: uint(side[0][0]), UnitsAlive: uint(side[0][1]),
				UnitsDead: uint(side[0][2]), Groups: uint(side[0][3]),
				Vehicles: uint(side[0][4]), WeaponHolders: uint(side[0][5]),
			},
			Remote: core.EntityLocality{
				UnitsTotal: uint(side[1][0]), UnitsAlive: uint(side[1][1]),
				UnitsDead: uint(side[1][2]), Groups: uint(side[1][3]),
				Vehicles: uint(side[1][4]), WeaponHolders: uint(side[1][5]),
			},
		}
	}

	// [3] Global entity counts:
	//     [units_alive, units_dead, groups_total, vehicles_total,
	//      vehicles_weaponholder, players_alive, players_dead, players_connected]
	var global [8]float64
	if err := json.Unmarshal([]byte(data[3]), &global); err != nil {
		return result, fmt.Errorf("telemetry: parse global counts: %w", err)
	}
	result.GlobalCounts = core.GlobalEntityCount{
		UnitsAlive: uint(global[0]), UnitsDead: uint(global[1]),
		Groups: uint(global[2]), Vehicles: uint(global[3]),
		WeaponHolders: uint(global[4]), PlayersAlive: uint(global[5]),
		PlayersDead: uint(global[6]), PlayersConnected: uint(global[7]),
	}

	// [4] Running scripts: [spawn, execVM, exec, execFSM, pfh]
	//     0–3 from diag_activeScripts; 4 is CBA per-frame handler count
	var scripts [5]float64
	if err := json.Unmarshal([]byte(data[4]), &scripts); err != nil {
		return result, fmt.Errorf("telemetry: parse scripts: %w", err)
	}
	result.Scripts = core.ScriptCounts{
		Spawn: uint(scripts[0]), ExecVM: uint(scripts[1]),
		Exec: uint(scripts[2]), ExecFSM: uint(scripts[3]),
		PFH: uint(scripts[4]),
	}

	// [5] Weather (12 floats):
	//     [fog, overcast, rain, humidity, waves, windDir, windStr, gusts,
	//      lightnings, moonIntensity, moonPhase, sunOrMoon]
	var weather [12]float64
	if err := json.Unmarshal([]byte(data[5]), &weather); err != nil {
		return result, fmt.Errorf("telemetry: parse weather: %w", err)
	}
	result.Weather = core.WeatherData{
		Fog: float32(weather[0]), Overcast: float32(weather[1]),
		Rain: float32(weather[2]), Humidity: float32(weather[3]),
		Waves: float32(weather[4]), WindDir: float32(weather[5]),
		WindStr: float32(weather[6]), Gusts: float32(weather[7]),
		Lightnings: float32(weather[8]), MoonIntensity: float32(weather[9]),
		MoonPhase: float32(weather[10]), SunOrMoon: float32(weather[11]),
	}

	// [6] Player network data: [[playerUID, playerName, avgPing, avgBandwidth, desync], ...]
	//     Variable length; empty "[]" when no players connected.
	//     Excludes headless clients. Bandwidth may use scientific notation.
	//     Unmarshal each entry as []any in one call (instead of 5 separate RawMessage unmarshals)
	//     for fewer allocations per player.
	var players [][]any
	if err := json.Unmarshal([]byte(data[6]), &players); err != nil {
		return result, fmt.Errorf("telemetry: parse players: %w", err)
	}
	for _, pArr := range players {
		if len(pArr) < 5 {
			continue
		}
		uid, ok := pArr[0].(string)
		if !ok {
			p.logger.Warn("telemetry: skip player, bad uid", "value", pArr[0])
			continue
		}
		name, ok := pArr[1].(string)
		if !ok {
			p.logger.Warn("telemetry: skip player, bad name", "value", pArr[1])
			continue
		}
		ping, ok := pArr[2].(float64)
		if !ok {
			p.logger.Warn("telemetry: skip player, bad ping", "value", pArr[2])
			continue
		}
		bw, ok := pArr[3].(float64)
		if !ok {
			p.logger.Warn("telemetry: skip player, bad bw", "value", pArr[3])
			continue
		}
		desync, ok := pArr[4].(float64)
		if !ok {
			p.logger.Warn("telemetry: skip player, bad desync", "value", pArr[4])
			continue
		}
		result.Players = append(result.Players, core.PlayerNetworkData{
			UID: uid, Name: name,
			Ping: float32(ping), BW: float32(bw), Desync: float32(desync),
		})
	}

	return result, nil
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
