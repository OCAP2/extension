// internal/storage/memory/memory_test.go
package memory

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	require.NotNil(t, b)
	assert.Equal(t, "/tmp/test", b.cfg.OutputDir)
	assert.True(t, b.cfg.CompressOutput, "expected CompressOutput=true")
	assert.NotNil(t, b.soldiers)
	assert.NotNil(t, b.vehicles)
	assert.NotNil(t, b.markers)
}

func TestInitAndClose(t *testing.T) {
	b := New(config.MemoryConfig{})

	assert.NoError(t, b.Init())
	assert.NoError(t, b.Close())
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
	require.NoError(t, b.StartMission(mission, world))

	assert.Equal(t, mission, b.mission)
	assert.Equal(t, world, b.world)
	assert.Len(t, b.soldiers, 0)
}

func TestAddSoldier(t *testing.T) {
	b := New(config.MemoryConfig{})

	s1 := &core.Soldier{
		ID:       1,
		UnitName: "Soldier One",
		Side:     "WEST",
		IsPlayer: true,
	}
	s2 := &core.Soldier{
		ID:       2,
		UnitName: "Soldier Two",
		Side:     "EAST",
		IsPlayer: false,
	}

	require.NoError(t, b.AddSoldier(s1))
	require.NoError(t, b.AddSoldier(s2))

	// IDs are ObjectIDs set by caller, not auto-assigned
	assert.Equal(t, uint16(1), s1.ID)
	assert.Equal(t, uint16(2), s2.ID)

	// Check storage
	assert.Len(t, b.soldiers, 2)
	assert.Equal(t, "Soldier One", b.soldiers[1].Soldier.UnitName)
	assert.Equal(t, "Soldier Two", b.soldiers[2].Soldier.UnitName)
}

func TestAddVehicle(t *testing.T) {
	b := New(config.MemoryConfig{})

	v1 := &core.Vehicle{
		ID:          10,
		ClassName:   "B_MRAP_01_F",
		DisplayName: "Hunter",
	}
	v2 := &core.Vehicle{
		ID:          20,
		ClassName:   "B_Heli_Light_01_F",
		DisplayName: "MH-9 Hummingbird",
	}

	require.NoError(t, b.AddVehicle(v1))
	require.NoError(t, b.AddVehicle(v2))

	// IDs are ObjectIDs set by caller, not auto-assigned
	assert.Equal(t, uint16(10), v1.ID)
	assert.Equal(t, uint16(20), v2.ID)

	assert.Len(t, b.vehicles, 2)
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

	require.NoError(t, b.AddMarker(m1))
	require.NoError(t, b.AddMarker(m2))

	// Markers don't have ObjectIDs; ID is not auto-assigned
	// Just verify storage works

	assert.Len(t, b.markers, 2)
	assert.Equal(t, "Base", b.markers["marker_1"].Marker.Text)
}

func TestGetSoldierByObjectID(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{
		ID:       42,
		UnitName: "Test Soldier",
	}
	_ = b.AddSoldier(s)

	// Found case
	found, ok := b.GetSoldierByObjectID(42)
	require.True(t, ok, "soldier not found")
	assert.Equal(t, "Test Soldier", found.UnitName)

	// Not found case
	_, ok = b.GetSoldierByObjectID(999)
	assert.False(t, ok, "expected not found for non-existent ObjectID")
}

func TestGetVehicleByObjectID(t *testing.T) {
	b := New(config.MemoryConfig{})

	v := &core.Vehicle{
		ID:        100,
		ClassName: "B_Truck_01_transport_F",
	}
	_ = b.AddVehicle(v)

	// Found case
	found, ok := b.GetVehicleByObjectID(100)
	require.True(t, ok, "vehicle not found")
	assert.Equal(t, "B_Truck_01_transport_F", found.ClassName)

	// Not found case
	_, ok = b.GetVehicleByObjectID(999)
	assert.False(t, ok, "expected not found for non-existent ObjectID")
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
	require.True(t, ok, "marker not found")
	assert.Equal(t, "Respawn West", found.Text)

	// Not found case
	_, ok = b.GetMarkerByName("nonexistent")
	assert.False(t, ok, "expected not found for non-existent marker name")
}

func TestRecordSoldierState(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{ID: 1, UnitName: "Test"}
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

	require.NoError(t, b.RecordSoldierState(state1))
	require.NoError(t, b.RecordSoldierState(state2))

	record := b.soldiers[s.ID]
	assert.Len(t, record.States, 2)
	assert.Equal(t, uint(0), record.States[0].CaptureFrame)
	assert.Equal(t, uint(1), record.States[1].CaptureFrame)

	// Recording state for non-existent soldier should not error
	orphanState := &core.SoldierState{SoldierID: 999, CaptureFrame: 0}
	assert.NoError(t, b.RecordSoldierState(orphanState))
}

func TestRecordVehicleState(t *testing.T) {
	b := New(config.MemoryConfig{})

	v := &core.Vehicle{ID: 10, ClassName: "Car"}
	_ = b.AddVehicle(v)

	state := &core.VehicleState{
		VehicleID:    v.ID,
		CaptureFrame: 5,
		Position:     core.Position3D{X: 500, Y: 600, Z: 10},
		IsAlive:      true,
	}

	require.NoError(t, b.RecordVehicleState(state))

	record := b.vehicles[v.ID]
	assert.Len(t, record.States, 1)

	// Non-existent vehicle
	orphan := &core.VehicleState{VehicleID: 999}
	assert.NoError(t, b.RecordVehicleState(orphan))
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

	require.NoError(t, b.RecordMarkerState(state))

	record := b.markers["test_marker"]
	assert.Len(t, record.States, 1)

	// Non-existent marker
	orphan := &core.MarkerState{MarkerID: 999}
	assert.NoError(t, b.RecordMarkerState(orphan))
}

func TestDeleteMarker(t *testing.T) {
	b := New(config.MemoryConfig{})

	m := &core.Marker{MarkerName: "grenade_1", EndFrame: -1}
	_ = b.AddMarker(m)

	// Delete marker at frame 100
	b.DeleteMarker("grenade_1", 100)

	record := b.markers["grenade_1"]
	assert.Equal(t, 100, record.Marker.EndFrame)

	// Deleting non-existent marker should not panic
	b.DeleteMarker("nonexistent", 50)
}

func TestRecordFiredEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	s := &core.Soldier{ID: 1}
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

	require.NoError(t, b.RecordFiredEvent(fired))

	record := b.soldiers[s.ID]
	assert.Len(t, record.FiredEvents, 1)
	assert.Equal(t, "arifle_MX_F", record.FiredEvents[0].Weapon)

	// Non-existent soldier
	orphan := &core.FiredEvent{SoldierID: 999}
	assert.NoError(t, b.RecordFiredEvent(orphan))
}

func TestRecordGeneralEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.GeneralEvent{
		CaptureFrame: 50,
		Name:         "connected",
		Message:      "Player connected",
		ExtraData:    map[string]any{"playerName": "TestPlayer"},
	}

	require.NoError(t, b.RecordGeneralEvent(evt))

	assert.Len(t, b.generalEvents, 1)
	assert.Equal(t, "connected", b.generalEvents[0].Name)
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

	require.NoError(t, b.RecordHitEvent(evt))

	assert.Len(t, b.hitEvents, 1)
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

	require.NoError(t, b.RecordKillEvent(evt))

	assert.Len(t, b.killEvents, 1)
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

	require.NoError(t, b.RecordChatEvent(evt))

	assert.Len(t, b.chatEvents, 1)
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

	require.NoError(t, b.RecordRadioEvent(evt))

	assert.Len(t, b.radioEvents, 1)
}

func TestRecordServerFpsEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.ServerFpsEvent{
		CaptureFrame: 400,
		FpsAverage:   45.5,
		FpsMin:       30.0,
	}

	require.NoError(t, b.RecordServerFpsEvent(evt))

	assert.Len(t, b.serverFpsEvents, 1)
}

func TestRecordTimeState(t *testing.T) {
	b := New(config.MemoryConfig{})

	now := time.Now()
	state := &core.TimeState{
		Time:           now,
		CaptureFrame:   100,
		SystemTimeUTC:  "2024-01-15T14:30:45.123",
		MissionDate:    "2035-06-15T06:00:00",
		TimeMultiplier: 2.0,
		MissionTime:    3600.5,
	}

	require.NoError(t, b.RecordTimeState(state))

	assert.Len(t, b.timeStates, 1)
	assert.Equal(t, uint(100), b.timeStates[0].CaptureFrame)
	assert.Equal(t, float32(2.0), b.timeStates[0].TimeMultiplier)
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

	require.NoError(t, b.RecordAce3DeathEvent(evt))

	assert.Len(t, b.ace3DeathEvents, 1)
}

func TestRecordAce3UnconsciousEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.Ace3UnconsciousEvent{
		SoldierID:     1,
		CaptureFrame:  600,
		IsUnconscious: true,
	}

	require.NoError(t, b.RecordAce3UnconsciousEvent(evt))

	assert.Len(t, b.ace3UnconsciousEvents, 1)
}

func TestRecordProjectileEvent(t *testing.T) {
	b := New(config.MemoryConfig{})

	evt := &core.ProjectileEvent{
		CaptureFrame:   100,
		FirerObjectID:  42,
		SimulationType: "",
		MagazineDisplay: "RGO Grenade",
		Trajectory: []core.TrajectoryPoint{
			{Position: core.Position3D{X: 100, Y: 200, Z: 10}, Frame: 100},
			{Position: core.Position3D{X: 150, Y: 250, Z: 5}, Frame: 105},
		},
	}

	require.NoError(t, b.RecordProjectileEvent(evt))

	assert.Len(t, b.projectileEvents, 1)
	assert.Equal(t, uint16(42), b.projectileEvents[0].FirerObjectID)
	assert.Len(t, b.projectileEvents[0].Trajectory, 2)
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
	assert.Equal(t, expectedCount, len(b.soldiers))
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
	assert.Equal(t, uint16(1), s.ID)
	assert.Equal(t, uint16(10), v.ID)
	// Markers are keyed by name, not ID
	assert.NotNil(t, b.markers["test"])
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
	_ = b.RecordProjectileEvent(&core.ProjectileEvent{})

	// Start new mission
	mission := &core.Mission{MissionName: "New"}
	world := &core.World{WorldName: "Stratis"}
	_ = b.StartMission(mission, world)

	assert.Len(t, b.soldiers, 0)
	assert.Len(t, b.vehicles, 0)
	assert.Len(t, b.markers, 0)
	assert.Len(t, b.generalEvents, 0)
	assert.Len(t, b.hitEvents, 0)
	assert.Len(t, b.killEvents, 0)
	assert.Len(t, b.chatEvents, 0)
	assert.Len(t, b.radioEvents, 0)
	assert.Len(t, b.serverFpsEvents, 0)
	assert.Len(t, b.timeStates, 0)
	assert.Len(t, b.ace3DeathEvents, 0)
	assert.Len(t, b.ace3UnconsciousEvents, 0)
	assert.Len(t, b.projectileEvents, 0)
}

func TestGetExportedFilePath(t *testing.T) {
	b := New(config.MemoryConfig{
		OutputDir:      t.TempDir(),
		CompressOutput: true,
	})

	// Before export, should return empty
	assert.Empty(t, b.GetExportedFilePath())
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
	require.NotEmpty(t, path)
	assert.True(t, strings.HasPrefix(path, tmpDir), "expected path to start with tmpDir")
	assert.True(t, strings.HasSuffix(path, ".json.gz"), "expected path to end with .json.gz")
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
	require.NotEmpty(t, path)
	assert.True(t, strings.HasSuffix(path, ".json"), "expected path to end with .json")
	assert.False(t, strings.HasSuffix(path, ".json.gz"), "expected path to NOT end with .json.gz for uncompressed")
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
		SoldierID:    s.ID,
		CaptureFrame: 100,
	})

	meta := b.GetExportMetadata()

	assert.Equal(t, "Altis", meta.WorldName)
	assert.Equal(t, "Test Mission", meta.MissionName)
	assert.Equal(t, "TvT", meta.Tag)
	// Duration = endFrame * captureDelay / 1000 = 100 * 1.0 / 1000 = 0.1
	assert.Equal(t, 0.1, meta.MissionDuration)
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
		SoldierID:    s.ID,
		CaptureFrame: 50,
	})

	// Add vehicle with higher frame - this should determine endFrame
	v := &core.Vehicle{ID: 10}
	_ = b.AddVehicle(v)
	_ = b.RecordVehicleState(&core.VehicleState{
		VehicleID:    v.ID,
		CaptureFrame: 200,
	})

	meta := b.GetExportMetadata()

	// Duration should be based on vehicle's higher frame: 200 * 1.0 / 1000 = 0.2
	assert.Equal(t, 0.2, meta.MissionDuration)
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

	assert.Equal(t, "VR", meta.WorldName)
	assert.Equal(t, "Empty Mission", meta.MissionName)
	// Duration should be 0 with no frames
	assert.Equal(t, 0.0, meta.MissionDuration)
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
	require.NotEmpty(t, firstPath)

	// Start new mission - should reset path
	_ = b.StartMission(&core.Mission{MissionName: "Second", StartTime: time.Now()}, world)

	assert.Empty(t, b.GetExportedFilePath())
}

func TestEndMissionWithoutStartMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	// EndMission without StartMission should return an error, not panic
	err := b.EndMission()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no mission to end")
}

func TestGetExportMetadataWithoutStartMission(t *testing.T) {
	b := New(config.MemoryConfig{})

	// GetExportMetadata without StartMission should return empty metadata, not panic
	meta := b.GetExportMetadata()

	assert.Empty(t, meta.WorldName)
	assert.Empty(t, meta.MissionName)
	assert.Empty(t, meta.Tag)
	assert.Equal(t, 0.0, meta.MissionDuration)
}
