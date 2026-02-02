package worker

import (
	"fmt"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/handlers"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/queue"
	"github.com/OCAP2/extension/v5/internal/storage"

	"gorm.io/gorm"
)

// ErrTooEarlyForStateAssociation is returned when state data arrives before entity is registered
var ErrTooEarlyForStateAssociation = fmt.Errorf("too early for state association")

// HandlerService defines the interface for event handlers
type HandlerService interface {
	LogNewSoldier(args []string) (model.Soldier, error)
	LogNewVehicle(args []string) (model.Vehicle, error)
	LogSoldierState(args []string) (model.SoldierState, error)
	LogVehicleState(args []string) (model.VehicleState, error)
	LogFiredEvent(args []string) (model.FiredEvent, error)
	LogProjectileEvent(args []string) (model.ProjectileEvent, error)
	LogGeneralEvent(args []string) (model.GeneralEvent, error)
	LogHitEvent(args []string) (model.HitEvent, error)
	LogKillEvent(args []string) (model.KillEvent, error)
	LogChatEvent(args []string) (model.ChatEvent, error)
	LogRadioEvent(args []string) (model.RadioEvent, error)
	LogFpsEvent(args []string) (model.ServerFpsEvent, error)
	LogAce3DeathEvent(args []string) (model.Ace3DeathEvent, error)
	LogAce3UnconsciousEvent(args []string) (model.Ace3UnconsciousEvent, error)
	LogMarkerCreate(args []string) (model.Marker, error)
	LogMarkerMove(args []string) (model.MarkerState, error)
	LogMarkerDelete(args []string) (string, uint, error)
	GetMissionContext() *handlers.MissionContext
}

// Queues holds all the write queues
type Queues struct {
	Soldiers              *queue.Queue[model.Soldier]
	SoldierStates         *queue.Queue[model.SoldierState]
	Vehicles              *queue.Queue[model.Vehicle]
	VehicleStates         *queue.Queue[model.VehicleState]
	FiredEvents           *queue.Queue[model.FiredEvent]
	ProjectileEvents      *queue.Queue[model.ProjectileEvent]
	GeneralEvents         *queue.Queue[model.GeneralEvent]
	HitEvents             *queue.Queue[model.HitEvent]
	KillEvents            *queue.Queue[model.KillEvent]
	ChatEvents            *queue.Queue[model.ChatEvent]
	RadioEvents           *queue.Queue[model.RadioEvent]
	FpsEvents             *queue.Queue[model.ServerFpsEvent]
	Ace3DeathEvents       *queue.Queue[model.Ace3DeathEvent]
	Ace3UnconsciousEvents *queue.Queue[model.Ace3UnconsciousEvent]
	Markers               *queue.Queue[model.Marker]
	MarkerStates          *queue.Queue[model.MarkerState]
}

// NewQueues creates all write queues
func NewQueues() *Queues {
	return &Queues{
		Soldiers:              queue.New[model.Soldier](),
		SoldierStates:         queue.New[model.SoldierState](),
		Vehicles:              queue.New[model.Vehicle](),
		VehicleStates:         queue.New[model.VehicleState](),
		FiredEvents:           queue.New[model.FiredEvent](),
		ProjectileEvents:      queue.New[model.ProjectileEvent](),
		GeneralEvents:         queue.New[model.GeneralEvent](),
		HitEvents:             queue.New[model.HitEvent](),
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

// Dependencies holds all dependencies for the worker manager
type Dependencies struct {
	DB              *gorm.DB
	EntityCache     *cache.EntityCache
	MarkerCache     *cache.MarkerCache
	LogManager      *logging.SlogManager
	HandlerService  HandlerService
	IsDatabaseValid func() bool
	ShouldSaveLocal func() bool
	DBInsertsPaused func() bool
}

// Manager manages worker goroutines
type Manager struct {
	deps                Dependencies
	queues              *Queues
	lastDBWriteDuration time.Duration

	// Optional storage backend
	backend storage.Backend
}

// NewManager creates a new worker manager
func NewManager(deps Dependencies, queues *Queues) *Manager {
	return &Manager{
		deps:   deps,
		queues: queues,
	}
}

// SetBackend sets the storage backend (replaces GORM path when set)
func (m *Manager) SetBackend(b storage.Backend) {
	m.backend = b
}

// hasBackend returns true if a storage backend is configured
func (m *Manager) hasBackend() bool {
	return m.backend != nil
}

// GetLastDBWriteDuration returns the duration of the last DB write cycle
func (m *Manager) GetLastDBWriteDuration() time.Duration {
	return m.lastDBWriteDuration
}

// writeQueue writes all items from a queue to the database in a transaction.
// If onSuccess is provided, it's called with the written items after successful commit.
func writeQueue[T any](db *gorm.DB, q *queue.Queue[T], name string, log func(string, string, string), onSuccess func([]T)) {
	if q.Empty() {
		return
	}

	tx := db.Begin()
	items := q.GetAndEmpty()
	if err := tx.Create(&items).Error; err != nil {
		log(":DB:WRITER:", fmt.Sprintf("Error creating %s: %v", name, err), "ERROR")
		tx.Rollback()
		return
	}

	tx.Commit()
	if onSuccess != nil {
		onSuccess(items)
	}
}

// StartDBWriters starts the database writer goroutine
func (m *Manager) StartDBWriters() {
	log := m.deps.LogManager.WriteLog

	go func() {
		for {
			if !m.deps.IsDatabaseValid() {
				time.Sleep(1 * time.Second)
				continue
			}

			if m.deps.DBInsertsPaused() {
				time.Sleep(1 * time.Second)
				continue
			}

			writeStart := time.Now()

			// Entities with cache updates
			writeQueue(m.deps.DB, m.queues.Soldiers, "soldiers", log, func(items []model.Soldier) {
				for _, v := range items {
					m.deps.EntityCache.AddSoldier(v)
				}
			})
			writeQueue(m.deps.DB, m.queues.Vehicles, "vehicles", log, func(items []model.Vehicle) {
				for _, v := range items {
					m.deps.EntityCache.AddVehicle(v)
				}
			})
			writeQueue(m.deps.DB, m.queues.Markers, "markers", log, func(items []model.Marker) {
				for _, marker := range items {
					if marker.ID != 0 {
						m.deps.MarkerCache.Set(marker.MarkerName, marker.ID)
					}
				}
			})

			// State updates (no post-processing)
			writeQueue(m.deps.DB, m.queues.SoldierStates, "soldier states", log, nil)
			writeQueue(m.deps.DB, m.queues.VehicleStates, "vehicle states", log, nil)
			writeQueue(m.deps.DB, m.queues.MarkerStates, "marker states", log, nil)

			// Events (no post-processing)
			writeQueue(m.deps.DB, m.queues.FiredEvents, "fired events", log, nil)
			writeQueue(m.deps.DB, m.queues.ProjectileEvents, "projectile events", log, nil)
			writeQueue(m.deps.DB, m.queues.GeneralEvents, "general events", log, nil)
			writeQueue(m.deps.DB, m.queues.HitEvents, "hit events", log, nil)
			writeQueue(m.deps.DB, m.queues.KillEvents, "kill events", log, nil)
			writeQueue(m.deps.DB, m.queues.ChatEvents, "chat events", log, nil)
			writeQueue(m.deps.DB, m.queues.RadioEvents, "radio events", log, nil)
			writeQueue(m.deps.DB, m.queues.FpsEvents, "serverfps events", log, nil)
			writeQueue(m.deps.DB, m.queues.Ace3DeathEvents, "ace3 death events", log, nil)
			writeQueue(m.deps.DB, m.queues.Ace3UnconsciousEvents, "ace3 unconscious events", log, nil)

			m.lastDBWriteDuration = time.Since(writeStart)
			time.Sleep(2 * time.Second)
		}
	}()
}
