package worker

import (
	"fmt"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/handlers"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/queue"
	"github.com/OCAP2/extension/v5/internal/storage"

	"gorm.io/gorm"
)

// ErrTooEarlyForStateAssociation is returned when state data arrives before entity is registered
var ErrTooEarlyForStateAssociation = fmt.Errorf("too early for state association")

// Queues holds all the write queues
type Queues struct {
	Soldiers              *queue.SoldiersQueue
	SoldierStates         *queue.SoldierStatesQueue
	Vehicles              *queue.VehiclesQueue
	VehicleStates         *queue.VehicleStatesQueue
	FiredEvents           *queue.FiredEventsQueue
	ProjectileEvents      *queue.ProjectileEventsQueue
	GeneralEvents         *queue.GeneralEventsQueue
	HitEvents             *queue.HitEventsQueue
	KillEvents            *queue.KillEventsQueue
	ChatEvents            *queue.ChatEventsQueue
	RadioEvents           *queue.RadioEventsQueue
	FpsEvents             *queue.FpsEventsQueue
	Ace3DeathEvents       *queue.Ace3DeathEventsQueue
	Ace3UnconsciousEvents *queue.Ace3UnconsciousEventsQueue
	Markers               *queue.MarkersQueue
	MarkerStates          *queue.MarkerStatesQueue
}

// NewQueues creates all write queues
func NewQueues() *Queues {
	return &Queues{
		Soldiers:              &queue.SoldiersQueue{},
		SoldierStates:         &queue.SoldierStatesQueue{},
		Vehicles:              &queue.VehiclesQueue{},
		VehicleStates:         &queue.VehicleStatesQueue{},
		FiredEvents:           &queue.FiredEventsQueue{},
		ProjectileEvents:      &queue.ProjectileEventsQueue{},
		GeneralEvents:         &queue.GeneralEventsQueue{},
		HitEvents:             &queue.HitEventsQueue{},
		KillEvents:            &queue.KillEventsQueue{},
		ChatEvents:            &queue.ChatEventsQueue{},
		RadioEvents:           &queue.RadioEventsQueue{},
		FpsEvents:             &queue.FpsEventsQueue{},
		Ace3DeathEvents:       &queue.Ace3DeathEventsQueue{},
		Ace3UnconsciousEvents: &queue.Ace3UnconsciousEventsQueue{},
		Markers:               &queue.MarkersQueue{},
		MarkerStates:          &queue.MarkerStatesQueue{},
	}
}

// Dependencies holds all dependencies for the worker manager
type Dependencies struct {
	DB              *gorm.DB
	EntityCache     *cache.EntityCache
	MarkerCache     *cache.MarkerCache
	LogManager      *logging.SlogManager
	HandlerService  *handlers.Service
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

// StartDBWriters starts the database writer goroutine
func (m *Manager) StartDBWriters() {
	functionName := ":DB:WRITER:"

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

			// write new soldiers
			if !m.queues.Soldiers.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.Soldiers.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating soldiers: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
					for _, v := range toWrite {
						m.deps.EntityCache.AddSoldier(v)
					}
				}
			}

			// write soldier states
			if !m.queues.SoldierStates.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.SoldierStates.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating soldier states: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write new vehicles
			if !m.queues.Vehicles.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.Vehicles.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating vehicles: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
					for _, v := range toWrite {
						m.deps.EntityCache.AddVehicle(v)
					}
				}
			}

			// write vehicle states
			if !m.queues.VehicleStates.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.VehicleStates.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating vehicle states: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					m.deps.LogManager.WriteLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write fired events
			if !m.queues.FiredEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.FiredEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating fired events: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					m.deps.LogManager.WriteLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write projectile events
			if !m.queues.ProjectileEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.ProjectileEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating projectile events: %v`, err), "ERROR")
					stmt := tx.Statement.SQL.String()
					m.deps.LogManager.WriteLog(functionName, stmt, "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write general events
			if !m.queues.GeneralEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.GeneralEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating general events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write hit events
			if !m.queues.HitEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.HitEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating hit events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write kill events
			if !m.queues.KillEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.KillEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating killed events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write chat events
			if !m.queues.ChatEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.ChatEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating chat events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write radio events
			if !m.queues.RadioEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.RadioEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating radio events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write serverfps events
			if !m.queues.FpsEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.FpsEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating serverfps events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write ace3 death events
			if !m.queues.Ace3DeathEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.Ace3DeathEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating ace3 death events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write ace3 unconscious events
			if !m.queues.Ace3UnconsciousEvents.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.Ace3UnconsciousEvents.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating ace3 unconscious events: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			// write markers
			if !m.queues.Markers.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.Markers.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating markers: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
					for _, marker := range toWrite {
						if marker.ID != 0 {
							m.deps.MarkerCache.Set(marker.MarkerName, marker.ID)
						}
					}
				}
			}

			// write marker states
			if !m.queues.MarkerStates.Empty() {
				tx := m.deps.DB.Begin()
				toWrite := m.queues.MarkerStates.GetAndEmpty()
				err := tx.Create(&toWrite).Error
				if err != nil {
					m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Error creating marker states: %v`, err), "ERROR")
					tx.Rollback()
				} else {
					tx.Commit()
				}
			}

			m.lastDBWriteDuration = time.Since(writeStart)

			// sleep
			time.Sleep(2 * time.Second)
		}
	}()
}
