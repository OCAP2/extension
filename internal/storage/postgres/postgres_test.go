package postgres

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
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
		DB:          nil,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})
}

// Compile-time interface check
var _ storage.Backend = (*Backend)(nil)

func TestNew(t *testing.T) {
	b := newTestBackend()
	require.NotNil(t, b)
}

func TestInitClose(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	b := New(Dependencies{
		DB:          db,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})

	err = b.Init()
	require.NoError(t, err)
	require.NotNil(t, b.queues)
	require.NotNil(t, b.stopChan)

	err = b.Close()
	require.NoError(t, err)
}

func TestAddSoldier_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	marker := &core.Marker{
		MarkerName: "TestMarker",
		MarkerType: "mil_dot",
	}

	id, err := b.AddMarker(marker)
	require.NoError(t, err)
	assert.Equal(t, uint(0), id, "no DB → ID should be 0")
}

func TestRecordSoldierState_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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

func TestRecordGeneralEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	event := &core.RadioEvent{
		Radio:     "AN/PRC-152",
		RadioType: "SW",
	}

	err := b.RecordRadioEvent(event)
	require.NoError(t, err)
	assert.Equal(t, 1, b.queues.RadioEvents.Len())
}

func TestRecordAce3DeathEvent_QueuesToInternalQueue(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

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
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	ts := &core.TimeState{
		CaptureFrame: 100,
	}

	err := b.RecordTimeState(ts)
	require.NoError(t, err)
	// No queue for TimeState — it's a no-op
}

func TestRecordFiredEvent_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	err := b.RecordFiredEvent(&core.FiredEvent{})
	require.NoError(t, err)
}

func TestRecordHitEvent_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	err := b.RecordHitEvent(&core.HitEvent{})
	require.NoError(t, err)
}

func TestStartMission_NoDB_NoOp(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	err := b.StartMission(&core.Mission{}, &core.World{})
	require.NoError(t, err)
}

func TestStartMission_WithDB(t *testing.T) {
	db := newTestDB(t)

	b := New(Dependencies{
		DB:          db,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})
	require.NoError(t, b.Init())
	defer func() { require.NoError(t, b.Close()) }()

	mission := &core.Mission{
		MissionName: "Test Mission",
		Author:      "Test Author",
		StartTime:   time.Now(),
		Addons: []core.Addon{
			{Name: "CBA_A3", WorkshopID: "450814997"},
			{Name: "ACE3", WorkshopID: "463939057"},
		},
	}
	world := &core.World{
		WorldName:   "altis",
		DisplayName: "Altis",
		WorldSize:   30720,
	}

	err := b.StartMission(mission, world)
	require.NoError(t, err)

	assert.NotZero(t, mission.ID, "mission should get DB-assigned ID")
	assert.NotZero(t, world.ID, "world should get DB-assigned ID")
	assert.Equal(t, uint64(mission.ID), b.missionID.Load(), "backend missionID should be set")

	// Verify DB state
	var missionCount, worldCount, addonCount int64
	db.Model(&model.Mission{}).Count(&missionCount)
	db.Model(&model.World{}).Count(&worldCount)
	db.Model(&model.Addon{}).Count(&addonCount)

	assert.Equal(t, int64(1), missionCount)
	assert.Equal(t, int64(1), worldCount)
	assert.Equal(t, int64(2), addonCount)

	// Second call with same addons should reuse them (get-or-create)
	mission2 := &core.Mission{
		MissionName: "Test Mission 2",
		StartTime:   time.Now(),
		Addons: []core.Addon{
			{Name: "CBA_A3", WorkshopID: "450814997"},
		},
	}
	err = b.StartMission(mission2, world)
	require.NoError(t, err)

	db.Model(&model.Addon{}).Count(&addonCount)
	assert.Equal(t, int64(2), addonCount, "addons should be reused, not duplicated")
	assert.Equal(t, uint64(mission2.ID), b.missionID.Load(), "missionID should update to latest")
}

func TestSetMissionID(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	assert.Equal(t, uint64(0), b.missionID.Load())
	b.SetMissionID(42)
	assert.Equal(t, uint64(42), b.missionID.Load())
}

func TestSetupDB_CreatesOcapInfo(t *testing.T) {
	// Use a raw DB without prior AutoMigrate so setupDB creates the OcapInfo table and seed row
	rawDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	sqlDB, err := rawDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	b := New(Dependencies{
		DB:          rawDB,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})

	// Init calls setupDB
	err = b.Init()
	require.NoError(t, err)
	defer func() { require.NoError(t, b.Close()) }()

	var info model.OcapInfo
	require.NoError(t, rawDB.First(&info).Error)
	assert.Equal(t, "OCAP", info.GroupName)

	// Verify full schema was migrated
	assert.True(t, rawDB.Migrator().HasTable(&model.Mission{}))
}

func TestEndMission_IsNoOp(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	err := b.EndMission()
	require.NoError(t, err)
}

func TestDeleteMarker_PushesAlphaZeroState(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	// Pre-populate marker cache
	b.deps.MarkerCache.Set("TestMarker", 42)

	require.NoError(t, b.DeleteMarker(&core.DeleteMarker{Name: "TestMarker", EndFrame: 500}))

	assert.Equal(t, 1, b.queues.MarkerStates.Len())
	items := b.queues.MarkerStates.GetAndEmpty()
	require.Len(t, items, 1)
	assert.Equal(t, uint(42), items[0].MarkerID)
	assert.Equal(t, float32(0), items[0].Alpha)
	assert.Equal(t, uint(500), items[0].CaptureFrame)
}

func TestDeleteMarker_UnknownMarker_NoOp(t *testing.T) {
	b := newTestBackend()
	b.Init() //nolint:errcheck // Init fails (no postgres) but queues are created for testing
	defer func() { require.NoError(t, b.Close()) }()

	require.NoError(t, b.DeleteMarker(&core.DeleteMarker{Name: "NonExistent", EndFrame: 500}))

	assert.Equal(t, 0, b.queues.MarkerStates.Len())
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
	require.NoError(t, db.AutoMigrate(model.DatabaseModels...))
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
	require.NoError(t, db.Migrator().DropTable(&model.Soldier{}))

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

	b := New(Dependencies{
		DB:          db,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})
	b.missionID.Store(1)
	require.NoError(t, b.Init())
	defer func() { require.NoError(t, b.Close()) }()

	marker := &core.Marker{
		MarkerName: "TestMarker",
		MarkerType: "mil_dot",
		Text:       "HQ",
	}

	id, err := b.AddMarker(marker)
	require.NoError(t, err)
	assert.NotZero(t, id, "returned ID should be assigned by DB")

	var count int64
	db.Model(&model.Marker{}).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestStartDBWriters_DrainsQueues(t *testing.T) {
	db := newTestDB(t)

	// Create a mission first (foreign key)
	db.Create(&model.Mission{MissionName: "test"})

	b := New(Dependencies{
		DB:          db,
		EntityCache: cache.NewEntityCache(),
		MarkerCache: cache.NewMarkerCache(),
		LogManager:  logging.NewSlogManager(),
	})
	b.missionID.Store(1)
	require.NoError(t, b.Init())
	defer func() { require.NoError(t, b.Close()) }()

	// Push items via the public API (which queues GORM models internally)
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Alpha", Side: "WEST"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 2, ClassName: "Humvee"}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 2}))
	require.NoError(t, b.RecordProjectileEvent(&core.ProjectileEvent{FirerObjectID: 1, CaptureFrame: 1}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{Name: "connected", Message: "Player1"}))
	require.NoError(t, b.RecordKillEvent(&core.KillEvent{CaptureFrame: 1}))
	require.NoError(t, b.RecordChatEvent(&core.ChatEvent{Message: "hello", CaptureFrame: 1}))
	require.NoError(t, b.RecordRadioEvent(&core.RadioEvent{CaptureFrame: 1}))
	require.NoError(t, b.RecordTelemetryEvent(&core.TelemetryEvent{FpsAverage: 50, FpsMin: 30, CaptureFrame: 1}))
	require.NoError(t, b.RecordAce3DeathEvent(&core.Ace3DeathEvent{SoldierID: 1, CaptureFrame: 1}))
	require.NoError(t, b.RecordAce3UnconsciousEvent(&core.Ace3UnconsciousEvent{SoldierID: 1, CaptureFrame: 1}))

	// Wait for the background writer to drain (it runs on a 2s loop, so wait up to 5s)
	require.Eventually(t, func() bool {
		var count int64
		db.Model(&model.Soldier{}).Count(&count)
		return count > 0
	}, 5*time.Second, 100*time.Millisecond, "soldiers should be written to DB")

	var soldierCount, vehicleCount, generalCount, fpsCount int64
	db.Model(&model.Soldier{}).Count(&soldierCount)
	db.Model(&model.Vehicle{}).Count(&vehicleCount)
	db.Model(&model.GeneralEvent{}).Count(&generalCount)
	db.Model(&model.ServerFpsEvent{}).Count(&fpsCount)

	assert.Equal(t, int64(1), soldierCount)
	assert.Equal(t, int64(1), vehicleCount)
	assert.Equal(t, int64(1), generalCount)
	assert.Equal(t, int64(1), fpsCount)
}
