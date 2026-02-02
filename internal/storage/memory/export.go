// internal/storage/memory/export.go
package memory

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OcapExport is the root JSON structure
// Note: Markers uses capital M for compatibility with ocap2-web
type OcapExport struct {
	AddonVersion     string       `json:"addonVersion"`
	ExtensionVersion string       `json:"extensionVersion"`
	MissionName      string       `json:"missionName"`
	MissionAuthor    string       `json:"missionAuthor"`
	WorldName        string       `json:"worldName"`
	EndFrame         uint         `json:"endFrame"`
	CaptureDelay     float32      `json:"captureDelay"`
	Entities         []EntityJSON `json:"entities"`
	Events           [][]any      `json:"events"`
	Markers          [][]any      `json:"Markers"` // Capital M for ocap2-web compatibility
}

// EntityJSON represents a soldier or vehicle
type EntityJSON struct {
	ID            uint16  `json:"id"`
	Name          string  `json:"name"`
	Group         string  `json:"group,omitempty"`
	Side          string  `json:"side"`
	IsPlayer      int     `json:"isPlayer"`
	Type          string  `json:"type"`
	Class         string  `json:"class,omitempty"`
	StartFrameNum uint    `json:"startFrameNum"`
	Positions     [][]any `json:"positions"`
	FramesFired   [][]any `json:"framesFired"`
}

// sideToIndex converts side string to numeric index for markers
// -1=GLOBAL, 0=EAST, 1=WEST, 2=GUER, 3=CIV
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
		return -1 // GLOBAL
	}
}

// exportJSON writes the mission data to a gzipped JSON file
func (b *Backend) exportJSON() error {
	export := b.buildExport()

	// Build filename
	missionName := strings.ReplaceAll(b.mission.MissionName, " ", "_")
	missionName = strings.ReplaceAll(missionName, ":", "_")
	timestamp := b.mission.StartTime.Format("20060102_150405")

	var filename string
	if b.cfg.CompressOutput {
		filename = fmt.Sprintf("%s_%s.json.gz", missionName, timestamp)
	} else {
		filename = fmt.Sprintf("%s_%s.json", missionName, timestamp)
	}

	outputPath := filepath.Join(b.cfg.OutputDir, filename)

	// Ensure output directory exists
	if err := os.MkdirAll(b.cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if b.cfg.CompressOutput {
		if err := b.writeGzipJSON(outputPath, export); err != nil {
			return err
		}
	} else {
		if err := b.writeJSON(outputPath, export); err != nil {
			return err
		}
	}

	b.lastExportPath = outputPath
	return nil
}

func (b *Backend) buildExport() OcapExport {
	export := OcapExport{
		AddonVersion:     b.mission.AddonVersion,
		ExtensionVersion: b.mission.ExtensionVersion,
		MissionName:      b.mission.MissionName,
		MissionAuthor:    b.mission.Author,
		WorldName:        b.world.WorldName,
		CaptureDelay:     b.mission.CaptureDelay,
		Entities:         make([]EntityJSON, 0),
		Events:           make([][]any, 0),
		Markers:          make([][]any, 0),
	}

	var maxFrame uint = 0

	// Convert soldiers
	for _, record := range b.soldiers {
		entity := EntityJSON{
			ID:            record.Soldier.ID, // ID is the ObjectID
			Name:          record.Soldier.UnitName,
			Group:         record.Soldier.GroupID,
			Side:          record.Soldier.Side,
			IsPlayer:      boolToInt(record.Soldier.IsPlayer),
			Type:          "unit",
			StartFrameNum: record.Soldier.JoinFrame,
			Positions:     make([][]any, 0, len(record.States)),
			FramesFired:   make([][]any, 0, len(record.FiredEvents)),
		}

		for _, state := range record.States {
			pos := []any{
				[]float64{state.Position.X, state.Position.Y},
				state.Bearing,
				state.Lifestate,
				state.InVehicleObjectID,
				state.UnitName,
				boolToInt(state.IsPlayer),
				state.CurrentRole,
			}
			entity.Positions = append(entity.Positions, pos)
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}

		for _, fired := range record.FiredEvents {
			ff := []any{
				fired.CaptureFrame,
				[]float64{fired.EndPos.X, fired.EndPos.Y},
				[]float64{fired.StartPos.X, fired.StartPos.Y},
				fired.Weapon,
				fired.Magazine,
				fired.FiringMode,
			}
			entity.FramesFired = append(entity.FramesFired, ff)
		}

		export.Entities = append(export.Entities, entity)
	}

	// Convert vehicles
	for _, record := range b.vehicles {
		entity := EntityJSON{
			ID:            record.Vehicle.ID, // ID is the ObjectID
			Name:          record.Vehicle.DisplayName,
			Side:          "UNKNOWN",
			IsPlayer:      0,
			Type:          record.Vehicle.OcapType,
			Class:         record.Vehicle.ClassName,
			StartFrameNum: record.Vehicle.JoinFrame,
			Positions:     make([][]any, 0, len(record.States)),
			FramesFired:   [][]any{},
		}

		for _, state := range record.States {
			pos := []any{
				[]float64{state.Position.X, state.Position.Y},
				state.Bearing,
				boolToInt(state.IsAlive),
				state.Crew,
			}
			entity.Positions = append(entity.Positions, pos)
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}

		export.Entities = append(export.Entities, entity)
	}

	export.EndFrame = maxFrame

	// Convert general events
	// Format: [frameNum, "type", message]
	for _, evt := range b.generalEvents {
		export.Events = append(export.Events, []any{
			evt.CaptureFrame,
			evt.Name,
			evt.Message,
		})
	}

	// Convert hit events
	// Format: [frameNum, "hit", victimId, [causedById, weapon], distance]
	for _, evt := range b.hitEvents {
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
	for _, evt := range b.killEvents {
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
	// Where positions is: [[frameNum, [x, y, z], direction, alpha], ...]
	for _, record := range b.markers {
		positions := make([][]any, 0)

		// Initial position
		positions = append(positions, []any{
			record.Marker.CaptureFrame,
			[]float64{record.Marker.Position.X, record.Marker.Position.Y, 0},
			record.Marker.Direction,
			record.Marker.Alpha,
		})

		// State changes
		for _, state := range record.States {
			positions = append(positions, []any{
				state.CaptureFrame,
				[]float64{state.Position.X, state.Position.Y, 0},
				state.Direction,
				state.Alpha,
			})
		}

		marker := []any{
			record.Marker.MarkerType,          // [0] type
			record.Marker.Text,                // [1] text
			record.Marker.CaptureFrame,        // [2] startFrame
			-1,                                // [3] endFrame (-1 = persists until end)
			-1,                                // [4] playerId (-1 = no player)
			record.Marker.Color,               // [5] color
			sideToIndex(record.Marker.Side),   // [6] sideIndex
			positions,                         // [7] positions
			[]float32{1.0, 1.0},               // [8] size
			record.Marker.Shape,               // [9] shape
			"Solid",                           // [10] brush
		}

		export.Markers = append(export.Markers, marker)
	}

	return export
}

func (b *Backend) writeJSON(path string, data OcapExport) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	return encoder.Encode(data)
}

func (b *Backend) writeGzipJSON(path string, data OcapExport) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	gzWriter := gzip.NewWriter(f)
	defer gzWriter.Close()

	encoder := json.NewEncoder(gzWriter)
	return encoder.Encode(data)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
