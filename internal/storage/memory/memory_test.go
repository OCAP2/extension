// internal/storage/memory/memory_test.go
package memory

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/storage"
)

// Verify Backend implements storage.Backend interface
var _ storage.Backend = (*Backend)(nil)

// Verify Backend implements storage.Uploadable interface
var _ storage.Uploadable = (*Backend)(nil)

func TestNew(t *testing.T) {
	cfg := config.MemoryConfig{
		OutputDir:      "/tmp/test",
		CompressOutput: true,
	}
	b := New(cfg)

	if b == nil {
		t.Fatal("New returned nil")
	}
	if b.cfg.OutputDir != "/tmp/test" {
		t.Errorf("expected OutputDir=/tmp/test, got %s", b.cfg.OutputDir)
	}
	if !b.cfg.CompressOutput {
		t.Error("expected CompressOutput=true")
	}
	if b.soldiers == nil {
		t.Error("soldiers map not initialized")
	}
	if b.vehicles == nil {
		t.Error("vehicles map not initialized")
	}
	if b.markers == nil {
		t.Error("markers map not initialized")
	}
}

func TestInitAndClose(t *testing.T) {
	b := New(config.MemoryConfig{})

	if err := b.Init(); err != nil {
		t.Errorf("Init failed: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestStartMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{
		MissionName: "Test Mission",
		Author:      "Test Author",
		StartTime:   time.Now(),
	}
	world := &core.World{
		WorldName:   "Altis",
		DisplayName: "Altis",
	}

	// Add some data before starting
	soldier := &core.Soldier{ID: 1, UnitName: "Old Soldier"}
	_ = b.AddSoldier(soldier)

	// Start a new mission - should reset collections
	if err := b.StartMission(mission, world); err != nil {
		t.Fatalf("StartMission failed: %v", err)
	}

	if b.mission != mission {
		t.Error("mission not set")
	}
	if b.world != world {
		t.Error("world not set")
	}
	if len(b.soldiers) != 0 {
		t.Error("soldiers not reset")
	}
}

func TestAddSoldier(t *testing.T) {
	b := New(config.MemoryConfig{})

	s1 := &core.Soldier{
		ID:   1,
		UnitName: "Soldier One",
		Side:     "WEST",
		IsPlayer: true,
	}
	s2 := &core.Soldier{
		ID:   2,
		UnitName: "Soldier Two",
		Side:     "EAST",
		IsPlayer: false,
	}

	if err := b.AddSoldier(s1); err != nil {
		t.Fatalf("AddSoldier failed: %v", err)
	}
	if err := b.AddSoldier(s2); err != nil {
		t.Fatalf("AddSoldier failed: %v", err)
	}

	// IDs are ObjectIDs set by caller, not auto-assigned
	if s1.ID != 1 {
		t.Errorf("expected s1.ID=1 (ObjectID), got %d", s1.ID)
	}
	if s2.ID != 2 {
		t.Errorf("expected s2.ID=2 (ObjectID), got %d", s2.ID)
	}

	// Check storage
	if len(b.soldiers) != 2 {
		t.Errorf("expected 2 soldiers, got %d", len(b.soldiers))
	}
	if b.soldiers[1].Soldier.UnitName != "Soldier One" {
		t.Error("soldier 1 not stored correctly")
	}
	if b.soldiers[2].Soldier.UnitName != "Soldier Two" {
		t.Error("soldier 2 not stored correctly")
	}
}

func TestAddVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	v1 := &core.Vehicle{
		ID:      10,
		ClassName:   "B_MRAP_01_F",
		DisplayName: "Hunter",
	}
	v2 := &core.Vehicle{
		ID:      20,
		ClassName:   "B_Heli_Light_01_F",
		DisplayName: "MH-9 Hummingbird",
	}

	if err := b.AddVehicle(v1); err != nil {
		t.Fatalf("AddVehicle failed: %v", err)
	}
	if err := b.AddVehicle(v2); err != nil {
		t.Fatalf("AddVehicle failed: %v", err)
	}

	// IDs are ObjectIDs set by caller, not auto-assigned
	if v1.ID != 10 {
		t.Errorf("expected v1.ID=10 (ObjectID), got %d", v1.ID)
	}
	if v2.ID != 20 {
		t.Errorf("expected v2.ID=20 (ObjectID), got %d", v2.ID)
	}

	if len(b.vehicles) != 2 {
		t.Errorf("expected 2 vehicles, got %d", len(b.vehicles))
	}
}

func TestAddMarker(t *testing.T) {
	b := New(config.MemoryConfig{})

	m1 := &core.Marker{
		MarkerName: "marker_1",
		Text:       "Base",
		MarkerType: "mil_dot",
		Color:      "ColorBLUFOR",
	}
	m2 := &core.Marker{
		MarkerName: "marker_2",
		Text:       "Objective",
		MarkerType: "mil_objective",
		Color:      "ColorRed",
	}

	if err := b.AddMarker(m1); err != nil {
		t.Fatalf("AddMarker failed: %v", err)
	}
	if err := b.AddMarker(m2); err != nil {
		t.Fatalf("AddMarker failed: %v", err)
	}

	// Markers don't have ObjectIDs; ID is not auto-assigned
	// Just verify storage works

	if len(b.markers) != 2 {
		t.Errorf("expected 2 markers, got %d", len(b.markers))
	}
	if b.markers["marker_1"].Marker.Text != "Base" {
		t.Error("marker_1 not stored correctly")
	}
}

func TestGetSoldierByObjectID(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{
		ID:   42,
		UnitName: "Test Soldier",
	}
	_ = b.AddSoldier(s)

	// Found case
	found, ok := b.GetSoldierByObjectID(42)
	if !ok {
		t.Fatal("soldier not found")
	}
	if found.UnitName != "Test Soldier" {
		t.Errorf("expected UnitName=Test Soldier, got %s", found.UnitName)
	}

	// Not found case
	_, ok = b.GetSoldierByObjectID(999)
	if ok {
		t.Error("expected not found for non-existent ObjectID")
	}
}

func TestGetVehicleByObjectID(t *testing.T) {
	b := New(config.MemoryConfig{})

	v := &core.Vehicle{
		ID:    100,
		ClassName: "B_Truck_01_transport_F",
	}
	_ = b.AddVehicle(v)

	// Found case
	found, ok := b.GetVehicleByObjectID(100)
	if !ok {
		t.Fatal("vehicle not found")
	}
	if found.ClassName != "B_Truck_01_transport_F" {
		t.Errorf("expected ClassName=B_Truck_01_transport_F, got %s", found.ClassName)
	}

	// Not found case
	_, ok = b.GetVehicleByObjectID(999)
	if ok {
		t.Error("expected not found for non-existent ObjectID")
	}
}

func TestGetMarkerByName(t *testing.T) {
	b := New(config.MemoryConfig{})

	m := &core.Marker{
		MarkerName: "respawn_west",
		Text:       "Respawn West",
	}
	_ = b.AddMarker(m)

	// Found case
	found, ok := b.GetMarkerByName("respawn_west")
	if !ok {
		t.Fatal("marker not found")
	}
	if found.Text != "Respawn West" {
		t.Errorf("expected Text=Respawn West, got %s", found.Text)
	}

	// Not found case
	_, ok = b.GetMarkerByName("nonexistent")
	if ok {
		t.Error("expected not found for non-existent marker name")
	}
}

func TestRecordSoldierState(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{ID: 1, UnitName: "Test"}
	_ = b.AddSoldier(s)

	state1 := &core.SoldierState{
		SoldierID:     s.ID,
				CaptureFrame:  0,
		Position:      core.Position3D{X: 100, Y: 200, Z: 0},
		Bearing:       90,
		Lifestate:     1,
	}
	state2 := &core.SoldierState{
		SoldierID:     s.ID,
				CaptureFrame:  1,
		Position:      core.Position3D{X: 105, Y: 205, Z: 0},
		Bearing:       95,
		Lifestate:     1,
	}

	if err := b.RecordSoldierState(state1); err != nil {
		t.Fatalf("RecordSoldierState failed: %v", err)
	}
	if err := b.RecordSoldierState(state2); err != nil {
		t.Fatalf("RecordSoldierState failed: %v", err)
	}

	record := b.soldiers[s.ID]
	if len(record.States) != 2 {
		t.Errorf("expected 2 states, got %d", len(record.States))
	}
	if record.States[0].CaptureFrame != 0 {
		t.Error("first state not recorded correctly")
	}
	if record.States[1].CaptureFrame != 1 {
		t.Error("second state not recorded correctly")
	}

	// Recording state for non-existent soldier should not error
	orphanState := &core.SoldierState{SoldierID: 999, CaptureFrame: 0}
	if err := b.RecordSoldierState(orphanState); err != nil {
		t.Errorf("RecordSoldierState should not error for missing soldier: %v", err)
	}
}

func TestRecordVehicleState(t *testing.T) {
	b := New(config.MemoryConfig{})

	v := &core.Vehicle{ID: 10, ClassName: "Car"}
	_ = b.AddVehicle(v)

	state := &core.VehicleState{
		VehicleID:     v.ID,
				CaptureFrame:  5,
		Position:      core.Position3D{X: 500, Y: 600, Z: 10},
		IsAlive:       true,
	}

	if err := b.RecordVehicleState(state); err != nil {
		t.Fatalf("RecordVehicleState failed: %v", err)
	}

	record := b.vehicles[v.ID]
	if len(record.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(record.States))
	}

	// Non-existent vehicle
	orphan := &core.VehicleState{VehicleID: 999}
	if err := b.RecordVehicleState(orphan); err != nil {
		t.Errorf("RecordVehicleState should not error for missing vehicle: %v", err)
	}
}

func TestRecordMarkerState(t *testing.T) {
	b := New(config.MemoryConfig{})

	m := &core.Marker{MarkerName: "test_marker"}
	_ = b.AddMarker(m)

	state := &core.MarkerState{
		MarkerID:     m.ID,
		CaptureFrame: 10,
		Position:     core.Position3D{X: 1000, Y: 2000, Z: 0},
		Direction:    45.0,
		Alpha:        1.0,
	}

	if err := b.RecordMarkerState(state); err != nil {
		t.Fatalf("RecordMarkerState failed: %v", err)
	}

	record := b.markers["test_marker"]
	if len(record.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(record.States))
	}

	// Non-existent marker
	orphan := &core.MarkerState{MarkerID: 999}
	if err := b.RecordMarkerState(orphan); err != nil {
		t.Errorf("RecordMarkerState should not error for missing marker: %v", err)
	}
}

func TestDeleteMarker(t *testing.T) {
	b := New(config.MemoryConfig{})

	m := &core.Marker{MarkerName: "grenade_1", EndFrame: -1}
	_ = b.AddMarker(m)

	// Delete marker at frame 100
	b.DeleteMarker("grenade_1", 100)

	record := b.markers["grenade_1"]
	if record.Marker.EndFrame != 100 {
		t.Errorf("expected EndFrame=100, got %d", record.Marker.EndFrame)
	}

	// Deleting non-existent marker should not panic
	b.DeleteMarker("nonexistent", 50)
}

func TestRecordFiredEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{ID: 1}
	_ = b.AddSoldier(s)

	fired := &core.FiredEvent{
		SoldierID:     s.ID,
				CaptureFrame:  100,
		Weapon:        "arifle_MX_F",
		Magazine:      "30Rnd_65x39_caseless_mag",
		FiringMode:    "Single",
		StartPos:      core.Position3D{X: 100, Y: 100, Z: 1},
		EndPos:        core.Position3D{X: 200, Y: 200, Z: 1},
	}

	if err := b.RecordFiredEvent(fired); err != nil {
		t.Fatalf("RecordFiredEvent failed: %v", err)
	}

	record := b.soldiers[s.ID]
	if len(record.FiredEvents) != 1 {
		t.Errorf("expected 1 fired event, got %d", len(record.FiredEvents))
	}
	if record.FiredEvents[0].Weapon != "arifle_MX_F" {
		t.Error("fired event not recorded correctly")
	}

	// Non-existent soldier
	orphan := &core.FiredEvent{SoldierID: 999}
	if err := b.RecordFiredEvent(orphan); err != nil {
		t.Errorf("RecordFiredEvent should not error for missing soldier: %v", err)
	}
}

func TestRecordGeneralEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.GeneralEvent{
		CaptureFrame: 50,
		Name:         "connected",
		Message:      "Player connected",
		ExtraData:    map[string]any{"playerName": "TestPlayer"},
	}

	if err := b.RecordGeneralEvent(evt); err != nil {
		t.Fatalf("RecordGeneralEvent failed: %v", err)
	}

	if len(b.generalEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.generalEvents))
	}
	if b.generalEvents[0].Name != "connected" {
		t.Error("event not recorded correctly")
	}
}

func TestRecordHitEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	victimID := uint(1)
	shooterID := uint(2)
	evt := &core.HitEvent{
		CaptureFrame:     75,
		VictimSoldierID:  &victimID,
		ShooterSoldierID: &shooterID,
		EventText:        "Player1 hit Player2",
		Distance:         150.5,
	}

	if err := b.RecordHitEvent(evt); err != nil {
		t.Fatalf("RecordHitEvent failed: %v", err)
	}

	if len(b.hitEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.hitEvents))
	}
}

func TestRecordKillEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	victimID := uint(1)
	killerID := uint(2)
	evt := &core.KillEvent{
		CaptureFrame:    100,
		VictimSoldierID: &victimID,
		KillerSoldierID: &killerID,
		EventText:       "Player2 killed Player1",
		Distance:        200.0,
	}

	if err := b.RecordKillEvent(evt); err != nil {
		t.Fatalf("RecordKillEvent failed: %v", err)
	}

	if len(b.killEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.killEvents))
	}
}

func TestRecordChatEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	soldierID := uint(1)
	evt := &core.ChatEvent{
		SoldierID:    &soldierID,
		CaptureFrame: 200,
		Channel:      "side",
		FromName:     "Player1",
		Message:      "Hello team",
	}

	if err := b.RecordChatEvent(evt); err != nil {
		t.Fatalf("RecordChatEvent failed: %v", err)
	}

	if len(b.chatEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.chatEvents))
	}
}

func TestRecordRadioEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	soldierID := uint(1)
	evt := &core.RadioEvent{
		SoldierID:    &soldierID,
		CaptureFrame: 300,
		Radio:        "ACRE_PRC152",
		RadioType:    "handheld",
		StartEnd:     "start",
		Channel:      1,
		Frequency:    152.0,
	}

	if err := b.RecordRadioEvent(evt); err != nil {
		t.Fatalf("RecordRadioEvent failed: %v", err)
	}

	if len(b.radioEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.radioEvents))
	}
}

func TestRecordServerFpsEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.ServerFpsEvent{
		CaptureFrame: 400,
		FpsAverage:   45.5,
		FpsMin:       30.0,
	}

	if err := b.RecordServerFpsEvent(evt); err != nil {
		t.Fatalf("RecordServerFpsEvent failed: %v", err)
	}

	if len(b.serverFpsEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.serverFpsEvents))
	}
}

func TestRecordTimeState(t *testing.T) {
	b := New(config.MemoryConfig{})

	now := time.Now()
	state := &core.TimeState{
		MissionID:      1,
		Time:           now,
		CaptureFrame:   100,
		SystemTimeUTC:  "2024-01-15T14:30:45.123",
		MissionDate:    "2035-06-15T06:00:00",
		TimeMultiplier: 2.0,
		MissionTime:    3600.5,
	}

	if err := b.RecordTimeState(state); err != nil {
		t.Fatalf("RecordTimeState failed: %v", err)
	}

	if len(b.timeStates) != 1 {
		t.Errorf("expected 1 state, got %d", len(b.timeStates))
	}
	if b.timeStates[0].CaptureFrame != 100 {
		t.Errorf("expected CaptureFrame=100, got %d", b.timeStates[0].CaptureFrame)
	}
	if b.timeStates[0].TimeMultiplier != 2.0 {
		t.Errorf("expected TimeMultiplier=2.0, got %f", b.timeStates[0].TimeMultiplier)
	}
}

func TestRecordAce3DeathEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	damageSource := uint(2)
	evt := &core.Ace3DeathEvent{
		SoldierID:          1,
		CaptureFrame:       500,
		Reason:             "cardiac_arrest",
		LastDamageSourceID: &damageSource,
	}

	if err := b.RecordAce3DeathEvent(evt); err != nil {
		t.Fatalf("RecordAce3DeathEvent failed: %v", err)
	}

	if len(b.ace3DeathEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.ace3DeathEvents))
	}
}

func TestRecordAce3UnconsciousEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.Ace3UnconsciousEvent{
		SoldierID:     1,
		CaptureFrame:  600,
		IsUnconscious: true,
	}

	if err := b.RecordAce3UnconsciousEvent(evt); err != nil {
		t.Fatalf("RecordAce3UnconsciousEvent failed: %v", err)
	}

	if len(b.ace3UnconsciousEvents) != 1 {
		t.Errorf("expected 1 event, got %d", len(b.ace3UnconsciousEvents))
	}
}

func TestConcurrentAccess(t *testing.T) {
	b := New(config.MemoryConfig{})

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperationsPerGoroutine := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				ocapID := uint16(id*1000 + j)
				s := &core.Soldier{ID: ocapID, UnitName: "Concurrent"}
				_ = b.AddSoldier(s)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				ocapID := uint16(id*1000 + j)
				_, _ = b.GetSoldierByObjectID(ocapID)
			}
		}(i)
	}

	wg.Wait()

	// Verify all soldiers were added
	expectedCount := numGoroutines * numOperationsPerGoroutine
	if len(b.soldiers) != expectedCount {
		t.Errorf("expected %d soldiers, got %d", expectedCount, len(b.soldiers))
	}
}

func TestIDsPreserved(t *testing.T) {
	b := New(config.MemoryConfig{})

	// IDs are ObjectIDs set by caller, not auto-assigned
	s := &core.Soldier{ID: 1}
	v := &core.Vehicle{ID: 10}
	m := &core.Marker{MarkerName: "test"}

	_ = b.AddSoldier(s)
	_ = b.AddVehicle(v)
	_ = b.AddMarker(m)

	// IDs should be preserved as set
	if s.ID != 1 {
		t.Errorf("expected soldier ID=1, got %d", s.ID)
	}
	if v.ID != 10 {
		t.Errorf("expected vehicle ID=10, got %d", v.ID)
	}
	// Markers are keyed by name, not ID
	if b.markers["test"] == nil {
		t.Error("marker not stored")
	}
}

func TestStartMissionResetsEverything(t *testing.T) {
	b := New(config.MemoryConfig{})

	// Populate with data
	_ = b.AddSoldier(&core.Soldier{ID: 1})
	_ = b.AddVehicle(&core.Vehicle{ID: 10})
	_ = b.AddMarker(&core.Marker{MarkerName: "m1"})
	_ = b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"})
	_ = b.RecordHitEvent(&core.HitEvent{})
	_ = b.RecordKillEvent(&core.KillEvent{})
	_ = b.RecordChatEvent(&core.ChatEvent{})
	_ = b.RecordRadioEvent(&core.RadioEvent{})
	_ = b.RecordServerFpsEvent(&core.ServerFpsEvent{})
	_ = b.RecordTimeState(&core.TimeState{})
	_ = b.RecordAce3DeathEvent(&core.Ace3DeathEvent{})
	_ = b.RecordAce3UnconsciousEvent(&core.Ace3UnconsciousEvent{})

	// Start new mission
	mission := &core.Mission{MissionName: "New"}
	world := &core.World{WorldName: "Stratis"}
	_ = b.StartMission(mission, world)

	if len(b.soldiers) != 0 {
		t.Error("soldiers not reset")
	}
	if len(b.vehicles) != 0 {
		t.Error("vehicles not reset")
	}
	if len(b.markers) != 0 {
		t.Error("markers not reset")
	}
	if len(b.generalEvents) != 0 {
		t.Error("generalEvents not reset")
	}
	if len(b.hitEvents) != 0 {
		t.Error("hitEvents not reset")
	}
	if len(b.killEvents) != 0 {
		t.Error("killEvents not reset")
	}
	if len(b.chatEvents) != 0 {
		t.Error("chatEvents not reset")
	}
	if len(b.radioEvents) != 0 {
		t.Error("radioEvents not reset")
	}
	if len(b.serverFpsEvents) != 0 {
		t.Error("serverFpsEvents not reset")
	}
	if len(b.timeStates) != 0 {
		t.Error("timeStates not reset")
	}
	if len(b.ace3DeathEvents) != 0 {
		t.Error("ace3DeathEvents not reset")
	}
	if len(b.ace3UnconsciousEvents) != 0 {
		t.Error("ace3UnconsciousEvents not reset")
	}
}

func TestGetExportedFilePath(t *testing.T) {
	b := New(config.MemoryConfig{
		OutputDir:      t.TempDir(),
		CompressOutput: true,
	})

	// Before export, should return empty
	if path := b.GetExportedFilePath(); path != "" {
		t.Errorf("expected empty path before export, got %s", path)
	}
}

func TestGetExportedFilePath_AfterExport(t *testing.T) {
	tmpDir := t.TempDir()
	b := New(config.MemoryConfig{
		OutputDir:      tmpDir,
		CompressOutput: true,
	})

	mission := &core.Mission{
		MissionName: "Test",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Altis"}

	_ = b.StartMission(mission, world)
	_ = b.EndMission()

	path := b.GetExportedFilePath()
	if path == "" {
		t.Fatal("expected non-empty path after export")
	}
	if !strings.HasPrefix(path, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, path)
	}
	if !strings.HasSuffix(path, ".json.gz") {
		t.Errorf("expected path to end with .json.gz, got %s", path)
	}
}

func TestGetExportedFilePath_UncompressedExport(t *testing.T) {
	tmpDir := t.TempDir()
	b := New(config.MemoryConfig{
		OutputDir:      tmpDir,
		CompressOutput: false,
	})

	mission := &core.Mission{
		MissionName: "Test",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Altis"}

	_ = b.StartMission(mission, world)
	_ = b.EndMission()

	path := b.GetExportedFilePath()
	if path == "" {
		t.Fatal("expected non-empty path after export")
	}
	if !strings.HasSuffix(path, ".json") {
		t.Errorf("expected path to end with .json, got %s", path)
	}
	if strings.HasSuffix(path, ".json.gz") {
		t.Errorf("expected path to NOT end with .json.gz for uncompressed, got %s", path)
	}
}

func TestGetExportMetadata(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{
		MissionName:  "Test Mission",
		CaptureDelay: 1.0,
		Tag:          "TvT",
	}
	world := &core.World{
		WorldName: "Altis",
	}

	_ = b.StartMission(mission, world)

	// Add some frames
	s := &core.Soldier{ID: 1}
	_ = b.AddSoldier(s)
	_ = b.RecordSoldierState(&core.SoldierState{
		SoldierID:     s.ID,
				CaptureFrame:  100,
	})

	meta := b.GetExportMetadata()

	if meta.WorldName != "Altis" {
		t.Errorf("expected WorldName=Altis, got %s", meta.WorldName)
	}
	if meta.MissionName != "Test Mission" {
		t.Errorf("expected MissionName=Test Mission, got %s", meta.MissionName)
	}
	if meta.Tag != "TvT" {
		t.Errorf("expected Tag=TvT, got %s", meta.Tag)
	}
	// Duration = endFrame * captureDelay / 1000 = 100 * 1.0 / 1000 = 0.1
	if meta.MissionDuration != 0.1 {
		t.Errorf("expected MissionDuration=0.1, got %f", meta.MissionDuration)
	}
}

func TestGetExportMetadata_VehicleEndFrame(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{
		MissionName:  "Vehicle Test",
		CaptureDelay: 1.0,
		Tag:          "PvE",
	}
	world := &core.World{WorldName: "Stratis"}

	_ = b.StartMission(mission, world)

	// Add soldier with lower frame
	s := &core.Soldier{ID: 1}
	_ = b.AddSoldier(s)
	_ = b.RecordSoldierState(&core.SoldierState{
		SoldierID:     s.ID,
				CaptureFrame:  50,
	})

	// Add vehicle with higher frame - this should determine endFrame
	v := &core.Vehicle{ID: 10}
	_ = b.AddVehicle(v)
	_ = b.RecordVehicleState(&core.VehicleState{
		VehicleID:     v.ID,
				CaptureFrame:  200,
	})

	meta := b.GetExportMetadata()

	// Duration should be based on vehicle's higher frame: 200 * 1.0 / 1000 = 0.2
	if meta.MissionDuration != 0.2 {
		t.Errorf("expected MissionDuration=0.2 (from vehicle frame 200), got %f", meta.MissionDuration)
	}
}

func TestGetExportMetadata_EmptyMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{
		MissionName:  "Empty Mission",
		CaptureDelay: 1.0,
		Tag:          "",
	}
	world := &core.World{WorldName: "VR"}

	_ = b.StartMission(mission, world)

	// No soldiers or vehicles added

	meta := b.GetExportMetadata()

	if meta.WorldName != "VR" {
		t.Errorf("expected WorldName=VR, got %s", meta.WorldName)
	}
	if meta.MissionName != "Empty Mission" {
		t.Errorf("expected MissionName=Empty Mission, got %s", meta.MissionName)
	}
	// Duration should be 0 with no frames
	if meta.MissionDuration != 0 {
		t.Errorf("expected MissionDuration=0, got %f", meta.MissionDuration)
	}
}

func TestStartMissionResetsExportPath(t *testing.T) {
	b := New(config.MemoryConfig{
		OutputDir:      t.TempDir(),
		CompressOutput: true,
	})

	mission := &core.Mission{
		MissionName: "First",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Altis"}

	_ = b.StartMission(mission, world)
	_ = b.EndMission()

	firstPath := b.GetExportedFilePath()
	if firstPath == "" {
		t.Fatal("expected non-empty path after export")
	}

	// Start new mission - should reset path
	_ = b.StartMission(&core.Mission{MissionName: "Second", StartTime: time.Now()}, world)

	if path := b.GetExportedFilePath(); path != "" {
		t.Errorf("expected empty path after StartMission, got %s", path)
	}
}

func TestEndMissionWithoutStartMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	// EndMission without StartMission should return an error, not panic
	err := b.EndMission()
	if err == nil {
		t.Error("expected error when ending mission that was never started")
	}
	if !strings.Contains(err.Error(), "no mission to end") {
		t.Errorf("expected error message to contain 'no mission to end', got: %s", err.Error())
	}
}

func TestGetExportMetadataWithoutStartMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	// GetExportMetadata without StartMission should return empty metadata, not panic
	meta := b.GetExportMetadata()

	if meta.WorldName != "" {
		t.Errorf("expected empty WorldName, got %s", meta.WorldName)
	}
	if meta.MissionName != "" {
		t.Errorf("expected empty MissionName, got %s", meta.MissionName)
	}
	if meta.Tag != "" {
		t.Errorf("expected empty Tag, got %s", meta.Tag)
	}
	if meta.MissionDuration != 0 {
		t.Errorf("expected MissionDuration=0, got %f", meta.MissionDuration)
	}
}
