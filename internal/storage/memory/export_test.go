// internal/storage/memory/export_test.go
package memory

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/model/core"
)

func TestBoolToInt(t *testing.T) {
	tests := []struct {
		input    bool
		expected int
	}{
		{true, 1},
		{false, 0},
	}

	for _, tt := range tests {
		result := boolToInt(tt.input)
		if result != tt.expected {
			t.Errorf("boolToInt(%v) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestBuildExport(t *testing.T) {
	b := New(config.MemoryConfig{})

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

	_ = b.StartMission(mission, world)

	// Add soldier with states and fired events
	soldier := &core.Soldier{
		ID:    1,
		UnitName:  "Player1",
		GroupID:   "Alpha",
		Side:      "WEST",
		IsPlayer:  true,
		JoinFrame: 0,
	}
	_ = b.AddSoldier(soldier)

	state1 := &core.SoldierState{
		SoldierID:     soldier.ID,
				CaptureFrame:  0,
		Position:      core.Position3D{X: 1000, Y: 2000, Z: 100},
		Bearing:       90,
		Lifestate:     1,
		UnitName:      "Player1",
		IsPlayer:      true,
		CurrentRole:   "Rifleman",
	}
	state2 := &core.SoldierState{
		SoldierID:     soldier.ID,
				CaptureFrame:  10,
		Position:      core.Position3D{X: 1050, Y: 2050, Z: 100},
		Bearing:       95,
		Lifestate:     1,
		UnitName:      "Player1",
		IsPlayer:      true,
		CurrentRole:   "Rifleman",
	}
	_ = b.RecordSoldierState(state1)
	_ = b.RecordSoldierState(state2)

	fired := &core.FiredEvent{
		SoldierID:     soldier.ID,
				CaptureFrame:  5,
		Weapon:        "arifle_MX_F",
		Magazine:      "30Rnd_65x39_caseless_mag",
		FiringMode:    "Single",
		StartPos:      core.Position3D{X: 1000, Y: 2000, Z: 1.5},
		EndPos:        core.Position3D{X: 1100, Y: 2100, Z: 1.5},
	}
	_ = b.RecordFiredEvent(fired)

	// Add vehicle with states
	vehicle := &core.Vehicle{
		ID:      10,
		ClassName:   "B_MRAP_01_F",
		DisplayName: "Hunter",
		OcapType:    "car",
		JoinFrame:   2,
	}
	_ = b.AddVehicle(vehicle)

	vState := &core.VehicleState{
		VehicleID:     vehicle.ID,
				CaptureFrame:  5,
		Position:      core.Position3D{X: 3000, Y: 4000, Z: 50},
		Bearing:       180,
		IsAlive:       true,
		Crew:          "[[1,\"driver\"]]",
	}
	_ = b.RecordVehicleState(vState)

	// Add marker with states
	marker := &core.Marker{
		MarkerName:   "objective_1",
		Text:         "Objective Alpha",
		MarkerType:   "mil_objective",
		Color:        "ColorBLUFOR",
		Side:         "WEST",
		Shape:        "ICON",
		CaptureFrame: 0,
		Position:     core.Position3D{X: 5000, Y: 6000, Z: 0},
		Direction:    0,
		Alpha:        1.0,
	}
	_ = b.AddMarker(marker)

	mState := &core.MarkerState{
		MarkerID:     marker.ID,
		CaptureFrame: 20,
		Position:     core.Position3D{X: 5100, Y: 6100, Z: 0},
		Direction:    45,
		Alpha:        0.8,
	}
	_ = b.RecordMarkerState(mState)

	// Add general event
	evt := &core.GeneralEvent{
		CaptureFrame: 15,
		Name:         "connected",
		Message:      "Player1 connected",
		ExtraData:    map[string]any{"uid": "12345"},
	}
	_ = b.RecordGeneralEvent(evt)

	// Build export
	export := b.buildExport()

	// Verify mission metadata
	if export.MissionName != "Test Mission" {
		t.Errorf("expected MissionName='Test Mission', got '%s'", export.MissionName)
	}
	if export.MissionAuthor != "Test Author" {
		t.Errorf("expected MissionAuthor='Test Author', got '%s'", export.MissionAuthor)
	}
	if export.WorldName != "Altis" {
		t.Errorf("expected WorldName='Altis', got '%s'", export.WorldName)
	}
	if export.CaptureDelay != 1.0 {
		t.Errorf("expected CaptureDelay=1.0, got %f", export.CaptureDelay)
	}
	if export.AddonVersion != "1.0.0" {
		t.Errorf("expected AddonVersion='1.0.0', got '%s'", export.AddonVersion)
	}
	if export.ExtensionVersion != "2.0.0" {
		t.Errorf("expected ExtensionVersion='2.0.0', got '%s'", export.ExtensionVersion)
	}

	// Verify EndFrame is maximum frame from states
	if export.EndFrame != 10 {
		t.Errorf("expected EndFrame=10, got %d", export.EndFrame)
	}

	// Verify entities
	if len(export.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(export.Entities))
	}

	// Find soldier entity
	var soldierEntity, vehicleEntity *EntityJSON
	for i := range export.Entities {
		if export.Entities[i].Type == "unit" {
			soldierEntity = &export.Entities[i]
		} else {
			vehicleEntity = &export.Entities[i]
		}
	}

	if soldierEntity == nil {
		t.Fatal("soldier entity not found")
	}
	if soldierEntity.ID != 1 {
		t.Errorf("expected soldier ID=1, got %d", soldierEntity.ID)
	}
	if soldierEntity.Name != "Player1" {
		t.Errorf("expected soldier Name='Player1', got '%s'", soldierEntity.Name)
	}
	if soldierEntity.Group != "Alpha" {
		t.Errorf("expected soldier Group='Alpha', got '%s'", soldierEntity.Group)
	}
	if soldierEntity.Side != "WEST" {
		t.Errorf("expected soldier Side='WEST', got '%s'", soldierEntity.Side)
	}
	if soldierEntity.IsPlayer != 1 {
		t.Errorf("expected soldier IsPlayer=1, got %d", soldierEntity.IsPlayer)
	}
	if len(soldierEntity.Positions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(soldierEntity.Positions))
	}
	if len(soldierEntity.FramesFired) != 1 {
		t.Errorf("expected 1 framesFired, got %d", len(soldierEntity.FramesFired))
	}

	if vehicleEntity == nil {
		t.Fatal("vehicle entity not found")
	}
	if vehicleEntity.ID != 10 {
		t.Errorf("expected vehicle ID=10, got %d", vehicleEntity.ID)
	}
	if vehicleEntity.Name != "Hunter" {
		t.Errorf("expected vehicle Name='Hunter', got '%s'", vehicleEntity.Name)
	}
	if vehicleEntity.Type != "car" {
		t.Errorf("expected vehicle Type='car', got '%s'", vehicleEntity.Type)
	}
	if vehicleEntity.Class != "B_MRAP_01_F" {
		t.Errorf("expected vehicle Class='B_MRAP_01_F', got '%s'", vehicleEntity.Class)
	}
	if len(vehicleEntity.Positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(vehicleEntity.Positions))
	}

	// Verify events
	if len(export.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(export.Events))
	}
	if export.Events[0].Type != "connected" {
		t.Errorf("expected event Type='connected', got '%s'", export.Events[0].Type)
	}
	if export.Events[0].Frame != 15 {
		t.Errorf("expected event Frame=15, got %d", export.Events[0].Frame)
	}

	// Verify markers
	if len(export.Markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(export.Markers))
	}
	if export.Markers[0].Name != "Objective Alpha" {
		t.Errorf("expected marker Name='Objective Alpha', got '%s'", export.Markers[0].Name)
	}
	if export.Markers[0].Type != "mil_objective" {
		t.Errorf("expected marker Type='mil_objective', got '%s'", export.Markers[0].Type)
	}
	// Initial position + 1 state change = 2 positions
	if len(export.Markers[0].Positions) != 2 {
		t.Errorf("expected 2 marker positions, got %d", len(export.Markers[0].Positions))
	}
}

func TestExportJSON(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{
		OutputDir:      tempDir,
		CompressOutput: false,
	})

	mission := &core.Mission{
		MissionName: "Export Test",
		Author:      "Author",
		StartTime:   time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
	}
	world := &core.World{WorldName: "Tanoa"}

	_ = b.StartMission(mission, world)
	_ = b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Test"})

	// EndMission triggers export
	if err := b.EndMission(); err != nil {
		t.Fatalf("EndMission failed: %v", err)
	}

	// Check file was created
	pattern := filepath.Join(tempDir, "Export_Test_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 JSON file, found %d", len(matches))
	}

	// Read and validate JSON
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var export OcapExport
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if export.MissionName != "Export Test" {
		t.Errorf("expected MissionName='Export Test', got '%s'", export.MissionName)
	}
}

func TestExportGzipJSON(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{
		OutputDir:      tempDir,
		CompressOutput: true,
	})

	mission := &core.Mission{
		MissionName: "Gzip Test",
		Author:      "Author",
		StartTime:   time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
	}
	world := &core.World{WorldName: "Livonia"}

	_ = b.StartMission(mission, world)
	_ = b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Test"})

	if err := b.EndMission(); err != nil {
		t.Fatalf("EndMission failed: %v", err)
	}

	// Check .json.gz file was created
	pattern := filepath.Join(tempDir, "Gzip_Test_*.json.gz")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 .json.gz file, found %d", len(matches))
	}

	// Read and decompress
	f, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("failed to open gzip file: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	var export OcapExport
	if err := json.NewDecoder(gzReader).Decode(&export); err != nil {
		t.Fatalf("failed to decode gzipped JSON: %v", err)
	}

	if export.MissionName != "Gzip Test" {
		t.Errorf("expected MissionName='Gzip Test', got '%s'", export.MissionName)
	}
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
		b := New(config.MemoryConfig{
			OutputDir:      tempDir,
			CompressOutput: tt.compress,
		})

		mission := &core.Mission{
			MissionName: tt.missionName,
			StartTime:   time.Now(),
		}
		world := &core.World{WorldName: "Test"}

		_ = b.StartMission(mission, world)
		_ = b.EndMission()

		// Find the file
		pattern := filepath.Join(tempDir, "*"+tt.expectedSuffix)
		matches, _ := filepath.Glob(pattern)
		if len(matches) == 0 {
			t.Errorf("no file with suffix %s found for mission '%s'", tt.expectedSuffix, tt.missionName)
			continue
		}

		// Check filename doesn't contain spaces or colons
		filename := filepath.Base(matches[len(matches)-1])
		if strings.Contains(filename, " ") {
			t.Errorf("filename contains spaces: %s", filename)
		}
		if strings.Contains(filename, ":") {
			t.Errorf("filename contains colons: %s", filename)
		}
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

	_ = b.StartMission(mission, world)
	if err := b.EndMission(); err != nil {
		t.Fatalf("EndMission failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		t.Error("output directory was not created")
	}

	// Verify file exists in nested directory
	pattern := filepath.Join(nonExistentDir, "*.json")
	matches, _ := filepath.Glob(pattern)
	if len(matches) != 1 {
		t.Errorf("expected 1 file in nested dir, found %d", len(matches))
	}
}

func TestSoldierPositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	soldier := &core.Soldier{ID: 1, UnitName: "Player1", IsPlayer: true}
	_ = b.AddSoldier(soldier)

	inVehicleID := uint16(100)
	state := &core.SoldierState{
		SoldierID:         soldier.ID,
		CaptureFrame:      5,
		Position:          core.Position3D{X: 1234.56, Y: 7890.12, Z: 50},
		Bearing:           270,
		Lifestate:         1,
		InVehicleObjectID: &inVehicleID,
		UnitName:          "Player1_InVeh",
		IsPlayer:          true,
		CurrentRole:       "Gunner",
	}
	_ = b.RecordSoldierState(state)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if len(entity.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(entity.Positions))
	}

	pos := entity.Positions[0]
	// Position format: [[x, y], bearing, lifestate, inVehicleObjectID, unitName, isPlayer, currentRole]
	if len(pos) != 7 {
		t.Fatalf("expected position array length 7, got %d", len(pos))
	}

	// Check coordinate array
	coords, ok := pos[0].([]float64)
	if !ok {
		t.Fatal("position[0] is not []float64")
	}
	if coords[0] != 1234.56 || coords[1] != 7890.12 {
		t.Errorf("expected coords [1234.56, 7890.12], got %v", coords)
	}

	// Check bearing
	if pos[1].(uint16) != 270 {
		t.Errorf("expected bearing=270, got %v", pos[1])
	}

	// Check lifestate
	if pos[2].(uint8) != 1 {
		t.Errorf("expected lifestate=1, got %v", pos[2])
	}

	// Check isPlayer (should be 1 for true)
	if pos[5].(int) != 1 {
		t.Errorf("expected isPlayer=1, got %v", pos[5])
	}
}

func TestVehiclePositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	vehicle := &core.Vehicle{ID: 10, DisplayName: "Hunter", OcapType: "car"}
	_ = b.AddVehicle(vehicle)

	state := &core.VehicleState{
		VehicleID:     vehicle.ID,
				CaptureFrame:  3,
		Position:      core.Position3D{X: 5000, Y: 6000, Z: 25},
		Bearing:       45,
		IsAlive:       true,
		Crew:          "[[1,\"driver\"],[2,\"gunner\"]]",
	}
	_ = b.RecordVehicleState(state)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if len(entity.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(entity.Positions))
	}

	pos := entity.Positions[0]
	// Vehicle position format: [[x, y], bearing, isAlive, crew]
	if len(pos) != 4 {
		t.Fatalf("expected position array length 4, got %d", len(pos))
	}

	// Check coordinate array
	coords, ok := pos[0].([]float64)
	if !ok {
		t.Fatal("position[0] is not []float64")
	}
	if coords[0] != 5000 || coords[1] != 6000 {
		t.Errorf("expected coords [5000, 6000], got %v", coords)
	}

	// Check isAlive (should be 1 for true)
	if pos[2].(int) != 1 {
		t.Errorf("expected isAlive=1, got %v", pos[2])
	}
}

func TestFiredEventFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	soldier := &core.Soldier{ID: 1}
	_ = b.AddSoldier(soldier)

	fired := &core.FiredEvent{
		SoldierID:     soldier.ID,
				CaptureFrame:  100,
		Weapon:        "arifle_MX_F",
		Magazine:      "30Rnd_65x39_caseless_mag",
		FiringMode:    "FullAuto",
		StartPos:      core.Position3D{X: 100, Y: 200, Z: 1.5},
		EndPos:        core.Position3D{X: 300, Y: 400, Z: 1.8},
	}
	_ = b.RecordFiredEvent(fired)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if len(entity.FramesFired) != 1 {
		t.Fatalf("expected 1 framesFired, got %d", len(entity.FramesFired))
	}

	ff := entity.FramesFired[0]
	// Format: [captureFrame, [endX, endY], [startX, startY], weapon, magazine, firingMode]
	if len(ff) != 6 {
		t.Fatalf("expected framesFired array length 6, got %d", len(ff))
	}

	if ff[0].(uint) != 100 {
		t.Errorf("expected captureFrame=100, got %v", ff[0])
	}

	endPos, ok := ff[1].([]float64)
	if !ok {
		t.Fatal("endPos is not []float64")
	}
	if endPos[0] != 300 || endPos[1] != 400 {
		t.Errorf("expected endPos [300, 400], got %v", endPos)
	}

	startPos, ok := ff[2].([]float64)
	if !ok {
		t.Fatal("startPos is not []float64")
	}
	if startPos[0] != 100 || startPos[1] != 200 {
		t.Errorf("expected startPos [100, 200], got %v", startPos)
	}

	if ff[3].(string) != "arifle_MX_F" {
		t.Errorf("expected weapon='arifle_MX_F', got %v", ff[3])
	}
	if ff[4].(string) != "30Rnd_65x39_caseless_mag" {
		t.Errorf("expected magazine='30Rnd_65x39_caseless_mag', got %v", ff[4])
	}
	if ff[5].(string) != "FullAuto" {
		t.Errorf("expected firingMode='FullAuto', got %v", ff[5])
	}
}

func TestMarkerPositionFormat(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	marker := &core.Marker{
		MarkerName:   "test_marker",
		Text:         "Test Marker",
		MarkerType:   "mil_dot",
		Color:        "ColorRed",
		Side:         "EAST",
		Shape:        "ICON",
		CaptureFrame: 0,
		Position:     core.Position3D{X: 1000, Y: 2000, Z: 0},
		Direction:    90,
		Alpha:        1.0,
	}
	_ = b.AddMarker(marker)

	state := &core.MarkerState{
		MarkerID:     marker.ID,
		CaptureFrame: 50,
		Position:     core.Position3D{X: 1100, Y: 2100, Z: 0},
		Direction:    180,
		Alpha:        0.5,
	}
	_ = b.RecordMarkerState(state)

	export := b.buildExport()

	if len(export.Markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(export.Markers))
	}

	m := export.Markers[0]
	// Should have initial position + 1 state = 2 positions
	if len(m.Positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(m.Positions))
	}

	// Check initial position format: [frame, [x, y], direction, alpha]
	initialPos := m.Positions[0]
	if len(initialPos) != 4 {
		t.Fatalf("expected position array length 4, got %d", len(initialPos))
	}

	if initialPos[0].(uint) != 0 {
		t.Errorf("expected initial frame=0, got %v", initialPos[0])
	}

	coords, ok := initialPos[1].([]float64)
	if !ok {
		t.Fatal("coords is not []float64")
	}
	if coords[0] != 1000 || coords[1] != 2000 {
		t.Errorf("expected initial coords [1000, 2000], got %v", coords)
	}

	// Check state position
	statePos := m.Positions[1]
	if statePos[0].(uint) != 50 {
		t.Errorf("expected state frame=50, got %v", statePos[0])
	}
}

func TestEmptyExport(t *testing.T) {
	tempDir := t.TempDir()

	b := New(config.MemoryConfig{
		OutputDir:      tempDir,
		CompressOutput: false,
	})

	mission := &core.Mission{
		MissionName: "Empty Mission",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Test"}

	_ = b.StartMission(mission, world)
	// No entities, events, or markers added

	if err := b.EndMission(); err != nil {
		t.Fatalf("EndMission failed: %v", err)
	}

	// Find and validate the file
	pattern := filepath.Join(tempDir, "*.json")
	matches, _ := filepath.Glob(pattern)
	if len(matches) != 1 {
		t.Fatalf("expected 1 file, found %d", len(matches))
	}

	data, _ := os.ReadFile(matches[0])
	var export OcapExport
	_ = json.Unmarshal(data, &export)

	if len(export.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(export.Entities))
	}
	if len(export.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(export.Events))
	}
	if len(export.Markers) != 0 {
		t.Errorf("expected 0 markers, got %d", len(export.Markers))
	}
}

func TestMaxFrameCalculation(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	// Add soldier with states at different frames
	soldier := &core.Soldier{ID: 1}
	_ = b.AddSoldier(soldier)

	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier.ID, CaptureFrame: 10})
	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier.ID, CaptureFrame: 50})
	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier.ID, CaptureFrame: 30})

	// Add vehicle with states at higher frames
	vehicle := &core.Vehicle{ID: 10}
	_ = b.AddVehicle(vehicle)

	_ = b.RecordVehicleState(&core.VehicleState{VehicleID: vehicle.ID, CaptureFrame: 100})
	_ = b.RecordVehicleState(&core.VehicleState{VehicleID: vehicle.ID, CaptureFrame: 75})

	export := b.buildExport()

	// EndFrame should be max of all state frames (100)
	if export.EndFrame != 100 {
		t.Errorf("expected EndFrame=100, got %d", export.EndFrame)
	}
}

func TestSoldierWithoutVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	soldier := &core.Soldier{ID: 1, UnitName: "Infantry", IsPlayer: false}
	_ = b.AddSoldier(soldier)

	// State with nil InVehicleObjectID (soldier on foot)
	state := &core.SoldierState{
		SoldierID:         soldier.ID,
		CaptureFrame:      0,
		Position:          core.Position3D{X: 100, Y: 200, Z: 10},
		Bearing:           45,
		Lifestate:         1,
		InVehicleObjectID: nil, // Not in vehicle
		UnitName:          "Infantry",
		IsPlayer:          false,
		CurrentRole:       "Rifleman",
	}
	_ = b.RecordSoldierState(state)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	pos := entity.Positions[0]

	// InVehicleObjectID should be nil (*uint16 nil stored in any)
	if pos[3] != (*uint16)(nil) {
		t.Errorf("expected InVehicleObjectID=nil, got %v", pos[3])
	}

	// IsPlayer should be 0 for non-player
	if entity.IsPlayer != 0 {
		t.Errorf("expected IsPlayer=0, got %d", entity.IsPlayer)
	}
	if pos[5].(int) != 0 {
		t.Errorf("expected position isPlayer=0, got %v", pos[5])
	}
}

func TestDeadVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	vehicle := &core.Vehicle{ID: 5, DisplayName: "Destroyed Tank", OcapType: "tank", ClassName: "B_MBT_01_cannon_F"}
	_ = b.AddVehicle(vehicle)

	state := &core.VehicleState{
		VehicleID:     vehicle.ID,
				CaptureFrame:  50,
		Position:      core.Position3D{X: 2000, Y: 3000, Z: 0},
		Bearing:       90,
		IsAlive:       false, // Destroyed
		Crew:          "[]",  // Empty crew (everyone dead/bailed)
	}
	_ = b.RecordVehicleState(state)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if entity.Side != "UNKNOWN" {
		t.Errorf("expected Side='UNKNOWN' for vehicle, got '%s'", entity.Side)
	}
	if entity.IsPlayer != 0 {
		t.Errorf("expected IsPlayer=0 for vehicle, got %d", entity.IsPlayer)
	}

	pos := entity.Positions[0]
	// IsAlive should be 0 for destroyed vehicle
	if pos[2].(int) != 0 {
		t.Errorf("expected isAlive=0, got %v", pos[2])
	}
}

func TestMultipleEntitiesExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	// Add multiple soldiers
	soldier1 := &core.Soldier{ID: 1, UnitName: "Alpha1", GroupID: "Alpha", Side: "WEST", IsPlayer: true, JoinFrame: 0}
	soldier2 := &core.Soldier{ID: 2, UnitName: "Alpha2", GroupID: "Alpha", Side: "WEST", IsPlayer: false, JoinFrame: 5}
	soldier3 := &core.Soldier{ID: 3, UnitName: "Bravo1", GroupID: "Bravo", Side: "EAST", IsPlayer: false, JoinFrame: 10}
	_ = b.AddSoldier(soldier1)
	_ = b.AddSoldier(soldier2)
	_ = b.AddSoldier(soldier3)

	// Add states for each soldier
	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier1.ID, CaptureFrame: 0, Position: core.Position3D{X: 100, Y: 100}, Lifestate: 1})
	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier2.ID, CaptureFrame: 5, Position: core.Position3D{X: 110, Y: 100}, Lifestate: 1})
	_ = b.RecordSoldierState(&core.SoldierState{SoldierID: soldier3.ID, CaptureFrame: 10, Position: core.Position3D{X: 500, Y: 500}, Lifestate: 1})

	// Add multiple vehicles
	vehicle1 := &core.Vehicle{ID: 10, DisplayName: "Hunter", OcapType: "car", JoinFrame: 0}
	vehicle2 := &core.Vehicle{ID: 11, DisplayName: "Heli", OcapType: "heli", JoinFrame: 20}
	_ = b.AddVehicle(vehicle1)
	_ = b.AddVehicle(vehicle2)

	_ = b.RecordVehicleState(&core.VehicleState{VehicleID: vehicle1.ID, CaptureFrame: 0, Position: core.Position3D{X: 200, Y: 200}, IsAlive: true})
	_ = b.RecordVehicleState(&core.VehicleState{VehicleID: vehicle2.ID, CaptureFrame: 20, Position: core.Position3D{X: 300, Y: 300}, IsAlive: true})

	export := b.buildExport()

	// 3 soldiers + 2 vehicles = 5 entities
	if len(export.Entities) != 5 {
		t.Errorf("expected 5 entities, got %d", len(export.Entities))
	}

	// Count by type
	unitCount := 0
	vehicleCount := 0
	for _, e := range export.Entities {
		if e.Type == "unit" {
			unitCount++
		} else {
			vehicleCount++
		}
	}
	if unitCount != 3 {
		t.Errorf("expected 3 units, got %d", unitCount)
	}
	if vehicleCount != 2 {
		t.Errorf("expected 2 vehicles, got %d", vehicleCount)
	}
}

func TestEventWithoutExtraData(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	// Event without ExtraData - tests the event export path
	evt := &core.GeneralEvent{
		CaptureFrame: 100,
		Name:         "endMission",
		Message:      "Mission ended",
		ExtraData:    nil,
	}
	_ = b.RecordGeneralEvent(evt)

	export := b.buildExport()

	if len(export.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(export.Events))
	}

	if export.Events[0].Type != "endMission" {
		t.Errorf("expected Type='endMission', got '%s'", export.Events[0].Type)
	}
	if export.Events[0].Frame != 100 {
		t.Errorf("expected Frame=100, got %d", export.Events[0].Frame)
	}
	if export.Events[0].Message != "Mission ended" {
		t.Errorf("expected Message='Mission ended', got '%s'", export.Events[0].Message)
	}
}

func TestMultipleMarkersExport(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	// Add multiple markers
	marker1 := &core.Marker{
		MarkerName: "obj_alpha", Text: "Alpha", MarkerType: "mil_objective", Color: "ColorBLUFOR", Side: "WEST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 1000, Y: 1000}, Direction: 0, Alpha: 1.0,
	}
	marker2 := &core.Marker{
		MarkerName: "obj_bravo", Text: "Bravo", MarkerType: "mil_objective", Color: "ColorOPFOR", Side: "EAST", Shape: "ICON",
		CaptureFrame: 0, Position: core.Position3D{X: 2000, Y: 2000}, Direction: 45, Alpha: 1.0,
	}
	_ = b.AddMarker(marker1)
	_ = b.AddMarker(marker2)

	// Add states to first marker only
	_ = b.RecordMarkerState(&core.MarkerState{MarkerID: marker1.ID, CaptureFrame: 10, Position: core.Position3D{X: 1100, Y: 1100}, Direction: 90, Alpha: 0.8})
	_ = b.RecordMarkerState(&core.MarkerState{MarkerID: marker1.ID, CaptureFrame: 20, Position: core.Position3D{X: 1200, Y: 1200}, Direction: 180, Alpha: 0.6})

	export := b.buildExport()

	if len(export.Markers) != 2 {
		t.Fatalf("expected 2 markers, got %d", len(export.Markers))
	}

	// Find markers by name
	var m1, m2 *MarkerJSON
	for i := range export.Markers {
		if export.Markers[i].Name == "Alpha" {
			m1 = &export.Markers[i]
		} else if export.Markers[i].Name == "Bravo" {
			m2 = &export.Markers[i]
		}
	}

	if m1 == nil || m2 == nil {
		t.Fatal("markers not found by name")
	}

	// Marker1 should have 3 positions (initial + 2 states)
	if len(m1.Positions) != 3 {
		t.Errorf("expected 3 positions for marker1, got %d", len(m1.Positions))
	}

	// Marker2 should have only 1 position (initial, no states)
	if len(m2.Positions) != 1 {
		t.Errorf("expected 1 position for marker2, got %d", len(m2.Positions))
	}

	// Verify marker properties
	if m1.Color != "ColorBLUFOR" {
		t.Errorf("expected marker1 Color='ColorBLUFOR', got '%s'", m1.Color)
	}
	if m2.Side != "EAST" {
		t.Errorf("expected marker2 Side='EAST', got '%s'", m2.Side)
	}
}

func TestMultipleFiredEvents(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	soldier := &core.Soldier{ID: 1, UnitName: "Shooter"}
	_ = b.AddSoldier(soldier)

	// Multiple fired events for same soldier
	_ = b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: soldier.ID, CaptureFrame: 10, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39", FiringMode: "Single",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 200, Y: 200},
	})
	_ = b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: soldier.ID, CaptureFrame: 15, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39", FiringMode: "FullAuto",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 250, Y: 250},
	})
	_ = b.RecordFiredEvent(&core.FiredEvent{
		SoldierID: soldier.ID, CaptureFrame: 50, Weapon: "launch_NLAW_F", Magazine: "NLAW_F", FiringMode: "Single",
		StartPos: core.Position3D{X: 100, Y: 100}, EndPos: core.Position3D{X: 500, Y: 500},
	})

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if len(entity.FramesFired) != 3 {
		t.Errorf("expected 3 framesFired, got %d", len(entity.FramesFired))
	}

	// Verify different weapons are recorded
	weapons := make(map[string]bool)
	for _, ff := range entity.FramesFired {
		weapons[ff[3].(string)] = true
	}
	if !weapons["arifle_MX_F"] || !weapons["launch_NLAW_F"] {
		t.Errorf("expected both weapons recorded, got %v", weapons)
	}
}

func TestVehicleWithJoinFrame(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{MissionName: "Test", StartTime: time.Now()}
	world := &core.World{WorldName: "Test"}
	_ = b.StartMission(mission, world)

	vehicle := &core.Vehicle{
		ID:      20,
		DisplayName: "Late Arrival",
		OcapType:    "plane",
		ClassName:   "B_Plane_Fighter_01_F",
		JoinFrame:   500, // Spawned late in the mission
	}
	_ = b.AddVehicle(vehicle)

	export := b.buildExport()

	if len(export.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(export.Entities))
	}

	entity := export.Entities[0]
	if entity.StartFrameNum != 500 {
		t.Errorf("expected StartFrameNum=500, got %d", entity.StartFrameNum)
	}
	if entity.Type != "plane" {
		t.Errorf("expected Type='plane', got '%s'", entity.Type)
	}
	if entity.Class != "B_Plane_Fighter_01_F" {
		t.Errorf("expected Class='B_Plane_Fighter_01_F', got '%s'", entity.Class)
	}
}
