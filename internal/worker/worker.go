package worker

import (
	"fmt"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/queue"
	"github.com/OCAP2/extension/v5/internal/storage"

	"gorm.io/gorm"
)

// ErrTooEarlyForStateAssociation is returned when state data arrives before entity is registered
var ErrTooEarlyForStateAssociation = fmt.Errorf("too early for state association")

// ParserService defines the interface for data parsers
type ParserService interface {
	ParseMission(args []string) (model.Mission, model.World, error)
	ParseSoldier(args []string) (model.Soldier, error)
	ParseVehicle(args []string) (model.Vehicle, error)
	ParseSoldierState(args []string) (model.SoldierState, error)
	ParseVehicleState(args []string) (model.VehicleState, error)
	ParseProjectileEvent(args []string) (parser.ParsedProjectileEvent, error)
	ParseGeneralEvent(args []string) (model.GeneralEvent, error)
	ParseKillEvent(args []string) (parser.ParsedKillEvent, error)
	ParseChatEvent(args []string) (model.ChatEvent, error)
	ParseRadioEvent(args []string) (model.RadioEvent, error)
	ParseFpsEvent(args []string) (model.ServerFpsEvent, error)
	ParseTimeState(args []string) (model.TimeState, error)
	ParseAce3DeathEvent(args []string) (model.Ace3DeathEvent, error)
	ParseAce3UnconsciousEvent(args []string) (model.Ace3UnconsciousEvent, error)
	ParseMarkerCreate(args []string) (model.Marker, error)
	ParseMarkerMove(args []string) (parser.ParsedMarkerMove, error)
	ParseMarkerDelete(args []string) (string, uint, error)
}

// Queues holds all the write queues
type Queues struct {
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

// NewQueues creates all write queues
func NewQueues() *Queues {
	return &Queues{
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

// Dependencies holds all dependencies for the worker manager
type Dependencies struct {
	DB              *gorm.DB
	EntityCache     *cache.EntityCache
	MarkerCache     *cache.MarkerCache
	LogManager      *logging.SlogManager
	ParserService   ParserService
	MissionContext  *parser.MissionContext
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
// If prepare is provided, it's called with the items before inserting (e.g. to stamp MissionID).
// If onSuccess is provided, it's called with the written items after successful commit.
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

			// Read missionID once per write cycle
			missionID := m.deps.MissionContext.GetMission().ID

			// stampMissionID is a helper for simple structs with a MissionID field
			stampSoldiers := func(items []model.Soldier) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampSoldierStates := func(items []model.SoldierState) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampVehicles := func(items []model.Vehicle) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampVehicleStates := func(items []model.VehicleState) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampMarkers := func(items []model.Marker) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampMarkerStates := func(items []model.MarkerState) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampProjectileEvents := func(items []model.ProjectileEvent) {
				for i := range items {
					items[i].MissionID = missionID
					for j := range items[i].HitSoldiers { items[i].HitSoldiers[j].MissionID = missionID }
					for j := range items[i].HitVehicles { items[i].HitVehicles[j].MissionID = missionID }
				}
			}
			stampGeneralEvents := func(items []model.GeneralEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampKillEvents := func(items []model.KillEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampChatEvents := func(items []model.ChatEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampRadioEvents := func(items []model.RadioEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampFpsEvents := func(items []model.ServerFpsEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampAce3DeathEvents := func(items []model.Ace3DeathEvent) {
				for i := range items { items[i].MissionID = missionID }
			}
			stampAce3UnconsciousEvents := func(items []model.Ace3UnconsciousEvent) {
				for i := range items { items[i].MissionID = missionID }
			}

			// Entities with cache updates
			writeQueue(m.deps.DB, m.queues.Soldiers, "soldiers", log, stampSoldiers, func(items []model.Soldier) {
				for _, v := range items {
					m.deps.EntityCache.AddSoldier(v)
				}
			})
			writeQueue(m.deps.DB, m.queues.Vehicles, "vehicles", log, stampVehicles, func(items []model.Vehicle) {
				for _, v := range items {
					m.deps.EntityCache.AddVehicle(v)
				}
			})
			writeQueue(m.deps.DB, m.queues.Markers, "markers", log, stampMarkers, func(items []model.Marker) {
				for _, marker := range items {
					if marker.ID != 0 {
						m.deps.MarkerCache.Set(marker.MarkerName, marker.ID)
					}
				}
			})

			// State updates
			writeQueue(m.deps.DB, m.queues.SoldierStates, "soldier states", log, stampSoldierStates, nil)
			writeQueue(m.deps.DB, m.queues.VehicleStates, "vehicle states", log, stampVehicleStates, nil)
			writeQueue(m.deps.DB, m.queues.MarkerStates, "marker states", log, stampMarkerStates, nil)

			// Events
			writeQueue(m.deps.DB, m.queues.ProjectileEvents, "projectile events", log, stampProjectileEvents, nil)
			writeQueue(m.deps.DB, m.queues.GeneralEvents, "general events", log, stampGeneralEvents, nil)
			writeQueue(m.deps.DB, m.queues.KillEvents, "kill events", log, stampKillEvents, nil)
			writeQueue(m.deps.DB, m.queues.ChatEvents, "chat events", log, stampChatEvents, nil)
			writeQueue(m.deps.DB, m.queues.RadioEvents, "radio events", log, stampRadioEvents, nil)
			writeQueue(m.deps.DB, m.queues.FpsEvents, "serverfps events", log, stampFpsEvents, nil)
			writeQueue(m.deps.DB, m.queues.Ace3DeathEvents, "ace3 death events", log, stampAce3DeathEvents, nil)
			writeQueue(m.deps.DB, m.queues.Ace3UnconsciousEvents, "ace3 unconscious events", log, stampAce3UnconsciousEvents, nil)

			m.lastDBWriteDuration = time.Since(writeStart)
			time.Sleep(2 * time.Second)
		}
	}()
}
