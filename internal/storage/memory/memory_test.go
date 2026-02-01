// internal/storage/memory/memory_test.go
package memory

import (
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/storage"
)

// Verify Backend implements storage.Backend interface
var _ storage.Backend = (*Backend)(nil)

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
	soldier := &core.Soldier{OcapID: 1, UnitName: "Old Soldier"}
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
	if b.idCounter != 0 {
		t.Error("idCounter not reset")
	}
}

func TestAddSoldier(t *testing.T) {
	b := New(config.MemoryConfig{})

	s1 := &core.Soldier{
		OcapID:   1,
		UnitName: "Soldier One",
		Side:     "WEST",
		IsPlayer: true,
	}
	s2 := &core.Soldier{
		OcapID:   2,
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

	// Check IDs assigned
	if s1.ID != 1 {
		t.Errorf("expected s1.ID=1, got %d", s1.ID)
	}
	if s2.ID != 2 {
		t.Errorf("expected s2.ID=2, got %d", s2.ID)
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
		OcapID:      10,
		ClassName:   "B_MRAP_01_F",
		DisplayName: "Hunter",
	}
	v2 := &core.Vehicle{
		OcapID:      20,
		ClassName:   "B_Heli_Light_01_F",
		DisplayName: "MH-9 Hummingbird",
	}

	if err := b.AddVehicle(v1); err != nil {
		t.Fatalf("AddVehicle failed: %v", err)
	}
	if err := b.AddVehicle(v2); err != nil {
		t.Fatalf("AddVehicle failed: %v", err)
	}

	if v1.ID != 1 {
		t.Errorf("expected v1.ID=1, got %d", v1.ID)
	}
	if v2.ID != 2 {
		t.Errorf("expected v2.ID=2, got %d", v2.ID)
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

	if m1.ID != 1 {
		t.Errorf("expected m1.ID=1, got %d", m1.ID)
	}
	if m2.ID != 2 {
		t.Errorf("expected m2.ID=2, got %d", m2.ID)
	}

	if len(b.markers) != 2 {
		t.Errorf("expected 2 markers, got %d", len(b.markers))
	}
	if b.markers["marker_1"].Marker.Text != "Base" {
		t.Error("marker_1 not stored correctly")
	}
}

func TestGetSoldierByOcapID(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{
		OcapID:   42,
		UnitName: "Test Soldier",
	}
	_ = b.AddSoldier(s)

	// Found case
	found, ok := b.GetSoldierByOcapID(42)
	if !ok {
		t.Fatal("soldier not found")
	}
	if found.UnitName != "Test Soldier" {
		t.Errorf("expected UnitName=Test Soldier, got %s", found.UnitName)
	}

	// Not found case
	_, ok = b.GetSoldierByOcapID(999)
	if ok {
		t.Error("expected not found for non-existent OcapID")
	}
}

func TestGetVehicleByOcapID(t *testing.T) {
	b := New(config.MemoryConfig{})

	v := &core.Vehicle{
		OcapID:    100,
		ClassName: "B_Truck_01_transport_F",
	}
	_ = b.AddVehicle(v)

	// Found case
	found, ok := b.GetVehicleByOcapID(100)
	if !ok {
		t.Fatal("vehicle not found")
	}
	if found.ClassName != "B_Truck_01_transport_F" {
		t.Errorf("expected ClassName=B_Truck_01_transport_F, got %s", found.ClassName)
	}

	// Not found case
	_, ok = b.GetVehicleByOcapID(999)
	if ok {
		t.Error("expected not found for non-existent OcapID")
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

	s := &core.Soldier{OcapID: 1, UnitName: "Test"}
	_ = b.AddSoldier(s)

	state1 := &core.SoldierState{
		SoldierID:    s.ID,
		CaptureFrame: 0,
		Position:     core.Position3D{X: 100, Y: 200, Z: 0},
		Bearing:      90,
		Lifestate:    1,
	}
	state2 := &core.SoldierState{
		SoldierID:    s.ID,
		CaptureFrame: 1,
		Position:     core.Position3D{X: 105, Y: 205, Z: 0},
		Bearing:      95,
		Lifestate:    1,
	}

	if err := b.RecordSoldierState(state1); err != nil {
		t.Fatalf("RecordSoldierState failed: %v", err)
	}
	if err := b.RecordSoldierState(state2); err != nil {
		t.Fatalf("RecordSoldierState failed: %v", err)
	}

	record := b.soldiers[1]
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

	v := &core.Vehicle{OcapID: 10, ClassName: "Car"}
	_ = b.AddVehicle(v)

	state := &core.VehicleState{
		VehicleID:    v.ID,
		CaptureFrame: 5,
		Position:     core.Position3D{X: 500, Y: 600, Z: 10},
		IsAlive:      true,
	}

	if err := b.RecordVehicleState(state); err != nil {
		t.Fatalf("RecordVehicleState failed: %v", err)
	}

	record := b.vehicles[10]
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

func TestRecordFiredEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{OcapID: 1}
	_ = b.AddSoldier(s)

	fired := &core.FiredEvent{
		SoldierID:    s.ID,
		CaptureFrame: 100,
		Weapon:       "arifle_MX_F",
		Magazine:     "30Rnd_65x39_caseless_mag",
		FiringMode:   "Single",
		StartPos:     core.Position3D{X: 100, Y: 100, Z: 1},
		EndPos:       core.Position3D{X: 200, Y: 200, Z: 1},
	}

	if err := b.RecordFiredEvent(fired); err != nil {
		t.Fatalf("RecordFiredEvent failed: %v", err)
	}

	record := b.soldiers[1]
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
		SoldierID:    1,
		CaptureFrame: 600,
		IsAwake:      false,
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
				s := &core.Soldier{OcapID: ocapID, UnitName: "Concurrent"}
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
				_, _ = b.GetSoldierByOcapID(ocapID)
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

func TestIDCounter(t *testing.T) {
	b := New(config.MemoryConfig{})

	// Add soldier, vehicle, marker - all should increment same counter
	s := &core.Soldier{OcapID: 1}
	v := &core.Vehicle{OcapID: 10}
	m := &core.Marker{MarkerName: "test"}

	_ = b.AddSoldier(s)
	_ = b.AddVehicle(v)
	_ = b.AddMarker(m)

	if s.ID != 1 {
		t.Errorf("expected soldier ID=1, got %d", s.ID)
	}
	if v.ID != 2 {
		t.Errorf("expected vehicle ID=2, got %d", v.ID)
	}
	if m.ID != 3 {
		t.Errorf("expected marker ID=3, got %d", m.ID)
	}
}

func TestStartMissionResetsEverything(t *testing.T) {
	b := New(config.MemoryConfig{})

	// Populate with data
	_ = b.AddSoldier(&core.Soldier{OcapID: 1})
	_ = b.AddVehicle(&core.Vehicle{OcapID: 10})
	_ = b.AddMarker(&core.Marker{MarkerName: "m1"})
	_ = b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"})
	_ = b.RecordHitEvent(&core.HitEvent{})
	_ = b.RecordKillEvent(&core.KillEvent{})
	_ = b.RecordChatEvent(&core.ChatEvent{})
	_ = b.RecordRadioEvent(&core.RadioEvent{})
	_ = b.RecordServerFpsEvent(&core.ServerFpsEvent{})
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
	if len(b.ace3DeathEvents) != 0 {
		t.Error("ace3DeathEvents not reset")
	}
	if len(b.ace3UnconsciousEvents) != 0 {
		t.Error("ace3UnconsciousEvents not reset")
	}
	if b.idCounter != 0 {
		t.Error("idCounter not reset")
	}
}
