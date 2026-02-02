// internal/storage/memory/export_test.go
package memory

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoolToInt(t *testing.T) {
	assert.Equal(t, 1, boolToInt(true))
	assert.Equal(t, 0, boolToInt(false))
}

// TestIntegrationFullExport is a comprehensive integration test that verifies the full flow:
// start mission -> add entities (soldier, vehicle, marker) -> record states/events -> export JSON -> verify output
func TestIntegrationFullExport(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{
		OutputDir:      tempDir,
		CompressOutput: false,
	})

	mission := &core.Mission{
		MissionName:      "Test Mission",
		Author:           "Test Author",
		StartTime:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		CaptureDelay:     1.0,
		AddonVersion:     "1.0.0",
		ExtensionVersion: "2.0.0",
	}
	world := &core.World{
		WorldName: "Altis",
	}

	require.NoError(t, b.StartMission(mission, world))
	require.NoError(t, b.AddSoldier(&core.Soldier{
		ID: 1, UnitName: "Player1", GroupID: "Alpha", Side: "WEST", IsPlayer: true, JoinFrame: 0,
	}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 1, CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000, Z: 100},
		Bearing: 90, Lifestate: 1, UnitName: "Player1", IsPlayer: true, CurrentRole: "Rifleman",
	}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 1, CaptureFrame: 10, Position: core.Position3D{X: 1050, Y: 2050, Z: 100},
		Bearing: 95, Lifestate: 1, UnitName: "Player1", IsPlayer: true, CurrentRole: "Rifleman",
	}))
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: 1, CaptureFrame: 5, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39_caseless_mag",
		FiringMode: "Single", StartPos: core.Position3D{X: 1000, Y: 2000, Z: 1.5}, EndPos: core.Position3D{X: 1100, Y: 2100, Z: 1.5},
	}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{
		ID: 10, ClassName: "B_MRAP_01_F", DisplayName: "Hunter", OcapType: "car", JoinFrame: 2,
	}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{
		VehicleID: 10, CaptureFrame: 5, Position: core.Position3D{X: 3000, Y: 4000, Z: 50},
		Bearing: 180, IsAlive: true, Crew: "[[1,\"driver\"]]",
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "objective_1", Text: "Objective Alpha", MarkerType: "mil_objective",
		Color: "ColorBLUFOR", Side: "WEST", Shape: "ICON", CaptureFrame: 0,
		Position: core.Position3D{X: 5000, Y: 6000, Z: 0}, Direction: 0, Alpha: 1.0,
	}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{
		MarkerID: 0, CaptureFrame: 20, Position: core.Position3D{X: 5100, Y: 6100, Z: 0}, Direction: 45, Alpha: 0.8,
	}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{
		CaptureFrame: 15, Name: "connected", Message: "Player1 connected", ExtraData: map[string]any{"uid": "12345"},
	}))
	require.NoError(t, b.EndMission())

	// Find and read the exported JSON file
	pattern := filepath.Join(tempDir, "Test_Mission_*.json")
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected 1 JSON file")

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var export OcapExport
	require.NoError(t, json.Unmarshal(data, &export))

	// Verify mission metadata
	assert.Equal(t, "Test Mission", export.MissionName)
	assert.Equal(t, "Test Author", export.MissionAuthor)
	assert.Equal(t, "Altis", export.WorldName)
	assert.Equal(t, float32(1.0), export.CaptureDelay)
	assert.Equal(t, "1.0.0", export.AddonVersion)
	assert.Equal(t, "2.0.0", export.ExtensionVersion)
	assert.Equal(t, uint(10), export.EndFrame, "EndFrame should be max state frame")

	// Verify entities (1 soldier + 1 vehicle)
	require.Len(t, export.Entities, 2)

	// Find soldier and vehicle entities
	var soldierEntity, vehicleEntity *EntityJSON
	for i := range export.Entities {
		switch export.Entities[i].Type {
		case "unit":
			soldierEntity = &export.Entities[i]
		case "car":
			vehicleEntity = &export.Entities[i]
		}
	}

	// Verify soldier entity
	require.NotNil(t, soldierEntity, "soldier entity not found")
	assert.Equal(t, uint16(1), soldierEntity.ID)
	assert.Equal(t, "Player1", soldierEntity.Name)
	assert.Equal(t, "Alpha", soldierEntity.Group)
	assert.Equal(t, "WEST", soldierEntity.Side)
	assert.Equal(t, 1, soldierEntity.IsPlayer)
	require.Len(t, soldierEntity.Positions, 2)
	require.Len(t, soldierEntity.FramesFired, 1)

	// Verify soldier position coordinates after JSON round-trip
	coords, ok := soldierEntity.Positions[0][0].([]any)
	require.True(t, ok, "position coords should be []any after JSON unmarshal")
	assert.Equal(t, 1000.0, coords[0])
	assert.Equal(t, 2000.0, coords[1])

	// Verify fired event
	ff := soldierEntity.FramesFired[0]
	assert.Equal(t, 5.0, ff[0]) // JSON unmarshals numbers as float64
	assert.Equal(t, "arifle_MX_F", ff[3])

	// Verify vehicle entity
	require.NotNil(t, vehicleEntity, "vehicle entity not found")
	assert.Equal(t, uint16(10), vehicleEntity.ID)
	assert.Equal(t, "Hunter", vehicleEntity.Name)
	assert.Equal(t, "car", vehicleEntity.Type)
	assert.Equal(t, "B_MRAP_01_F", vehicleEntity.Class)
	require.Len(t, vehicleEntity.Positions, 1)

	// Verify events
	require.Len(t, export.Events, 1)
	assert.Equal(t, "connected", export.Events[0].Type)
	assert.Equal(t, uint(15), export.Events[0].Frame)
	assert.Equal(t, "Player1 connected", export.Events[0].Message)

	// Verify markers
	require.Len(t, export.Markers, 1)
	assert.Equal(t, "Objective Alpha", export.Markers[0].Name)
	assert.Equal(t, "mil_objective", export.Markers[0].Type)
	assert.Equal(t, "ColorBLUFOR", export.Markers[0].Color)
	assert.Equal(t, "WEST", export.Markers[0].Side)
	assert.Len(t, export.Markers[0].Positions, 2, "initial position + 1 state change")
}

func TestExportJSON(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{
		OutputDir:      tempDir,
		CompressOutput: false,
	})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Export Test", Author: "Author", StartTime: time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
	}, &core.World{WorldName: "Tanoa"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Test"}))
	require.NoError(t, b.EndMission())

	matches, err := filepath.Glob(filepath.Join(tempDir, "Export_Test_*.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var export OcapExport
	require.NoError(t, json.Unmarshal(data, &export))
	assert.Equal(t, "Export Test", export.MissionName)
}

func TestExportGzipJSON(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{OutputDir: tempDir, CompressOutput: true})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Gzip Test", Author: "Author", StartTime: time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
	}, &core.World{WorldName: "Livonia"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Test"}))
	require.NoError(t, b.EndMission())

	matches, err := filepath.Glob(filepath.Join(tempDir, "Gzip_Test_*.json.gz"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	f, err := os.Open(matches[0])
	require.NoError(t, err)
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer gzReader.Close()

	var export OcapExport
	require.NoError(t, json.NewDecoder(gzReader).Decode(&export))
	assert.Equal(t, "Gzip Test", export.MissionName)
}

func TestFilenameGeneration(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		missionName    string
		compress       bool
		expectedSuffix string
	}{
		{"Simple Name", false, ".json"},
		{"Simple Name", true, ".json.gz"},
		{"Name:With:Colons", false, ".json"},
		{"Name With Spaces", false, ".json"},
	}

	for _, tt := range tests {
		b := New(config.MemoryConfig{OutputDir: tempDir, CompressOutput: tt.compress})

		require.NoError(t, b.StartMission(&core.Mission{MissionName: tt.missionName, StartTime: time.Now()}, &core.World{WorldName: "Test"}))
		require.NoError(t, b.EndMission())

		matches, err := filepath.Glob(filepath.Join(tempDir, "*"+tt.expectedSuffix))
		require.NoError(t, err)
		require.NotEmpty(t, matches, "no file with suffix %s found for mission '%s'", tt.expectedSuffix, tt.missionName)

		filename := filepath.Base(matches[len(matches)-1])
		assert.NotContains(t, filename, " ", "filename should not contain spaces")
		assert.NotContains(t, filename, ":", "filename should not contain colons")
	}
}

func TestExportCreatesOutputDir(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "nested", "output", "dir")

	b := New(config.MemoryConfig{
		OutputDir:      nonExistentDir,
		CompressOutput: false,
	})

	mission := &core.Mission{
		MissionName: "Nested Dir Test",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Test"}

	require.NoError(t, b.StartMission(mission, world))
	require.NoError(t, b.EndMission())

	_, err := os.Stat(nonExistentDir)
	require.NoError(t, err, "output directory should be created")

	pattern := filepath.Join(nonExistentDir, "*.json")
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, matches, 1)
}

func TestSoldierPositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Player1", IsPlayer: true}))

	inVehicleID := uint16(100)
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 1, CaptureFrame: 5, Position: core.Position3D{X: 1234.56, Y: 7890.12, Z: 50},
		Bearing: 270, Lifestate: 1, InVehicleObjectID: &inVehicleID,
		UnitName: "Player1_InVeh", IsPlayer: true, CurrentRole: "Gunner",
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	require.Len(t, export.Entities[0].Positions, 1)

	pos := export.Entities[0].Positions[0]
	require.Len(t, pos, 7) // [[x, y], bearing, lifestate, inVehicleObjectID, unitName, isPlayer, currentRole]

	coords, ok := pos[0].([]float64)
	require.True(t, ok, "position[0] should be []float64")
	assert.Equal(t, 1234.56, coords[0])
	assert.Equal(t, 7890.12, coords[1])
	assert.Equal(t, uint16(270), pos[1])
	assert.Equal(t, uint8(1), pos[2])
	assert.Equal(t, 1, pos[5])
}

func TestVehiclePositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10, DisplayName: "Hunter", OcapType: "car"}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{
		VehicleID: 10, CaptureFrame: 3, Position: core.Position3D{X: 5000, Y: 6000, Z: 25},
		Bearing: 45, IsAlive: true, Crew: "[[1,\"driver\"],[2,\"gunner\"]]",
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	require.Len(t, export.Entities[0].Positions, 1)

	pos := export.Entities[0].Positions[0]
	require.Len(t, pos, 4) // [[x, y], bearing, isAlive, crew]

	coords, ok := pos[0].([]float64)
	require.True(t, ok, "position[0] should be []float64")
	assert.Equal(t, 5000.0, coords[0])
	assert.Equal(t, 6000.0, coords[1])
	assert.Equal(t, 1, pos[2])
}

func TestFiredEventFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1}))
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: 1, CaptureFrame: 100, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39_caseless_mag",
		FiringMode: "FullAuto", StartPos: core.Position3D{X: 100, Y: 200, Z: 1.5}, EndPos: core.Position3D{X: 300, Y: 400, Z: 1.8},
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	require.Len(t, export.Entities[0].FramesFired, 1)

	ff := export.Entities[0].FramesFired[0]
	require.Len(t, ff, 6) // [captureFrame, [endX, endY], [startX, startY], weapon, magazine, firingMode]
	assert.Equal(t, uint(100), ff[0])

	endPos, ok := ff[1].([]float64)
	require.True(t, ok, "endPos should be []float64")
	assert.Equal(t, 300.0, endPos[0])
	assert.Equal(t, 400.0, endPos[1])

	startPos, ok := ff[2].([]float64)
	require.True(t, ok, "startPos should be []float64")
	assert.Equal(t, 100.0, startPos[0])
	assert.Equal(t, 200.0, startPos[1])

	assert.Equal(t, "arifle_MX_F", ff[3])
	assert.Equal(t, "30Rnd_65x39_caseless_mag", ff[4])
	assert.Equal(t, "FullAuto", ff[5])
}

func TestMarkerPositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "test_marker", Text: "Test Marker", MarkerType: "mil_dot", Color: "ColorRed",
		Side: "EAST", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000, Z: 0}, Direction: 90, Alpha: 1.0,
	}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{
		MarkerID: 0, CaptureFrame: 50, Position: core.Position3D{X: 1100, Y: 2100, Z: 0}, Direction: 180, Alpha: 0.5,
	}))

	export := b.buildExport()

	require.Len(t, export.Markers, 1)
	require.Len(t, export.Markers[0].Positions, 2) // initial + 1 state

	initialPos := export.Markers[0].Positions[0]
	require.Len(t, initialPos, 4) // [frame, [x, y], direction, alpha]
	assert.Equal(t, uint(0), initialPos[0])

	coords, ok := initialPos[1].([]float64)
	require.True(t, ok, "coords should be []float64")
	assert.Equal(t, 1000.0, coords[0])
	assert.Equal(t, 2000.0, coords[1])

	assert.Equal(t, uint(50), export.Markers[0].Positions[1][0])
}

func TestEmptyExport(t *testing.T) {
	tempDir := t.TempDir()
	b := New(config.MemoryConfig{OutputDir: tempDir, CompressOutput: false})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Empty Mission", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.EndMission())

	matches, err := filepath.Glob(filepath.Join(tempDir, "*.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var export OcapExport
	require.NoError(t, json.Unmarshal(data, &export))

	assert.Empty(t, export.Entities)
	assert.Empty(t, export.Events)
	assert.Empty(t, export.Markers)
}

func TestMaxFrameCalculation(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 10}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 50}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 30}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 10, CaptureFrame: 100}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 10, CaptureFrame: 75}))

	assert.Equal(t, uint(100), b.buildExport().EndFrame)
}

func TestSoldierWithoutVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Infantry", IsPlayer: false}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 1, CaptureFrame: 0, Position: core.Position3D{X: 100, Y: 200, Z: 10},
		Bearing: 45, Lifestate: 1, InVehicleObjectID: nil, UnitName: "Infantry", IsPlayer: false, CurrentRole: "Rifleman",
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	pos := export.Entities[0].Positions[0]

	assert.Equal(t, (*uint16)(nil), pos[3], "InVehicleObjectID should be nil")
	assert.Equal(t, 0, export.Entities[0].IsPlayer)
	assert.Equal(t, 0, pos[5])
}

func TestDeadVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 5, DisplayName: "Destroyed Tank", OcapType: "tank", ClassName: "B_MBT_01_cannon_F"}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{
		VehicleID: 5, CaptureFrame: 50, Position: core.Position3D{X: 2000, Y: 3000, Z: 0},
		Bearing: 90, IsAlive: false, Crew: "[]",
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	assert.Equal(t, "UNKNOWN", export.Entities[0].Side)
	assert.Equal(t, 0, export.Entities[0].IsPlayer)
	assert.Equal(t, 0, export.Entities[0].Positions[0][2], "isAlive should be 0 for destroyed vehicle")
}

func TestMultipleEntitiesExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Alpha1", GroupID: "Alpha", Side: "WEST", IsPlayer: true, JoinFrame: 0}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 2, UnitName: "Alpha2", GroupID: "Alpha", Side: "WEST", IsPlayer: false, JoinFrame: 5}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 3, UnitName: "Bravo1", GroupID: "Bravo", Side: "EAST", IsPlayer: false, JoinFrame: 10}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 0, Position: core.Position3D{X: 100, Y: 100}, Lifestate: 1}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 2, CaptureFrame: 5, Position: core.Position3D{X: 110, Y: 100}, Lifestate: 1}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 3, CaptureFrame: 10, Position: core.Position3D{X: 500, Y: 500}, Lifestate: 1}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10, DisplayName: "Hunter", OcapType: "car", JoinFrame: 0}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 11, DisplayName: "Heli", OcapType: "heli", JoinFrame: 20}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 10, CaptureFrame: 0, Position: core.Position3D{X: 200, Y: 200}, IsAlive: true}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 11, CaptureFrame: 20, Position: core.Position3D{X: 300, Y: 300}, IsAlive: true}))

	export := b.buildExport()

	require.Len(t, export.Entities, 5) // 3 soldiers + 2 vehicles

	unitCount, vehicleCount := 0, 0
	for _, e := range export.Entities {
		if e.Type == "unit" {
			unitCount++
		} else {
			vehicleCount++
		}
	}
	assert.Equal(t, 3, unitCount)
	assert.Equal(t, 2, vehicleCount)
}

func TestEventWithoutExtraData(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{CaptureFrame: 100, Name: "endMission", Message: "Mission ended", ExtraData: nil}))

	export := b.buildExport()

	require.Len(t, export.Events, 1)
	assert.Equal(t, "endMission", export.Events[0].Type)
	assert.Equal(t, uint(100), export.Events[0].Frame)
	assert.Equal(t, "Mission ended", export.Events[0].Message)
}

func TestMultipleMarkersExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "obj_alpha", Text: "Alpha", MarkerType: "mil_objective", Color: "ColorBLUFOR", Side: "WEST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 1000}, Direction: 0, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "obj_bravo", Text: "Bravo", MarkerType: "mil_objective", Color: "ColorOPFOR", Side: "EAST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 2000, Y: 2000}, Direction: 45, Alpha: 1.0,
	}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 0, CaptureFrame: 10, Position: core.Position3D{X: 1100, Y: 1100}, Direction: 90, Alpha: 0.8}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 0, CaptureFrame: 20, Position: core.Position3D{X: 1200, Y: 1200}, Direction: 180, Alpha: 0.6}))

	export := b.buildExport()

	require.Len(t, export.Markers, 2)

	var m1, m2 *MarkerJSON
	for i := range export.Markers {
		switch export.Markers[i].Name {
		case "Alpha":
			m1 = &export.Markers[i]
		case "Bravo":
			m2 = &export.Markers[i]
		}
	}

	require.NotNil(t, m1, "marker Alpha not found")
	require.NotNil(t, m2, "marker Bravo not found")
	assert.Len(t, m1.Positions, 3, "marker1 should have initial + 2 states")
	assert.Len(t, m2.Positions, 1, "marker2 should have only initial position")
	assert.Equal(t, "ColorBLUFOR", m1.Color)
	assert.Equal(t, "EAST", m2.Side)
}

func TestMultipleFiredEvents(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Shooter"}))
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: 1, CaptureFrame: 10, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39", FiringMode: "Single",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 200, Y: 200},
	}))
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: 1, CaptureFrame: 15, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39", FiringMode: "FullAuto",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 250, Y: 250},
	}))
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: 1, CaptureFrame: 50, Weapon: "launch_NLAW_F", Magazine: "NLAW_F", FiringMode: "Single",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 500, Y: 500},
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	require.Len(t, export.Entities[0].FramesFired, 3)

	weapons := make(map[string]bool)
	for _, ff := range export.Entities[0].FramesFired {
		weapons[ff[3].(string)] = true
	}
	assert.True(t, weapons["arifle_MX_F"], "arifle_MX_F should be recorded")
	assert.True(t, weapons["launch_NLAW_F"], "launch_NLAW_F should be recorded")
}

func TestVehicleWithJoinFrame(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{
		ID:          20,
		DisplayName: "Late Arrival",
		OcapType:    "plane",
		ClassName:   "B_Plane_Fighter_01_F",
		JoinFrame:   500, // Spawned late in the mission
	}))

	export := b.buildExport()

	require.Len(t, export.Entities, 1)
	entity := export.Entities[0]
	assert.Equal(t, uint(500), entity.StartFrameNum)
	assert.Equal(t, "plane", entity.Type)
	assert.Equal(t, "B_Plane_Fighter_01_F", entity.Class)
}
