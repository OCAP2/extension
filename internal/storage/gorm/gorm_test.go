package gormstorage

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/mission"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/queue"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

// newTestDB creates an in-memory SQLite DB with auto-migrated tables.
// MaxOpenConns=1 ensures all operations use the same connection (in-memory
// SQLite databases are per-connection, so multiple connections would each
// see an empty database).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(model.DatabaseModelsSQLite...))
	return db
}

func noopLog(_, _, _ string) {}

func TestWriteQueue_Success(t *testing.T) {
	db := newTestDB(t)
	q := queue.New[model.Soldier]()

	now := time.Now()
	q.Push(model.Soldier{ObjectID: 1, MissionID: 1, UnitName: "Alpha", JoinTime: now})
	q.Push(model.Soldier{ObjectID: 2, MissionID: 1, UnitName: "Bravo", JoinTime: now})

	writeQueue(db, q, "soldiers", noopLog, nil, nil)

	assert.True(t, q.Empty(), "queue should be drained after successful write")

	var count int64
	db.Model(&model.Soldier{}).Count(&count)
	assert.Equal(t, int64(2), count)
}

func TestWriteQueue_EmptyQueue(t *testing.T) {
	db := newTestDB(t)
	q := queue.New[model.Soldier]()

	// Should be a no-op
	writeQueue(db, q, "soldiers", noopLog, nil, nil)

	var count int64
	db.Model(&model.Soldier{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestWriteQueue_PrepareCallback(t *testing.T) {
	db := newTestDB(t)
	q := queue.New[model.Soldier]()

	q.Push(model.Soldier{ObjectID: 1, UnitName: "Alpha", JoinTime: time.Now()})

	prepareCalled := false
	writeQueue(db, q, "soldiers", noopLog, func(items []model.Soldier) {
		prepareCalled = true
		for i := range items {
			items[i].MissionID = 99
		}
	}, nil)

	assert.True(t, prepareCalled)

	var soldier model.Soldier
	db.First(&soldier)
	assert.Equal(t, uint(99), soldier.MissionID)
}

func TestWriteQueue_OnSuccessCallback(t *testing.T) {
	db := newTestDB(t)
	q := queue.New[model.Soldier]()

	q.Push(model.Soldier{ObjectID: 1, MissionID: 1, UnitName: "Alpha", JoinTime: time.Now()})

	successCalled := false
	writeQueue(db, q, "soldiers", noopLog, nil, func(items []model.Soldier) {
		successCalled = true
		assert.Len(t, items, 1)
	})

	assert.True(t, successCalled)
}

func TestWriteQueue_FailureRequeues(t *testing.T) {
	db := newTestDB(t)
	// Drop the table so the insert fails
	db.Migrator().DropTable(&model.Soldier{})

	q := queue.New[model.Soldier]()
	q.Push(model.Soldier{ObjectID: 1, MissionID: 1, UnitName: "Alpha", JoinTime: time.Now()})

	var logged atomic.Bool
	logFn := func(_, _, _ string) { logged.Store(true) }

	writeQueue(db, q, "soldiers", logFn, nil, nil)

	assert.True(t, logged.Load(), "error should be logged")
	assert.Equal(t, 1, q.Len(), "failed items should be re-queued")
}

func TestAddMarker_WithDB(t *testing.T) {
	db := newTestDB(t)

	// Create a mission first (foreign key)
	db.Create(&model.Mission{MissionName: "test"})

	mCtx := mission.NewContext()
	mCtx.SetMission(&model.Mission{Model: gorm.Model{ID: 1}}, &model.World{})

	b := New(Dependencies{
		DB:              db,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     cache.NewMarkerCache(),
		LogManager:      logging.NewSlogManager(),
		MissionContext:  mCtx,
		IsDatabaseValid: func() bool { return true },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	})
	b.Init()
	defer b.Close()

	marker := &core.Marker{
		MarkerName: "TestMarker",
		MarkerType: "mil_dot",
		Text:       "HQ",
	}

	err := b.AddMarker(marker)
	require.NoError(t, err)
	assert.NotZero(t, marker.ID, "marker ID should be assigned by DB")

	var count int64
	db.Model(&model.Marker{}).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestStartDBWriters_DrainsQueues(t *testing.T) {
	db := newTestDB(t)

	// Create a mission first (foreign key)
	db.Create(&model.Mission{MissionName: "test"})

	mCtx := mission.NewContext()
	mCtx.SetMission(&model.Mission{Model: gorm.Model{ID: 1}}, &model.World{})

	b := New(Dependencies{
		DB:              db,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     cache.NewMarkerCache(),
		LogManager:      logging.NewSlogManager(),
		MissionContext:  mCtx,
		IsDatabaseValid: func() bool { return true },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	})
	b.Init()
	defer b.Close()

	// Push items via the public API (which queues GORM models internally)
	b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Alpha", Side: "WEST"})
	b.RecordGeneralEvent(&core.GeneralEvent{Name: "connected", Message: "Player1"})
	b.RecordServerFpsEvent(&core.ServerFpsEvent{FpsAverage: 50, FpsMin: 30, CaptureFrame: 1})

	// Wait for the background writer to drain (it runs on a 2s loop, so wait up to 5s)
	require.Eventually(t, func() bool {
		var count int64
		db.Model(&model.Soldier{}).Count(&count)
		return count > 0
	}, 5*time.Second, 100*time.Millisecond, "soldiers should be written to DB")

	var soldierCount, eventCount, fpsCount int64
	db.Model(&model.Soldier{}).Count(&soldierCount)
	db.Model(&model.GeneralEvent{}).Count(&eventCount)
	db.Model(&model.ServerFpsEvent{}).Count(&fpsCount)

	assert.Equal(t, int64(1), soldierCount)
	assert.Equal(t, int64(1), eventCount)
	assert.Equal(t, int64(1), fpsCount)
	assert.NotZero(t, b.GetLastDBWriteDuration())
}
