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
	v1 "github.com/OCAP2/extension/v5/internal/storage/memory/export/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		ExtensionBuild:   "Mon Jan 15 10:00:00 2024",
		Tag:              "PvP",
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
	require.NoError(t, b.RecordTimeState(&core.TimeState{
		CaptureFrame: 0, SystemTimeUTC: "2024-01-15T10:30:00.000", MissionDate: "2035-06-24T06:00:00", TimeMultiplier: 1.0, MissionTime: 0,
	}))
	require.NoError(t, b.EndMission())

	// Find and read the exported JSON file
	pattern := filepath.Join(tempDir, "Test_Mission_*.json")
	matches, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected 1 JSON file")

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var export v1.Export
	require.NoError(t, json.Unmarshal(data, &export))

	// Verify mission metadata
	assert.Equal(t, "Test Mission", export.MissionName)
	assert.Equal(t, "Test Author", export.MissionAuthor)
	assert.Equal(t, "Altis", export.WorldName)
	assert.Equal(t, float32(1.0), export.CaptureDelay)
	assert.Equal(t, "1.0.0", export.AddonVersion)
	assert.Equal(t, "2.0.0", export.ExtensionVersion)
	assert.Equal(t, "Mon Jan 15 10:00:00 2024", export.ExtensionBuild)
	assert.Equal(t, "PvP", export.Tags)
	assert.Equal(t, uint(10), export.EndFrame, "EndFrame should be max state frame")

	// Verify times
	require.Len(t, export.Times, 1)
	assert.Equal(t, uint(0), export.Times[0].FrameNum)
	assert.Equal(t, "2024-01-15T10:30:00.000", export.Times[0].SystemTimeUTC)

	// Verify entities (array is sparse: index = entity ID)
	// Soldier has ID 1, Vehicle has ID 10, so array length is 11 (0-10)
	require.Len(t, export.Entities, 11)

	// Access entities by their ID (which equals array index)
	soldierEntity := &export.Entities[1]  // ID 1
	vehicleEntity := &export.Entities[10] // ID 10

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

	// Verify events (array format: [frameNum, type, message])
	require.Len(t, export.Events, 1)
	assert.Equal(t, "connected", export.Events[0][1])         // type
	assert.EqualValues(t, 15, export.Events[0][0])            // frameNum
	assert.Equal(t, "Player1 connected", export.Events[0][2]) // message

	// Verify markers (array format: [type, text, startFrame, endFrame, playerId, color, sideIndex, positions, size, shape, brush])
	require.Len(t, export.Markers, 1)
	assert.Equal(t, "Objective Alpha", export.Markers[0][1]) // text
	assert.Equal(t, "mil_objective", export.Markers[0][0])   // type
	assert.Equal(t, "ColorBLUFOR", export.Markers[0][5])     // color
	assert.EqualValues(t, 1, export.Markers[0][6])           // sideIndex (WEST = 1)
	// JSON decodes nested arrays as []interface{}, not [][]any
	positions, ok := export.Markers[0][7].([]interface{})
	require.True(t, ok, "positions should be []interface{}")
	assert.Len(t, positions, 2, "initial position + 1 state change")
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

	var export v1.Export
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

	var export v1.Export
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

	export := b.BuildExport()

	// Sparse array: entity at index 1 (its ID)
	require.Len(t, export.Entities, 2) // indices 0 and 1
	entity := export.Entities[1]       // Access by ID
	require.Len(t, entity.Positions, 1)

	pos := entity.Positions[0]
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

	export := b.BuildExport()

	// Sparse array: entity at index 10 (its ID)
	require.Len(t, export.Entities, 11) // indices 0-10
	entity := export.Entities[10]       // Access by ID
	require.Len(t, entity.Positions, 1)

	pos := entity.Positions[0]
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

	export := b.BuildExport()

	// Sparse array: entity at index 1 (its ID)
	require.Len(t, export.Entities, 2)        // indices 0 and 1
	require.Len(t, export.Entities[1].FramesFired, 1)

	ff := export.Entities[1].FramesFired[0]
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

	export := b.BuildExport()

	require.Len(t, export.Markers, 1)
	positions := export.Markers[0][7].([][]any) // positions at index 7
	require.Len(t, positions, 2)                // initial + 1 state

	// Position format: [frameNum, [x, y], direction, alpha]
	initialPos := positions[0]
	assert.Equal(t, uint(0), initialPos[0])      // frameNum
	coords := initialPos[1].([]float64)
	assert.Equal(t, 1000.0, coords[0])           // posX
	assert.Equal(t, 2000.0, coords[1])           // posY

	assert.Equal(t, uint(50), positions[1][0])   // second position frameNum
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

	var export v1.Export
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

	assert.Equal(t, uint(100), b.BuildExport().EndFrame)
}

func TestSoldierWithoutVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Infantry", IsPlayer: false}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 1, CaptureFrame: 0, Position: core.Position3D{X: 100, Y: 200, Z: 10},
		Bearing: 45, Lifestate: 1, InVehicleObjectID: nil, UnitName: "Infantry", IsPlayer: false, CurrentRole: "Rifleman",
	}))

	export := b.BuildExport()

	// Sparse array: entity at index 1 (its ID)
	require.Len(t, export.Entities, 2) // indices 0 and 1
	entity := export.Entities[1]
	pos := entity.Positions[0]

	assert.Equal(t, 0, pos[3], "InVehicleObjectID should be 0 when not in vehicle")
	assert.Equal(t, 0, entity.IsPlayer)
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

	export := b.BuildExport()

	// Sparse array: entity at index 5 (its ID)
	require.Len(t, export.Entities, 6) // indices 0-5
	entity := export.Entities[5]
	assert.Equal(t, "UNKNOWN", entity.Side)
	assert.Equal(t, 0, entity.IsPlayer)
	assert.Equal(t, 0, entity.Positions[0][2], "isAlive should be 0 for destroyed vehicle")
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

	export := b.BuildExport()

	// Sparse array: max ID is 11, so array has indices 0-11
	require.Len(t, export.Entities, 12)

	// Count actual entities (non-placeholder)
	unitCount, vehicleCount := 0, 0
	for _, e := range export.Entities {
		switch e.Type {
		case "unit":
			unitCount++
		case "car", "heli":
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

	export := b.BuildExport()

	require.Len(t, export.Events, 1)
	assert.Equal(t, "endMission", export.Events[0][1])       // type at index 1
	assert.Equal(t, uint(100), export.Events[0][0])          // frameNum at index 0
	assert.Equal(t, "Mission ended", export.Events[0][2])    // message at index 2
}

func TestGeneralEventJSONMessageParsing(t *testing.T) {
	tests := []struct {
		name            string
		message         string
		expectedMessage any
	}{
		{
			name:            "JSON array is parsed as native array",
			message:         "[-1,-1,-1,-1]",
			expectedMessage: []any{float64(-1), float64(-1), float64(-1), float64(-1)},
		},
		{
			name:            "JSON object is parsed as native object",
			message:         `{"key":"value","num":42}`,
			expectedMessage: map[string]any{"key": "value", "num": float64(42)},
		},
		{
			name:            "plain string remains string",
			message:         "Mission ended",
			expectedMessage: "Mission ended",
		},
		{
			name:            "empty string remains string",
			message:         "",
			expectedMessage: "",
		},
		{
			name:            "invalid JSON array remains string",
			message:         "[1,2,3",
			expectedMessage: "[1,2,3",
		},
		{
			name:            "string starting with bracket but not JSON remains string",
			message:         "[alpha] objective complete",
			expectedMessage: "[alpha] objective complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(config.MemoryConfig{})
			require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
			require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{
				CaptureFrame: 10,
				Name:         "testEvent",
				Message:      tt.message,
			}))

			export := b.BuildExport()

			require.Len(t, export.Events, 1)
			assert.Equal(t, tt.expectedMessage, export.Events[0][2])
		})
	}
}

func TestMultipleMarkersExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))
	// Each marker needs a unique ID so RecordMarkerState can find the correct one
	require.NoError(t, b.AddMarker(&core.Marker{
		ID: 1, MarkerName: "obj_alpha", Text: "Alpha", MarkerType: "mil_objective", Color: "ColorBLUFOR", Side: "WEST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 1000}, Direction: 0, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		ID: 2, MarkerName: "obj_bravo", Text: "Bravo", MarkerType: "mil_objective", Color: "ColorOPFOR", Side: "EAST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 2000, Y: 2000}, Direction: 45, Alpha: 1.0,
	}))
	// States for marker 1 (Alpha)
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 1, CaptureFrame: 10, Position: core.Position3D{X: 1100, Y: 1100}, Direction: 90, Alpha: 0.8}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 1, CaptureFrame: 20, Position: core.Position3D{X: 1200, Y: 1200}, Direction: 180, Alpha: 0.6}))

	export := b.BuildExport()

	require.Len(t, export.Markers, 2)

	// Markers are now arrays: [type, text, startFrame, endFrame, playerId, color, sideIndex, positions, size, shape, brush]
	var m1, m2 []any
	for i := range export.Markers {
		text := export.Markers[i][1].(string) // text at index 1
		switch text {
		case "Alpha":
			m1 = export.Markers[i]
		case "Bravo":
			m2 = export.Markers[i]
		}
	}

	require.NotNil(t, m1, "marker Alpha not found")
	require.NotNil(t, m2, "marker Bravo not found")
	m1Positions := m1[7].([][]any)
	m2Positions := m2[7].([][]any)
	assert.Len(t, m1Positions, 3, "marker1 should have initial + 2 states")
	assert.Len(t, m2Positions, 1, "marker2 should have only initial position")
	assert.Equal(t, "ColorBLUFOR", m1[5]) // color at index 5
	assert.Equal(t, 0, m2[6])             // sideIndex at index 6 (EAST = 0)
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

	export := b.BuildExport()

	// Sparse array: entity at index 1 (its ID)
	require.Len(t, export.Entities, 2)
	entity := export.Entities[1]
	require.Len(t, entity.FramesFired, 3)

	weapons := make(map[string]bool)
	for _, ff := range entity.FramesFired {
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

	export := b.BuildExport()

	// Sparse array: entity at index 20 (its ID)
	require.Len(t, export.Entities, 21) // indices 0-20
	entity := export.Entities[20]
	assert.Equal(t, uint(500), entity.StartFrameNum)
	assert.Equal(t, "plane", entity.Type)
	assert.Equal(t, "B_Plane_Fighter_01_F", entity.Class)
}

// TestJSONFormatValidation validates that JSON output matches the old C++ extension format
func TestJSONFormatValidation(t *testing.T) {
	tempDir := t.TempDir()
	b := New(config.MemoryConfig{OutputDir: tempDir, CompressOutput: false})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Format Test", Author: "Author", StartTime: time.Now(),
		CaptureDelay: 1.0, AddonVersion: "1.0.0", ExtensionVersion: "2.0.0",
	}, &core.World{WorldName: "Altis"}))

	// Add entities with specific IDs to test sparse array
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 5, UnitName: "Player", Side: "WEST", IsPlayer: true}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10, DisplayName: "Tank", OcapType: "tank", ClassName: "B_MBT_01"}))

	// Record states
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{
		SoldierID: 5, CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000},
		Bearing: 90, Lifestate: 1, UnitName: "Player", IsPlayer: true, CurrentRole: "Rifleman",
	}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{
		VehicleID: 10, CaptureFrame: 0, Position: core.Position3D{X: 3000, Y: 4000},
		Bearing: 180, IsAlive: true, Crew: "[[5,\"driver\"]]",
	}))

	// Add events in correct format: [frameNum, type, victimId, [killerId, weapon], distance]
	require.NoError(t, b.RecordKillEvent(&core.KillEvent{
		CaptureFrame:      100,
		VictimSoldierID:   ptrUint(5),
		KillerSoldierID:   ptrUint(10),
		EventText:         "Tank Gun",
		Distance:          50,
	}))

	// Add marker (OwnerID: -1 indicates system/mission marker, not player-drawn)
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "obj1", Text: "Objective", MarkerType: "mil_objective",
		Color: "ColorRed", Side: "WEST", Shape: "ICON", OwnerID: -1,
		CaptureFrame: 0, Position: core.Position3D{X: 5000, Y: 6000}, Direction: 45, Alpha: 1.0,
	}))

	require.NoError(t, b.EndMission())

	// Read and parse the JSON file
	matches, err := filepath.Glob(filepath.Join(tempDir, "*.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Validate root structure
	assert.Equal(t, "Format Test", raw["missionName"])
	assert.Equal(t, "Altis", raw["worldName"])
	assert.Equal(t, "1.0.0", raw["addonVersion"])
	assert.Equal(t, "2.0.0", raw["extensionVersion"])

	// Validate entities array is sparse (ID = index)
	entities := raw["entities"].([]any)
	assert.Len(t, entities, 11) // indices 0-10

	// Entity at index 5 should be the soldier
	soldier := entities[5].(map[string]any)
	assert.Equal(t, float64(5), soldier["id"])
	assert.Equal(t, "unit", soldier["type"])

	// Entity at index 10 should be the vehicle
	vehicle := entities[10].(map[string]any)
	assert.Equal(t, float64(10), vehicle["id"])
	assert.Equal(t, "tank", vehicle["type"])

	// Validate vehicle position format: [[x, y], bearing, alive, crew]
	vehiclePositions := vehicle["positions"].([]any)
	require.Len(t, vehiclePositions, 1)
	vehPos := vehiclePositions[0].([]any)
	coords := vehPos[0].([]any)
	assert.Equal(t, float64(3000), coords[0])
	assert.Equal(t, float64(4000), coords[1])
	// Crew should be parsed as array, not string
	crew := vehPos[3].([]any)
	require.Len(t, crew, 1)
	crewEntry := crew[0].([]any)
	assert.Equal(t, float64(5), crewEntry[0]) // driver ID

	// Validate event format: [frameNum, "killed", victimId, [killerId, weapon], distance]
	events := raw["events"].([]any)
	require.Len(t, events, 1)
	killEvent := events[0].([]any)
	assert.Equal(t, float64(100), killEvent[0])   // frameNum
	assert.Equal(t, "killed", killEvent[1])        // type
	assert.Equal(t, float64(5), killEvent[2])      // victimId
	causedBy := killEvent[3].([]any)
	assert.Equal(t, float64(10), causedBy[0])      // killerId
	assert.Equal(t, "Tank Gun", causedBy[1])       // weapon
	assert.Equal(t, float64(50), killEvent[4])     // distance

	// Validate marker format: [type, text, startFrame, endFrame, playerId, color, side, positions, size, shape, brush]
	markers := raw["Markers"].([]any)
	require.Len(t, markers, 1)
	marker := markers[0].([]any)
	assert.Equal(t, "mil_objective", marker[0]) // type
	assert.Equal(t, "Objective", marker[1])     // text
	assert.EqualValues(t, 0, marker[2])         // startFrame
	assert.EqualValues(t, -1, marker[3])        // endFrame (-1 = persists)
	assert.EqualValues(t, -1, marker[4])        // playerId
	assert.Equal(t, "ColorRed", marker[5])      // color
	assert.EqualValues(t, 1, marker[6])         // side (WEST = 1)

	// Validate marker position format: [frameNum, [x, y], direction, alpha]
	markerPositions := marker[7].([]any)
	require.Len(t, markerPositions, 1)
	mPos := markerPositions[0].([]any)
	assert.EqualValues(t, 0, mPos[0])           // frameNum
	mCoords := mPos[1].([]any)
	assert.Equal(t, float64(5000), mCoords[0])  // x
	assert.Equal(t, float64(6000), mCoords[1])  // y
	assert.Len(t, mCoords, 2)                   // should be [x, y], not [x, y, z]
}

func ptrUint(v uint) *uint {
	return &v
}

func TestMarkerColorHashPrefixIsStripped(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))

	// Add marker with hex color including # prefix (should be stripped in export)
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "hex_marker", Text: "Hex Color", MarkerType: "respawn_inf", Color: "#800000",
		Side: "WEST", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000}, Direction: 0, Alpha: 1.0,
	}))

	// Add marker with named color (no prefix, should remain unchanged)
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "named_marker", Text: "Named Color", MarkerType: "mil_dot", Color: "ColorRed",
		Side: "WEST", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 2000, Y: 3000}, Direction: 0, Alpha: 1.0,
	}))

	export := b.BuildExport()

	require.Len(t, export.Markers, 2)

	// Find markers and verify color format
	var hexMarker, namedMarker []any
	for _, m := range export.Markers {
		text := m[1].(string)
		if text == "Hex Color" {
			hexMarker = m
		} else if text == "Named Color" {
			namedMarker = m
		}
	}

	require.NotNil(t, hexMarker, "hex color marker not found")
	require.NotNil(t, namedMarker, "named color marker not found")

	// Hex color should have # prefix stripped (for URL compatibility in web UI)
	assert.Equal(t, "800000", hexMarker[5], "hex color should have # prefix stripped")

	// Named color should remain unchanged
	assert.Equal(t, "ColorRed", namedMarker[5], "named color should remain unchanged")
}

func TestMarkerOwnerIDExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))

	// Add a system marker (OwnerID: -1)
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "system_marker", Text: "System", MarkerType: "mil_objective", Color: "ColorRed",
		Side: "WEST", Shape: "ICON", OwnerID: -1,
		CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000}, Direction: 0, Alpha: 1.0,
	}))

	// Add a player-drawn marker (OwnerID: 42, which would be the player's entity ID)
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "player_marker", Text: "Player Drawn", MarkerType: "mil_dot", Color: "ColorBlue",
		Side: "BLUFOR", Shape: "ICON", OwnerID: 42,
		CaptureFrame: 10, Position: core.Position3D{X: 3000, Y: 4000}, Direction: 45, Alpha: 1.0,
	}))

	export := b.BuildExport()

	require.Len(t, export.Markers, 2)

	// Find and verify each marker's playerId (index 4 in the array)
	var systemMarker, playerMarker []any
	for _, m := range export.Markers {
		text := m[1].(string)
		if text == "System" {
			systemMarker = m
		} else if text == "Player Drawn" {
			playerMarker = m
		}
	}

	require.NotNil(t, systemMarker, "system marker not found")
	require.NotNil(t, playerMarker, "player marker not found")

	// System marker should have playerId: -1
	assert.Equal(t, -1, systemMarker[4], "system marker should have playerId -1")

	// Player-drawn marker should have the player's entity ID
	assert.Equal(t, 42, playerMarker[4], "player marker should have playerId 42")
}

func TestMarkerSizeAndBrushExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))

	// Add marker with custom size and brush
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "custom_marker", Text: "Custom", MarkerType: "mil_objective", Color: "ColorRed",
		Side: "WEST", Shape: "RECTANGLE", OwnerID: -1, Size: "[2.5,3.0]", Brush: "SolidBorder",
		CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 2000}, Direction: 0, Alpha: 1.0,
	}))

	// Add marker without size (should default to [1.0, 1.0])
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "default_marker", Text: "Default", MarkerType: "mil_dot", Color: "ColorBlue",
		Side: "WEST", Shape: "ICON", OwnerID: -1, Size: "", Brush: "Solid",
		CaptureFrame: 0, Position: core.Position3D{X: 3000, Y: 4000}, Direction: 0, Alpha: 1.0,
	}))

	export := b.BuildExport()

	require.Len(t, export.Markers, 2)

	var customMarker, defaultMarker []any
	for _, m := range export.Markers {
		text := m[1].(string)
		if text == "Custom" {
			customMarker = m
		} else if text == "Default" {
			defaultMarker = m
		}
	}

	require.NotNil(t, customMarker, "custom marker not found")
	require.NotNil(t, defaultMarker, "default marker not found")

	// Custom marker should have the specified size and brush
	customSize := customMarker[8].([]float64)
	assert.Equal(t, []float64{2.5, 3.0}, customSize, "custom marker should have size [2.5, 3.0]")
	assert.Equal(t, "SolidBorder", customMarker[10], "custom marker should have brush SolidBorder")

	// Default marker should have fallback size [1.0, 1.0]
	defaultSize := defaultMarker[8].([]float64)
	assert.Equal(t, []float64{1.0, 1.0}, defaultSize, "default marker should have size [1.0, 1.0]")
	assert.Equal(t, "Solid", defaultMarker[10], "default marker should have brush Solid")
}

func TestExtensionBuildExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName:    "Test",
		StartTime:      time.Now(),
		ExtensionBuild: "Wed Jul 28 08:28:28 2021",
	}, &core.World{WorldName: "Test"}))

	export := b.BuildExport()

	assert.Equal(t, "Wed Jul 28 08:28:28 2021", export.ExtensionBuild)
}

func TestTagsExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Test",
		StartTime:   time.Now(),
		Tag:         "Zeus",
	}, &core.World{WorldName: "Test"}))

	export := b.BuildExport()

	assert.Equal(t, "Zeus", export.Tags)
}

func TestTimesExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Test",
		StartTime:   time.Now(),
	}, &core.World{WorldName: "Test"}))

	// Record time states
	require.NoError(t, b.RecordTimeState(&core.TimeState{
		CaptureFrame:   0,
		SystemTimeUTC:  "2026-01-25T18:46:48.850",
		MissionDate:    "2035-06-24T12:26:00",
		TimeMultiplier: 1.0,
		MissionTime:    1597.56,
	}))
	require.NoError(t, b.RecordTimeState(&core.TimeState{
		CaptureFrame:   10,
		SystemTimeUTC:  "2026-01-25T18:47:18.854",
		MissionDate:    "2035-06-24T12:27:00",
		TimeMultiplier: 2.0,
		MissionTime:    1627.58,
	}))

	export := b.BuildExport()

	require.Len(t, export.Times, 2)

	// Verify first time state
	assert.Equal(t, uint(0), export.Times[0].FrameNum)
	assert.Equal(t, "2026-01-25T18:46:48.850", export.Times[0].SystemTimeUTC)
	assert.Equal(t, "2035-06-24T12:26:00", export.Times[0].Date)
	assert.Equal(t, float32(1.0), export.Times[0].TimeMultiplier)
	assert.Equal(t, float32(1597.56), export.Times[0].Time)

	// Verify second time state
	assert.Equal(t, uint(10), export.Times[1].FrameNum)
	assert.Equal(t, "2026-01-25T18:47:18.854", export.Times[1].SystemTimeUTC)
	assert.Equal(t, "2035-06-24T12:27:00", export.Times[1].Date)
	assert.Equal(t, float32(2.0), export.Times[1].TimeMultiplier)
	assert.Equal(t, float32(1627.58), export.Times[1].Time)
}

func TestTimesExportEmpty(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName: "Test",
		StartTime:   time.Now(),
	}, &core.World{WorldName: "Test"}))

	// No time states recorded

	export := b.BuildExport()

	assert.Empty(t, export.Times)
}

func TestTimesExportJSON(t *testing.T) {
	tempDir := t.TempDir()
	b := New(config.MemoryConfig{OutputDir: tempDir, CompressOutput: false})

	require.NoError(t, b.StartMission(&core.Mission{
		MissionName:    "Times Test",
		StartTime:      time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		ExtensionBuild: "Build 123",
		Tag:            "TvT",
	}, &core.World{WorldName: "Altis"}))

	require.NoError(t, b.RecordTimeState(&core.TimeState{
		CaptureFrame:   0,
		SystemTimeUTC:  "2024-03-15T14:30:00.000",
		MissionDate:    "2035-06-24T06:00:00",
		TimeMultiplier: 1.0,
		MissionTime:    0,
	}))

	require.NoError(t, b.EndMission())

	// Read and parse the JSON file
	matches, err := filepath.Glob(filepath.Join(tempDir, "*.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Verify extensionBuild
	assert.Equal(t, "Build 123", raw["extensionBuild"])

	// Verify tags
	assert.Equal(t, "TvT", raw["tags"])

	// Verify times array structure
	times, ok := raw["times"].([]any)
	require.True(t, ok, "times should be an array")
	require.Len(t, times, 1)

	timeEntry := times[0].(map[string]any)
	assert.Equal(t, float64(0), timeEntry["frameNum"])
	assert.Equal(t, "2024-03-15T14:30:00.000", timeEntry["systemTimeUTC"])
	assert.Equal(t, "2035-06-24T06:00:00", timeEntry["date"])
	assert.Equal(t, float64(1.0), timeEntry["timeMultiplier"])
	assert.Equal(t, float64(0), timeEntry["time"])
}

func TestPolylineMarkerExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))

	// Add a polyline marker
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "polyline_1", Text: "", MarkerType: "mil_dot", Color: "000000",
		Side: "GLOBAL", Shape: "POLYLINE", OwnerID: 0, CaptureFrame: 71,
		Polyline: core.Polyline{
			{X: 8261.73, Y: 18543.5},
			{X: 8160.17, Y: 18527.4},
			{X: 8051.69, Y: 18497.4},
		},
		Direction: 0, Alpha: 1.0, Brush: "Solid",
	}))

	export := b.BuildExport()

	require.Len(t, export.Markers, 1)
	marker := export.Markers[0]

	// Verify shape at index 9
	assert.Equal(t, "POLYLINE", marker[9])

	// Verify positions at index 7 is frame-based: [[frameNum, [[x1,y1], [x2,y2], ...], direction, alpha]]
	positions, ok := marker[7].([][]any)
	require.True(t, ok, "positions should be [][]any, got %T", marker[7])
	require.Len(t, positions, 1) // Single frame entry for polylines

	frameEntry := positions[0]
	assert.EqualValues(t, 71, frameEntry[0])   // frameNum
	assert.EqualValues(t, 0, frameEntry[2])    // direction
	assert.EqualValues(t, 1.0, frameEntry[3])  // alpha

	// frameEntry[1] contains the coordinate array
	coords, ok := frameEntry[1].([][]float64)
	require.True(t, ok, "polyline coords should be [][]float64, got %T", frameEntry[1])
	require.Len(t, coords, 3)

	assert.Equal(t, 8261.73, coords[0][0])
	assert.Equal(t, 18543.5, coords[0][1])
	assert.Equal(t, 8160.17, coords[1][0])
	assert.Equal(t, 18527.4, coords[1][1])
	assert.Equal(t, 8051.69, coords[2][0])
	assert.Equal(t, 18497.4, coords[2][1])
}

func TestMarkerSideValues(t *testing.T) {
	// Test that sideToIndex correctly handles side string values
	b := New(config.MemoryConfig{})
	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Test", StartTime: time.Now()}, &core.World{WorldName: "Test"}))

	// Add markers with string side values
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "east_marker", Text: "East", MarkerType: "mil_dot", Color: "800000",
		Side: "EAST", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 1000}, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "west_marker", Text: "West", MarkerType: "mil_dot", Color: "004C99",
		Side: "WEST", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 2000, Y: 2000}, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "guer_marker", Text: "Guer", MarkerType: "mil_dot", Color: "008000",
		Side: "GUER", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 3000, Y: 3000}, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "civ_marker", Text: "Civ", MarkerType: "mil_dot", Color: "660080",
		Side: "CIV", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 4000, Y: 4000}, Alpha: 1.0,
	}))
	require.NoError(t, b.AddMarker(&core.Marker{
		MarkerName: "global_marker", Text: "Global", MarkerType: "mil_dot", Color: "000000",
		Side: "UNKNOWN", Shape: "ICON", CaptureFrame: 0, Position: core.Position3D{X: 5000, Y: 5000}, Alpha: 1.0,
	}))

	export := b.BuildExport()

	// Build a map of marker name to side index for easier assertions
	markerSides := make(map[string]int)
	for _, marker := range export.Markers {
		text := marker[1].(string)
		sideIdx := marker[6].(int)
		markerSides[text] = sideIdx
	}

	assert.Equal(t, 0, markerSides["East"], "Side 'EAST' should map to 0")
	assert.Equal(t, 1, markerSides["West"], "Side 'WEST' should map to 1")
	assert.Equal(t, 2, markerSides["Guer"], "Side 'GUER' should map to 2")
	assert.Equal(t, 3, markerSides["Civ"], "Side 'CIV' should map to 3")
	assert.Equal(t, -1, markerSides["Global"], "Unknown side should map to GLOBAL (-1)")
}
