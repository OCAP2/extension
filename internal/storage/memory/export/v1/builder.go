package v1

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// MissionData contains all the data needed to build an export
type MissionData struct {
	Mission   *core.Mission
	World     *core.World
	Soldiers  map[uint16]*SoldierRecord
	Vehicles  map[uint16]*VehicleRecord
	Markers   map[string]*MarkerRecord

	GeneralEvents    []core.GeneralEvent
	HitEvents        []core.HitEvent
	KillEvents       []core.KillEvent
	TimeStates       []core.TimeState
	ProjectileEvents []core.ProjectileEvent
}

// SoldierRecord groups a soldier with all its time-series data
type SoldierRecord struct {
	Soldier     core.Soldier
	States      []core.SoldierState
	FiredEvents []core.FiredEvent
}

// VehicleRecord groups a vehicle with all its time-series data
type VehicleRecord struct {
	Vehicle core.Vehicle
	States  []core.VehicleState
}

// MarkerRecord groups a marker with all its state changes
type MarkerRecord struct {
	Marker core.Marker
	States []core.MarkerState
}

// Build creates an Export from the mission data
func Build(data *MissionData) Export {
	export := Export{
		AddonVersion:     data.Mission.AddonVersion,
		ExtensionVersion: data.Mission.ExtensionVersion,
		ExtensionBuild:   data.Mission.ExtensionBuild,
		MissionName:      data.Mission.MissionName,
		MissionAuthor:    data.Mission.Author,
		WorldName:        data.World.WorldName,
		CaptureDelay:     data.Mission.CaptureDelay,
		Tags:             data.Mission.Tag,
		Times:            make([]Time, 0, len(data.TimeStates)),
		Entities:         make([]Entity, 0),
		Events:           make([][]any, 0),
		Markers:          make([][]any, 0),
	}

	// Convert time states
	for _, ts := range data.TimeStates {
		export.Times = append(export.Times, Time{
			Date:           ts.MissionDate,
			FrameNum:       ts.CaptureFrame,
			SystemTimeUTC:  ts.SystemTimeUTC,
			Time:           ts.MissionTime,
			TimeMultiplier: ts.TimeMultiplier,
		})
	}

	var maxFrame uint = 0

	// Find max entity ID to size the entities array correctly
	// The JS frontend uses entities[id] to look up entities, so array index must equal entity ID
	var maxEntityID uint16 = 0
	hasEntities := len(data.Soldiers) > 0 || len(data.Vehicles) > 0
	for _, record := range data.Soldiers {
		if record.Soldier.ID > maxEntityID {
			maxEntityID = record.Soldier.ID
		}
	}
	for _, record := range data.Vehicles {
		if record.Vehicle.ID > maxEntityID {
			maxEntityID = record.Vehicle.ID
		}
	}

	// Create entities array with placeholder entries
	// Index N will contain entity with ID=N
	if hasEntities {
		export.Entities = make([]Entity, maxEntityID+1)
	}

	// Convert soldiers - place at index matching their ID
	for _, record := range data.Soldiers {
		entity := Entity{
			ID:            record.Soldier.ID,
			Name:          record.Soldier.UnitName,
			Group:         record.Soldier.GroupID,
			Side:          record.Soldier.Side,
			IsPlayer:      boolToInt(record.Soldier.IsPlayer),
			Type:          "unit",
			Role:          record.Soldier.RoleDescription,
			StartFrameNum: record.Soldier.JoinFrame,
			Positions:     make([][]any, 0, len(record.States)),
			FramesFired:   make([][]any, 0, len(record.FiredEvents)),
		}

		for _, state := range record.States {
			// Convert nil InVehicleObjectID to 0 (old C++ extension uses 0 for "not in vehicle")
			var inVehicleID any = 0
			if state.InVehicleObjectID != nil {
				inVehicleID = *state.InVehicleObjectID
			}

			pos := []any{
				[]float64{state.Position.X, state.Position.Y, state.Position.Z},
				state.Bearing,
				state.Lifestate,
				inVehicleID,
				state.UnitName,
				boolToInt(state.IsPlayer),
				state.CurrentRole,
				state.GroupID,
				state.Side,
			}
			entity.Positions = append(entity.Positions, pos)
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}

		for _, fired := range record.FiredEvents {
			// v1 format: [frameNum, [x, y, z]] - matches old C++ extension
			ff := []any{
				fired.CaptureFrame,
				[]float64{fired.EndPos.X, fired.EndPos.Y, fired.EndPos.Z},
			}
			entity.FramesFired = append(entity.FramesFired, ff)
		}

		export.Entities[record.Soldier.ID] = entity
	}

	// Convert vehicles - place at index matching their ID
	for _, record := range data.Vehicles {
		entity := Entity{
			ID:            record.Vehicle.ID,
			Name:          record.Vehicle.DisplayName,
			Side:          "UNKNOWN",
			IsPlayer:      0,
			Type:          "vehicle",
			Class:         record.Vehicle.OcapType,
			StartFrameNum: record.Vehicle.JoinFrame,
			Positions:     make([][]any, 0, len(record.States)),
			FramesFired:   [][]any{},
		}

		for _, state := range record.States {
			// Parse crew JSON string into actual JSON array
			var crew any
			if state.Crew != "" {
				if err := json.Unmarshal([]byte(state.Crew), &crew); err != nil {
					crew = []any{} // Fallback to empty array on parse error
				}
			} else {
				crew = []any{}
			}

			pos := []any{
				[]float64{state.Position.X, state.Position.Y, state.Position.Z},
				state.Bearing,
				boolToInt(state.IsAlive),
				crew,
				[]uint{state.CaptureFrame, state.CaptureFrame},
			}
			entity.Positions = append(entity.Positions, pos)
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}

		export.Entities[record.Vehicle.ID] = entity
	}

	export.EndFrame = maxFrame

	// Convert general events
	// Format: [frameNum, "type", message]
	for _, evt := range data.GeneralEvents {
		// Try to parse message as JSON - if it's a valid JSON array/object, use parsed value
		// Otherwise keep as string
		var message any = evt.Message
		if len(evt.Message) > 0 && (evt.Message[0] == '[' || evt.Message[0] == '{') {
			var parsed any
			if err := json.Unmarshal([]byte(evt.Message), &parsed); err == nil {
				message = parsed
			}
		}
		export.Events = append(export.Events, []any{
			evt.CaptureFrame,
			evt.Name,
			message,
		})
	}

	// Convert hit events
	// Format: [frameNum, "hit", victimId, [causedById, weapon], distance]
	for _, evt := range data.HitEvents {
		var victimID uint
		if evt.VictimVehicleID != nil {
			victimID = *evt.VictimVehicleID
		} else if evt.VictimSoldierID != nil {
			victimID = *evt.VictimSoldierID
		}

		var sourceID uint
		if evt.ShooterVehicleID != nil {
			sourceID = *evt.ShooterVehicleID
		} else if evt.ShooterSoldierID != nil {
			sourceID = *evt.ShooterSoldierID
		}

		export.Events = append(export.Events, []any{
			evt.CaptureFrame,
			"hit",
			victimID,
			[]any{sourceID, evt.EventText}, // [causedById, weapon]
			evt.Distance,
		})
	}

	// Convert kill events
	// Format: [frameNum, "killed", victimId, [causedById, weapon], distance]
	for _, evt := range data.KillEvents {
		var victimID uint
		if evt.VictimVehicleID != nil {
			victimID = *evt.VictimVehicleID
		} else if evt.VictimSoldierID != nil {
			victimID = *evt.VictimSoldierID
		}

		var killerID uint
		if evt.KillerVehicleID != nil {
			killerID = *evt.KillerVehicleID
		} else if evt.KillerSoldierID != nil {
			killerID = *evt.KillerSoldierID
		}

		export.Events = append(export.Events, []any{
			evt.CaptureFrame,
			"killed",
			victimID,
			[]any{killerID, evt.EventText}, // [causedById, weapon]
			evt.Distance,
		})
	}

	// Convert markers
	// Format: [type, text, startFrame, endFrame, playerId, color, sideIndex, positions, size, shape, brush]
	// positions is always: [[frameNum, pos, direction, alpha], ...]
	// For POLYLINE: pos is [[x1,y1],[x2,y2],...] (array of coordinates)
	// For other shapes: pos is [x, y] (single coordinate)
	for _, record := range data.Markers {
		posArray := make([][]any, 0)

		if record.Marker.Shape == "POLYLINE" {
			// For polylines: pos contains the coordinate array
			coords := make([][]float64, len(record.Marker.Polyline))
			for i, pt := range record.Marker.Polyline {
				coords[i] = []float64{pt.X, pt.Y}
			}
			posArray = append(posArray, []any{
				record.Marker.CaptureFrame,
				coords, // [[x1,y1], [x2,y2], ...]
				record.Marker.Direction,
				record.Marker.Alpha,
			})
		} else {
			// For other shapes: pos is a single coordinate
			posArray = append(posArray, []any{
				record.Marker.CaptureFrame,
				[]float64{record.Marker.Position.X, record.Marker.Position.Y, record.Marker.Position.Z},
				record.Marker.Direction,
				record.Marker.Alpha,
			})

			// State changes
			for _, state := range record.States {
				posArray = append(posArray, []any{
					state.CaptureFrame,
					[]float64{state.Position.X, state.Position.Y, state.Position.Z},
					state.Direction,
					state.Alpha,
				})
			}
		}

		// Strip "#" prefix from hex colors (e.g., "#800000" -> "800000") for URL compatibility
		// The web UI constructs URLs like: /images/markers/${type}/${color}.png
		// With "#" prefix, browsers interpret the fragment as an anchor, causing 404s
		markerColor := strings.TrimPrefix(record.Marker.Color, "#")

		// EndFrame: 0 means not set (use -1 to persist), positive means specific end frame
		endFrame := record.Marker.EndFrame
		if endFrame == 0 {
			endFrame = -1
		}

		marker := []any{
			record.Marker.MarkerType,            // [0] type
			record.Marker.Text,                  // [1] text
			record.Marker.CaptureFrame,          // [2] startFrame
			endFrame,                            // [3] endFrame (-1 = persists until end, otherwise frame when marker disappears)
			record.Marker.OwnerID,               // [4] playerId (entity ID of creating player, -1 for system markers)
			markerColor,                         // [5] color (# prefix stripped for URL compatibility)
			sideToIndex(record.Marker.Side),     // [6] sideIndex
			posArray,                            // [7] positions
			parseMarkerSize(record.Marker.Size), // [8] size
			record.Marker.Shape,                 // [9] shape
			record.Marker.Brush,                 // [10] brush
		}

		export.Markers = append(export.Markers, marker)
	}

	// Convert projectile events into firelines, markers, and hit events
	for _, pe := range data.ProjectileEvents {
		if !isProjectileMarker(pe.SimulationType, pe.Weapon) {
			// Bullets become fire lines on the soldier entity
			if len(pe.Trajectory) >= 2 && int(pe.FirerObjectID) < len(export.Entities) {
				endPt := pe.Trajectory[len(pe.Trajectory)-1]
				ff := []any{
					pe.CaptureFrame,
					[]float64{endPt.Position.X, endPt.Position.Y, endPt.Position.Z},
				}
				export.Entities[pe.FirerObjectID].FramesFired = append(
					export.Entities[pe.FirerObjectID].FramesFired, ff,
				)
			}
		} else {
			// Non-bullet projectiles become markers
			// Determine icon and color
			iconFilename := extractFilename(pe.MagazineIcon)
			var markerType, color string
			if iconFilename != "" {
				markerType = "magIcons/" + iconFilename
				color = "ColorWhite"
			} else {
				markerType = "mil_triangle"
				color = "ColorRed"
			}

			// Determine text
			var text string
			switch {
			case pe.VehicleObjectID != nil && *pe.VehicleObjectID != pe.FirerObjectID:
				vehicleName := ""
				if vr, ok := data.Vehicles[*pe.VehicleObjectID]; ok {
					vehicleName = vr.Vehicle.DisplayName
				}
				text = fmt.Sprintf("%s %s - %s", vehicleName, pe.MuzzleDisplay, pe.MagazineDisplay)
			case pe.SimulationType == "shotGrenade" || pe.Weapon == "throw":
				text = pe.MagazineDisplay
			default:
				text = fmt.Sprintf("%s - %s", pe.MuzzleDisplay, pe.MagazineDisplay)
			}

			// Build position array from trajectory
			posArray := make([][]any, 0, len(pe.Trajectory))
			for _, tp := range pe.Trajectory {
				posArray = append(posArray, []any{
					tp.Frame,
					[]float64{tp.Position.X, tp.Position.Y, tp.Position.Z},
					0,
					1.0,
				})
			}

			// EndFrame is the last trajectory point's frame
			endFrame := -1
			if len(pe.Trajectory) > 0 {
				endFrame = int(pe.Trajectory[len(pe.Trajectory)-1].Frame)
			}

			marker := []any{
				markerType,                   // [0] type
				text,                         // [1] text
				pe.CaptureFrame,              // [2] startFrame
				endFrame,                     // [3] endFrame
				int(pe.FirerObjectID),        // [4] playerId
				color,                        // [5] color
				-1,                           // [6] sideIndex (GLOBAL)
				posArray,                     // [7] positions
				[]float64{1, 1},              // [8] size
				"ICON",                       // [9] shape
				"Solid",                      // [10] brush
			}

			export.Markers = append(export.Markers, marker)
		}

		// Hit events from projectile
		if len(pe.Hits) > 0 {
			// Build weapon display text
			weaponName := pe.MuzzleDisplay
			if weaponName == "" {
				weaponName = pe.WeaponDisplay
			}
			eventText := util.FormatWeaponText("", weaponName, pe.MagazineDisplay)

			// Start position for distance calculation
			var startPos core.Position3D
			if len(pe.Trajectory) > 0 {
				startPos = pe.Trajectory[0].Position
			}

			for _, hit := range pe.Hits {
				var victimID uint
				if hit.SoldierID != nil {
					victimID = uint(*hit.SoldierID)
				} else if hit.VehicleID != nil {
					victimID = uint(*hit.VehicleID)
				}

				dx := startPos.X - hit.Position.X
				dy := startPos.Y - hit.Position.Y
				dist := float32(math.Sqrt(dx*dx + dy*dy))

				export.Events = append(export.Events, []any{
					hit.CaptureFrame,
					"hit",
					victimID,
					[]any{uint(pe.FirerObjectID), eventText},
					dist,
				})
			}
		}
	}

	return export
}

// parseMarkerSize converts size string "[w,h]" to []float64{w, h}
// Falls back to [1.0, 1.0] if parsing fails
func parseMarkerSize(sizeStr string) []float64 {
	var size []float64
	if err := json.Unmarshal([]byte(sizeStr), &size); err != nil || len(size) != 2 {
		return []float64{1.0, 1.0}
	}
	return size
}

// sideToIndex converts side string to numeric index for markers
// Input: result of "str side" from SQF (EAST, WEST, GUER, CIV, EMPTY, LOGIC, UNKNOWN)
// Returns: -1=GLOBAL, 0=EAST, 1=WEST, 2=GUER, 3=CIV
func sideToIndex(side string) int {
	switch strings.ToUpper(side) {
	case "EAST", "OPFOR":
		return 0
	case "WEST", "BLUFOR":
		return 1
	case "GUER", "INDEPENDENT":
		return 2
	case "CIV", "CIVILIAN":
		return 3
	default:
		return -1 // GLOBAL (includes EMPTY, LOGIC, UNKNOWN)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isProjectileMarker returns true if the projectile should be rendered as a
// moving marker rather than a fire-line. Bullets are fire-lines; everything
// else (grenades, rockets, missiles, shells, etc.) becomes a marker.
func isProjectileMarker(sim, weapon string) bool {
	if sim != "" {
		return sim != "shotBullet"
	}
	return weapon == "throw"
}

// extractFilename returns the last path component from a file path.
// Handles both forward and backslash separators (Arma uses backslashes).
func extractFilename(path string) string {
	lastSep := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			lastSep = i
			break
		}
	}
	if lastSep >= 0 {
		return path[lastSep+1:]
	}
	return path
}
