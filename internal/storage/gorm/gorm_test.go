package gormstorage

import (
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/mission"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBackend creates a Backend with no DB (queue-only mode for unit testing).
func newTestBackend() *Backend {
	return New(Dependencies{
		DB:              nil,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     cache.NewMarkerCache(),
		LogManager:      logging.NewSlogManager(),
		MissionContext:  mission.NewContext(),
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	})
}

// Compile-time interface check
var _ storage.Backend = (*Backend)(nil)

func TestNew(t *testing.T) {
	b := newTestBackend()
	require.NotNil(t, b)
}

func TestInitClose(t *testing.T) {
	b := newTestBackend()

	err := b.Init()
	require.NoError(t, err)
	require.NotNil(t, b.queues)
	require.NotNil(t, b.stopChan)

	err = b.Close()
	require.NoError(t, err)
}

func TestAddSoldier_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	soldier := &core.Soldier{
		ID:       42,
		UnitName: "Test Soldier",
		Side:     "WEST",
	}

	err := b.AddSoldier(soldier)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.Soldiers.Len())
}

func TestAddVehicle_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	vehicle := &core.Vehicle{
		ID:          10,
		OcapType:    "car",
		DisplayName: "Hunter",
	}

	err := b.AddVehicle(vehicle)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.Vehicles.Len())
}

func TestAddMarker_NoDB_NoError(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	marker := &core.Marker{
		MarkerName: "TestMarker",
		MarkerType: "mil_dot",
	}

	err := b.AddMarker(marker)
	require.NoError(t, err)
	// No DB → marker not inserted, but no error
}

func TestRecordSoldierState_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	state := &core.SoldierState{
		SoldierID:    42,
		CaptureFrame: 100,
		Position:     core.Position3D{X: 100, Y: 200, Z: 10},
	}

	err := b.RecordSoldierState(state)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.SoldierStates.Len())
}

func TestRecordVehicleState_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	state := &core.VehicleState{
		VehicleID:    10,
		CaptureFrame: 50,
	}

	err := b.RecordVehicleState(state)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.VehicleStates.Len())
}

func TestRecordMarkerState_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	state := &core.MarkerState{
		MarkerID:     5,
		CaptureFrame: 100,
	}

	err := b.RecordMarkerState(state)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.MarkerStates.Len())
}

func TestRecordProjectileEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.ProjectileEvent{
		FirerObjectID: 1,
		CaptureFrame:  620,
		Trajectory: []core.TrajectoryPoint{
			{Position: core.Position3D{X: 100, Y: 200, Z: 10}, Frame: 620},
			{Position: core.Position3D{X: 110, Y: 210, Z: 11}, Frame: 621},
		},
	}

	err := b.RecordProjectileEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.ProjectileEvents.Len())
}

func TestRecordProjectileEvent_SkipsWhenSQLite(t *testing.T) {
	b := New(Dependencies{
		DB:              nil,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     cache.NewMarkerCache(),
		LogManager:      logging.NewSlogManager(),
		MissionContext:  mission.NewContext(),
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return true }, // SQLite mode
		DBInsertsPaused: func() bool { return false },
	})
	b.Init()
	defer b.Close()

	event := &core.ProjectileEvent{
		FirerObjectID: 1,
		CaptureFrame:  620,
	}

	err := b.RecordProjectileEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 0, b.queues.ProjectileEvents.Len(), "should not queue when SQLite")
}

func TestRecordGeneralEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.GeneralEvent{
		Name:    "connected",
		Message: "Player1",
	}

	err := b.RecordGeneralEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.GeneralEvents.Len())
}

func TestRecordKillEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	victimID := uint(5)
	event := &core.KillEvent{
		CaptureFrame:    100,
		VictimSoldierID: &victimID,
	}

	err := b.RecordKillEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.KillEvents.Len())
}

func TestRecordChatEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.ChatEvent{
		Channel: "Global",
		Message: "Hello",
	}

	err := b.RecordChatEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.ChatEvents.Len())
}

func TestRecordRadioEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.RadioEvent{
		Radio:     "AN/PRC-152",
		RadioType: "SW",
	}

	err := b.RecordRadioEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.RadioEvents.Len())
}

func TestRecordServerFpsEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.ServerFpsEvent{
		FpsAverage: 50.0,
		FpsMin:     30.0,
	}

	err := b.RecordServerFpsEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.FpsEvents.Len())
}

func TestRecordAce3DeathEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.Ace3DeathEvent{
		SoldierID: 5,
		Reason:    "BLOODLOSS",
	}

	err := b.RecordAce3DeathEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.Ace3DeathEvents.Len())
}

func TestRecordAce3UnconsciousEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	event := &core.Ace3UnconsciousEvent{
		SoldierID:     5,
		IsUnconscious: true,
	}

	err := b.RecordAce3UnconsciousEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.Ace3UnconsciousEvents.Len())
}

func TestRecordTimeState_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	ts := &core.TimeState{
		CaptureFrame: 100,
	}

	err := b.RecordTimeState(ts)
	require.NoError(t, err)
	// No queue for TimeState — it's a no-op
}

func TestRecordFiredEvent_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	err := b.RecordFiredEvent(&core.FiredEvent{})
	require.NoError(t, err)
}

func TestRecordHitEvent_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	err := b.RecordHitEvent(&core.HitEvent{})
	require.NoError(t, err)
}

func TestStartMission_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	err := b.StartMission(&core.Mission{}, &core.World{})
	require.NoError(t, err)
}

func TestEndMission_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	err := b.EndMission()
	require.NoError(t, err)
}

func TestDeleteMarker_PushesAlphaZeroState(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	// Pre-populate marker cache
	b.deps.MarkerCache.Set("TestMarker", 42)

	b.DeleteMarker("TestMarker", 500)

	assert.Equal(t, 1, b.queues.MarkerStates.Len())
	items := b.queues.MarkerStates.GetAndEmpty()
	require.Len(t, items, 1)
	assert.Equal(t, uint(42), items[0].MarkerID)
	assert.Equal(t, float32(0), items[0].Alpha)
	assert.Equal(t, uint(500), items[0].CaptureFrame)
}

func TestDeleteMarker_UnknownMarker_NoOp(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	b.DeleteMarker("NonExistent", 500)

	assert.Equal(t, 0, b.queues.MarkerStates.Len())
}

func TestGetSoldierByObjectID(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	_, found := b.GetSoldierByObjectID(42)
	assert.False(t, found, "should not find soldier not in cache")

	// Add to entity cache (cache stores core types)
	b.deps.EntityCache.AddSoldier(core.Soldier{ID: 42, UnitName: "Test"})
	soldier, found := b.GetSoldierByObjectID(42)
	assert.True(t, found)
	assert.Equal(t, uint16(42), soldier.ID)
	assert.Equal(t, "Test", soldier.UnitName)
}

func TestGetVehicleByObjectID(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	_, found := b.GetVehicleByObjectID(10)
	assert.False(t, found, "should not find vehicle not in cache")

	b.deps.EntityCache.AddVehicle(core.Vehicle{ID: 10, OcapType: "car"})
	vehicle, found := b.GetVehicleByObjectID(10)
	assert.True(t, found)
	assert.Equal(t, uint16(10), vehicle.ID)
	assert.Equal(t, "car", vehicle.OcapType)
}

func TestGetMarkerByName(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	_, found := b.GetMarkerByName("TestMarker")
	assert.False(t, found, "should not find marker not in cache")

	b.deps.MarkerCache.Set("TestMarker", 42)
	marker, found := b.GetMarkerByName("TestMarker")
	assert.True(t, found)
	assert.Equal(t, "TestMarker", marker.MarkerName)
}

func TestGetLastDBWriteDuration(t *testing.T) {
	b := newTestBackend()
	b.Init()
	defer b.Close()

	assert.Equal(t, time.Duration(0), b.GetLastDBWriteDuration())

	b.lastDBWriteDuration = 100 * time.Millisecond
	assert.Equal(t, 100*time.Millisecond, b.GetLastDBWriteDuration())
}
