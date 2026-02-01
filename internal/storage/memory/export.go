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
type OcapExport struct {
	AddonVersion     string       `json:"addonVersion"`
	ExtensionVersion string       `json:"extensionVersion"`
	MissionName      string       `json:"missionName"`
	MissionAuthor    string       `json:"missionAuthor"`
	WorldName        string       `json:"worldName"`
	EndFrame         uint         `json:"endFrame"`
	CaptureDelay     float32      `json:"captureDelay"`
	Entities         []EntityJSON `json:"entities"`
	Events           []EventJSON  `json:"events"`
	Markers          []MarkerJSON `json:"markers"`
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

// EventJSON represents a general event
type EventJSON struct {
	Frame   uint   `json:"frame"`
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// MarkerJSON represents a marker
type MarkerJSON struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Color     string  `json:"color"`
	Side      string  `json:"side"`
	Shape     string  `json:"shape"`
	Positions [][]any `json:"positions"`
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
		return b.writeGzipJSON(outputPath, export)
	}
	return b.writeJSON(outputPath, export)
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
		Events:           make([]EventJSON, 0),
		Markers:          make([]MarkerJSON, 0),
	}

	var maxFrame uint = 0

	// Convert soldiers
	for _, record := range b.soldiers {
		entity := EntityJSON{
			ID:            record.Soldier.OcapID,
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
			ID:            record.Vehicle.OcapID,
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
	for _, evt := range b.generalEvents {
		export.Events = append(export.Events, EventJSON{
			Frame:   evt.CaptureFrame,
			Type:    evt.Name,
			Message: evt.Message,
			Data:    evt.ExtraData,
		})
	}

	// Convert markers
	for _, record := range b.markers {
		marker := MarkerJSON{
			ID:        fmt.Sprintf("%d", record.Marker.ID),
			Name:      record.Marker.Text,
			Type:      record.Marker.MarkerType,
			Color:     record.Marker.Color,
			Side:      record.Marker.Side,
			Shape:     record.Marker.Shape,
			Positions: make([][]any, 0),
		}

		// Initial position
		marker.Positions = append(marker.Positions, []any{
			record.Marker.CaptureFrame,
			[]float64{record.Marker.Position.X, record.Marker.Position.Y},
			record.Marker.Direction,
			record.Marker.Alpha,
		})

		// State changes
		for _, state := range record.States {
			marker.Positions = append(marker.Positions, []any{
				state.CaptureFrame,
				[]float64{state.Position.X, state.Position.Y},
				state.Direction,
				state.Alpha,
			})
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
