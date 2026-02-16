// Package postgres implements the storage.Backend interface using GORM/PostgreSQL
// with internal queues and a background DB writer goroutine.
package postgres

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/database"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/convert"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/queue"

	"gorm.io/gorm"
)

// Dependencies holds all dependencies for the GORM storage backend.
type Dependencies struct {
	DB          *gorm.DB
	EntityCache *cache.EntityCache
	MarkerCache *cache.MarkerCache
	LogManager  *logging.SlogManager
}

// queues holds all the write queues for batch DB insertion.
type queues struct {
	Soldiers              *queue.Queue[model.Soldier]
	SoldierStates         *queue.Queue[model.SoldierState]
	Vehicles              *queue.Queue[model.Vehicle]
	VehicleStates         *queue.Queue[model.VehicleState]
	ProjectileEvents      *queue.Queue[model.ProjectileEvent]
	GeneralEvents         *queue.Queue[model.GeneralEvent]
	KillEvents            *queue.Queue[model.KillEvent]
	ChatEvents            *queue.Queue[model.ChatEvent]
	RadioEvents           *queue.Queue[model.RadioEvent]
	FpsEvents             *queue.Queue[model.ServerFpsEvent]
	Ace3DeathEvents       *queue.Queue[model.Ace3DeathEvent]
	Ace3UnconsciousEvents *queue.Queue[model.Ace3UnconsciousEvent]
	Markers               *queue.Queue[model.Marker]
	MarkerStates          *queue.Queue[model.MarkerState]
}

func newQueues() *queues {
	return &queues{
		Soldiers:              queue.New[model.Soldier](),
		SoldierStates:         queue.New[model.SoldierState](),
		Vehicles:              queue.New[model.Vehicle](),
		VehicleStates:         queue.New[model.VehicleState](),
		ProjectileEvents:      queue.New[model.ProjectileEvent](),
		GeneralEvents:         queue.New[model.GeneralEvent](),
		KillEvents:            queue.New[model.KillEvent](),
		ChatEvents:            queue.New[model.ChatEvent](),
		RadioEvents:           queue.New[model.RadioEvent](),
		FpsEvents:             queue.New[model.ServerFpsEvent](),
		Ace3DeathEvents:       queue.New[model.Ace3DeathEvent](),
		Ace3UnconsciousEvents: queue.New[model.Ace3UnconsciousEvent](),
		Markers:               queue.New[model.Marker](),
		MarkerStates:          queue.New[model.MarkerState](),
	}
}

// Backend implements storage.Backend using GORM/PostgreSQL with queue-based batch writes.
type Backend struct {
	deps      Dependencies
	queues    *queues
	missionID atomic.Uint64
	stopChan  chan struct{}
	dbReady   bool
}

// New creates a new GORM storage backend.
func New(deps Dependencies) *Backend {
	return &Backend{
		deps: deps,
	}
}

// Init creates internal queues, runs schema migration, and starts the DB writer goroutine.
// If no DB was injected via Dependencies, it creates its own postgres connection.
func (b *Backend) Init() error {
	b.queues = newQueues()
	b.stopChan = make(chan struct{})

	if b.deps.DB == nil {
		db, err := database.GetPostgresDBStandalone()
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to access sql interface: %w", err)
		}
		if err = sqlDB.Ping(); err != nil {
			return fmt.Errorf("failed to validate connection: %w", err)
		}
		sqlDB.SetMaxOpenConns(10)
		b.deps.DB = db
	}

	if err := b.setupDB(); err != nil {
		return fmt.Errorf("failed to setup DB: %w", err)
	}
	b.dbReady = true

	b.startDBWriters()
	return nil
}

// setupDB migrates tables and creates default group settings if they don't exist.
func (b *Backend) setupDB() error {
	db := b.deps.DB
	log := b.deps.LogManager

	if !db.Migrator().HasTable(&model.OcapInfo{}) {
		if err := db.AutoMigrate(&model.OcapInfo{}); err != nil {
			log.WriteLog("setupDB", fmt.Sprintf("Failed to create ocap_info table: %s", err), "ERROR")
			return fmt.Errorf("failed to auto-migrate OcapInfo: %w", err)
		}
		if err := db.Create(&model.OcapInfo{
			GroupName:        "OCAP",
			GroupDescription: "OCAP",
			GroupLogo:        "https://i.imgur.com/0Q4z0ZP.png",
			GroupWebsite:     "https://ocap.arma3.com",
		}).Error; err != nil {
			return fmt.Errorf("failed to create ocap_info entry: %w", err)
		}
	}

	if db.Name() == "postgres" {
		if err := db.Exec(`CREATE Extension IF NOT EXISTS postgis;`).Error; err != nil {
			return fmt.Errorf("failed to create PostGIS Extension: %w", err)
		}
		log.WriteLog("setupDB", "PostGIS Extension created", "INFO")
	}

	log.WriteLog("setupDB", "Migrating schema", "INFO")
	if err := db.AutoMigrate(model.DatabaseModels...); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}

	log.WriteLog("setupDB", "Database setup complete", "INFO")
	return nil
}

// Close stops the DB writer goroutine.
func (b *Backend) Close() error {
	if b.stopChan != nil {
		close(b.stopChan)
	}
	return nil
}

// StartMission performs addon get-or-create, world get-or-insert, and mission create in the DB.
func (b *Backend) StartMission(coreMission *core.Mission, coreWorld *core.World) error {
	if b.deps.DB == nil {
		return nil
	}

	db := b.deps.DB
	log := b.deps.LogManager

	gormMission := convert.CoreToMission(*coreMission)
	gormWorld := convert.CoreToWorld(*coreWorld)

	// Addon get-or-create
	for i, addon := range gormMission.Addons {
		err := db.Where("name = ?", addon.Name).First(&gormMission.Addons[i]).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				if err = db.Create(&gormMission.Addons[i]).Error; err != nil {
					log.WriteLog("StartMission", fmt.Sprintf("Failed to create addon: %v", err), "ERROR")
					return fmt.Errorf("failed to create addon %s: %w", addon.Name, err)
				}
			} else {
				return fmt.Errorf("failed to find addon %s: %w", addon.Name, err)
			}
		}
	}

	// World get-or-insert
	if err := db.Where(model.World{WorldName: gormWorld.WorldName}).FirstOrCreate(&gormWorld).Error; err != nil {
		return fmt.Errorf("failed to get or insert world: %w", err)
	}

	// Mission create
	gormMission.World = gormWorld
	if err := db.Create(&gormMission).Error; err != nil {
		return fmt.Errorf("failed to insert new mission: %w", err)
	}

	// Assign DB-generated IDs back to core types
	coreMission.ID = gormMission.ID
	coreWorld.ID = gormWorld.ID

	// Store mission ID for the DB writer goroutine
	b.missionID.Store(uint64(gormMission.ID))

	return nil
}

// SetMissionID sets the current mission ID for the DB writer (used by CLI tools).
func (b *Backend) SetMissionID(id uint) {
	b.missionID.Store(uint64(id))
}

// EndMission is a no-op — mission lifecycle is managed by main.go.
func (b *Backend) EndMission() error {
	return nil
}

// AddSoldier converts a core soldier to GORM and pushes to the write queue.
func (b *Backend) AddSoldier(s *core.Soldier) error {
	gormObj := convert.CoreToSoldier(*s)
	b.queues.Soldiers.Push(gormObj)
	return nil
}

// AddVehicle converts a core vehicle to GORM and pushes to the write queue.
func (b *Backend) AddVehicle(v *core.Vehicle) error {
	gormObj := convert.CoreToVehicle(*v)
	b.queues.Vehicles.Push(gormObj)
	return nil
}

// AddMarker inserts a marker synchronously (not queued) because markers are low-volume
// and need immediate ID assignment for the MarkerCache.
// Returns the DB-assigned ID (0 if no DB is configured).
func (b *Backend) AddMarker(m *core.Marker) (uint, error) {
	gormObj := convert.CoreToMarker(*m)
	if b.deps.DB != nil {
		gormObj.MissionID = uint(b.missionID.Load())
		if err := b.deps.DB.Create(&gormObj).Error; err != nil {
			return 0, fmt.Errorf("failed to insert marker: %w", err)
		}
		return gormObj.ID, nil
	}
	return 0, nil
}

// RecordSoldierState converts and queues a soldier state.
func (b *Backend) RecordSoldierState(s *core.SoldierState) error {
	gormObj := convert.CoreToSoldierState(*s)
	b.queues.SoldierStates.Push(gormObj)
	return nil
}

// RecordVehicleState converts and queues a vehicle state.
func (b *Backend) RecordVehicleState(v *core.VehicleState) error {
	gormObj := convert.CoreToVehicleState(*v)
	b.queues.VehicleStates.Push(gormObj)
	return nil
}

// RecordMarkerState converts and queues a marker state.
func (b *Backend) RecordMarkerState(s *core.MarkerState) error {
	gormObj := convert.CoreToMarkerState(*s)
	b.queues.MarkerStates.Push(gormObj)
	return nil
}

// DeleteMarker pushes an alpha=0 MarkerState to the queue and marks the marker as deleted in DB.
func (b *Backend) DeleteMarker(dm *core.DeleteMarker) error {
	markerID, ok := b.deps.MarkerCache.Get(dm.Name)
	if !ok {
		return nil
	}

	deleteState := model.MarkerState{
		MarkerID:     markerID,
		CaptureFrame: dm.EndFrame,
		Time:         time.Now(),
		Alpha:        0,
	}
	b.queues.MarkerStates.Push(deleteState)

	if b.deps.DB != nil {
		b.deps.DB.Model(&model.Marker{}).Where("id = ?", markerID).Update("is_deleted", true)
	}
	return nil
}

// RecordFiredEvent is a no-op — replaced by ProjectileEvent.
func (b *Backend) RecordFiredEvent(e *core.FiredEvent) error {
	return nil
}

// RecordProjectileEvent converts and queues a projectile event.
func (b *Backend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	gormObj := convert.CoreToProjectileEvent(*e)
	b.queues.ProjectileEvents.Push(gormObj)
	return nil
}

// RecordGeneralEvent converts and queues a general event.
func (b *Backend) RecordGeneralEvent(e *core.GeneralEvent) error {
	gormObj := convert.CoreToGeneralEvent(*e)
	b.queues.GeneralEvents.Push(gormObj)
	return nil
}

// RecordHitEvent is a no-op — replaced by ProjectileEvent.
func (b *Backend) RecordHitEvent(e *core.HitEvent) error {
	return nil
}

// RecordKillEvent converts and queues a kill event.
func (b *Backend) RecordKillEvent(e *core.KillEvent) error {
	gormObj := convert.CoreToKillEvent(*e)
	b.queues.KillEvents.Push(gormObj)
	return nil
}

// RecordChatEvent converts and queues a chat event.
func (b *Backend) RecordChatEvent(e *core.ChatEvent) error {
	gormObj := convert.CoreToChatEvent(*e)
	b.queues.ChatEvents.Push(gormObj)
	return nil
}

// RecordRadioEvent converts and queues a radio event.
func (b *Backend) RecordRadioEvent(e *core.RadioEvent) error {
	gormObj := convert.CoreToRadioEvent(*e)
	b.queues.RadioEvents.Push(gormObj)
	return nil
}

// RecordServerFpsEvent converts and queues a server FPS event.
func (b *Backend) RecordServerFpsEvent(e *core.ServerFpsEvent) error {
	gormObj := convert.CoreToServerFpsEvent(*e)
	b.queues.FpsEvents.Push(gormObj)
	return nil
}

// RecordTimeState is a no-op — TimeState is not in DatabaseModels, only used by memory backend.
func (b *Backend) RecordTimeState(t *core.TimeState) error {
	return nil
}

// RecordAce3DeathEvent converts and queues an ACE3 death event.
func (b *Backend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error {
	gormObj := convert.CoreToAce3DeathEvent(*e)
	b.queues.Ace3DeathEvents.Push(gormObj)
	return nil
}

// RecordAce3UnconsciousEvent converts and queues an ACE3 unconscious event.
func (b *Backend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	gormObj := convert.CoreToAce3UnconsciousEvent(*e)
	b.queues.Ace3UnconsciousEvents.Push(gormObj)
	return nil
}

// writeQueue writes all items from a queue to the database in a transaction.
func writeQueue[T any](db *gorm.DB, q *queue.Queue[T], name string, log func(string, string, string), prepare func([]T), onSuccess func([]T)) {
	if q.Empty() {
		return
	}

	tx := db.Begin()
	items := q.GetAndEmpty()
	if prepare != nil {
		prepare(items)
	}
	if err := tx.Create(&items).Error; err != nil {
		log(":DB:WRITER:", fmt.Sprintf("Error creating %s: %v", name, err), "ERROR")
		tx.Rollback()
		q.Push(items...)
		return
	}

	tx.Commit()
	if onSuccess != nil {
		onSuccess(items)
	}
}

// startDBWriters starts the background goroutine that periodically drains queues into the DB.
func (b *Backend) startDBWriters() {
	log := b.deps.LogManager.WriteLog

	go func() {
		for {
			select {
			case <-b.stopChan:
				return
			default:
			}

			if !b.dbReady {
				time.Sleep(1 * time.Second)
				continue
			}

			// Read missionID once per write cycle
			missionID := uint(b.missionID.Load())

			// stampMissionID helpers
			stampSoldiers := func(items []model.Soldier) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampSoldierStates := func(items []model.SoldierState) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampVehicles := func(items []model.Vehicle) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampVehicleStates := func(items []model.VehicleState) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampMarkers := func(items []model.Marker) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampMarkerStates := func(items []model.MarkerState) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampProjectileEvents := func(items []model.ProjectileEvent) {
				for i := range items {
					items[i].MissionID = missionID
					for j := range items[i].HitSoldiers {
						items[i].HitSoldiers[j].MissionID = missionID
					}
					for j := range items[i].HitVehicles {
						items[i].HitVehicles[j].MissionID = missionID
					}
				}
			}
			stampGeneralEvents := func(items []model.GeneralEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampKillEvents := func(items []model.KillEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampChatEvents := func(items []model.ChatEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampRadioEvents := func(items []model.RadioEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampFpsEvents := func(items []model.ServerFpsEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampAce3DeathEvents := func(items []model.Ace3DeathEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}
			stampAce3UnconsciousEvents := func(items []model.Ace3UnconsciousEvent) {
				for i := range items {
					items[i].MissionID = missionID
				}
			}

			// Entities (cache already populated by worker at parse time with core types)
			writeQueue(b.deps.DB, b.queues.Soldiers, "soldiers", log, stampSoldiers, nil)
			writeQueue(b.deps.DB, b.queues.Vehicles, "vehicles", log, stampVehicles, nil)
			writeQueue(b.deps.DB, b.queues.Markers, "markers", log, stampMarkers, func(items []model.Marker) {
				for _, marker := range items {
					if marker.ID != 0 {
						b.deps.MarkerCache.Set(marker.MarkerName, marker.ID)
					}
				}
			})

			// State updates
			writeQueue(b.deps.DB, b.queues.SoldierStates, "soldier states", log, stampSoldierStates, nil)
			writeQueue(b.deps.DB, b.queues.VehicleStates, "vehicle states", log, stampVehicleStates, nil)
			writeQueue(b.deps.DB, b.queues.MarkerStates, "marker states", log, stampMarkerStates, nil)

			// Events
			writeQueue(b.deps.DB, b.queues.ProjectileEvents, "projectile events", log, stampProjectileEvents, nil)
			writeQueue(b.deps.DB, b.queues.GeneralEvents, "general events", log, stampGeneralEvents, nil)
			writeQueue(b.deps.DB, b.queues.KillEvents, "kill events", log, stampKillEvents, nil)
			writeQueue(b.deps.DB, b.queues.ChatEvents, "chat events", log, stampChatEvents, nil)
			writeQueue(b.deps.DB, b.queues.RadioEvents, "radio events", log, stampRadioEvents, nil)
			writeQueue(b.deps.DB, b.queues.FpsEvents, "serverfps events", log, stampFpsEvents, nil)
			writeQueue(b.deps.DB, b.queues.Ace3DeathEvents, "ace3 death events", log, stampAce3DeathEvents, nil)
			writeQueue(b.deps.DB, b.queues.Ace3UnconsciousEvents, "ace3 unconscious events", log, stampAce3UnconsciousEvents, nil)

			time.Sleep(2 * time.Second)
		}
	}()
}
