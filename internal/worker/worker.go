package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/OCAP2/extension/internal/cache"
	"github.com/OCAP2/extension/internal/handlers"
	"github.com/OCAP2/extension/internal/logging"
	"github.com/OCAP2/extension/internal/model"
	"github.com/OCAP2/extension/internal/queue"

	influxdb2_write "github.com/influxdata/influxdb-client-go/v2/api/write"
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
	DB               *gorm.DB
	EntityCache      *cache.EntityCache
	MarkerCache      *cache.MarkerCache
	LogManager       *logging.Manager
	HandlerService   *handlers.Service
	IsDatabaseValid  func() bool
	ShouldSaveLocal  func() bool
	DBInsertsPaused  func() bool
}

// InfluxWriter is a function type for writing to InfluxDB
type InfluxWriter func(ctx context.Context, bucket string, point *influxdb2_write.Point) error

// MetricProcessor is a function type for processing metric data
type MetricProcessor func(data []string) (bucket string, point *influxdb2_write.Point, err error)

// Manager manages worker goroutines
type Manager struct {
	deps                Dependencies
	queues              *Queues
	channels            map[string]chan []string
	lastDBWriteDuration time.Duration

	// Optional InfluxDB integration
	influxWriter     InfluxWriter
	metricProcessor  MetricProcessor
	influxEnabled    func() bool
}

// NewManager creates a new worker manager
func NewManager(deps Dependencies, queues *Queues, channels map[string]chan []string) *Manager {
	return &Manager{
		deps:     deps,
		queues:   queues,
		channels: channels,
		influxEnabled: func() bool { return false },
	}
}

// SetInfluxIntegration sets up InfluxDB integration
func (m *Manager) SetInfluxIntegration(writer InfluxWriter, processor MetricProcessor, enabledFn func() bool) {
	m.influxWriter = writer
	m.metricProcessor = processor
	m.influxEnabled = enabledFn
}

// GetLastDBWriteDuration returns the duration of the last DB write cycle
func (m *Manager) GetLastDBWriteDuration() time.Duration {
	return m.lastDBWriteDuration
}

// StartAsyncProcessors starts all async data processors
func (m *Manager) StartAsyncProcessors() {
	functionName := ":DATA:PROCESSOR:"

	// process new soldiers
	go func() {
		for v := range m.channels[":NEW:SOLDIER:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogNewSoldier(v)
			if err == nil {
				m.queues.Soldiers.Push([]model.Soldier{obj})
			} else {
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log new soldier. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process new vehicles
	go func() {
		for v := range m.channels[":NEW:VEHICLE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogNewVehicle(v)
			if err == nil {
				m.queues.Vehicles.Push([]model.Vehicle{obj})
			} else {
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log new vehicle. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process soldier states
	go func() {
		for v := range m.channels[":NEW:SOLDIER:STATE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogSoldierState(v)
			if err == nil {
				m.queues.SoldierStates.Push([]model.SoldierState{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log soldier state. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process vehicle states
	go func() {
		for v := range m.channels[":NEW:VEHICLE:STATE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogVehicleState(v)
			if err == nil {
				m.queues.VehicleStates.Push([]model.VehicleState{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log vehicle state. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process fired events
	go func() {
		for v := range m.channels[":FIRED:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogFiredEvent(v)
			if err == nil {
				m.queues.FiredEvents.Push([]model.FiredEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log fired event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process projectile events
	go func() {
		for v := range m.channels[":PROJECTILE:"] {
			// new projectile events use linestringzm geo format, which is not supported by SQLite
			if !m.deps.IsDatabaseValid() || m.deps.ShouldSaveLocal() {
				continue
			}

			obj, err := m.deps.HandlerService.LogProjectileEvent(v)
			if err == nil {
				m.queues.ProjectileEvents.Push([]model.ProjectileEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log projectile event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process general events
	go func() {
		for v := range m.channels[":EVENT:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogGeneralEvent(v)
			if err == nil {
				m.queues.GeneralEvents.Push([]model.GeneralEvent{obj})
			} else {
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log general event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process hit events
	go func() {
		for v := range m.channels[":HIT:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogHitEvent(v)
			if err == nil {
				m.queues.HitEvents.Push([]model.HitEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log hit event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process kill events
	go func() {
		for v := range m.channels[":KILL:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogKillEvent(v)
			if err == nil {
				m.queues.KillEvents.Push([]model.KillEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log kill event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process chat events
	go func() {
		for v := range m.channels[":CHAT:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogChatEvent(v)
			if err == nil {
				m.queues.ChatEvents.Push([]model.ChatEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log chat event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process radio events
	go func() {
		for v := range m.channels[":RADIO:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogRadioEvent(v)
			if err == nil {
				m.queues.RadioEvents.Push([]model.RadioEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log radio event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process fps events
	go func() {
		for v := range m.channels[":FPS:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogFpsEvent(v)
			if err == nil {
				m.queues.FpsEvents.Push([]model.ServerFpsEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log fps event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process ace3 death events
	go func() {
		for v := range m.channels[":ACE3:DEATH:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogAce3DeathEvent(v)
			if err == nil {
				m.queues.Ace3DeathEvents.Push([]model.Ace3DeathEvent{obj})
			} else {
				if err == ErrTooEarlyForStateAssociation {
					continue
				}
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log ace3 death event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process ace3 unconscious events
	go func() {
		for v := range m.channels[":ACE3:UNCONSCIOUS:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			obj, err := m.deps.HandlerService.LogAce3UnconsciousEvent(v)
			if err == nil {
				m.queues.Ace3UnconsciousEvents.Push([]model.Ace3UnconsciousEvent{obj})
			} else {
				m.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to log ace3 unconscious event. Err: %s`, err), "ERROR")
			}
		}
	}()

	// process metric data
	go func() {
		for data := range m.channels[":METRIC:"] {
			if m.influxWriter == nil || m.metricProcessor == nil || !m.influxEnabled() {
				continue
			}

			bucket, point, err := m.metricProcessor(data)
			if err != nil {
				m.deps.LogManager.Logger.Error().Err(err).Msg("error processing metric data")
				continue
			}

			err = m.influxWriter(context.Background(), bucket, point)
			if err != nil {
				m.deps.LogManager.Logger.Error().Err(err).Msg("error writing influx point")
				continue
			}
		}
	}()

	// process marker create events
	go func() {
		for data := range m.channels[":MARKER:CREATE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			marker, err := m.deps.HandlerService.LogMarkerCreate(data)
			if err != nil {
				m.deps.LogManager.Logger.Error().Err(err).Msg("Error processing marker create")
				continue
			}
			m.queues.Markers.Push([]model.Marker{marker})
		}
	}()

	// process marker move events
	go func() {
		for data := range m.channels[":MARKER:MOVE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			markerState, err := m.deps.HandlerService.LogMarkerMove(data)
			if err != nil {
				m.deps.LogManager.Logger.Warn().Err(err).Msg("Error processing marker move")
				continue
			}
			m.queues.MarkerStates.Push([]model.MarkerState{markerState})
		}
	}()

	// process marker delete events
	go func() {
		for data := range m.channels[":MARKER:DELETE:"] {
			if !m.deps.IsDatabaseValid() {
				continue
			}

			markerName, frameNo, err := m.deps.HandlerService.LogMarkerDelete(data)
			if err != nil {
				m.deps.LogManager.Logger.Error().Err(err).Msg("Error processing marker delete")
				continue
			}

			// Look up marker and mark as deleted
			markerID, ok := m.deps.MarkerCache.Get(markerName)
			if ok {
				// Create a marker state marking deletion
				deleteState := model.MarkerState{
					MissionID:    m.deps.HandlerService.GetMissionContext().GetMission().ID,
					MarkerID:     markerID,
					CaptureFrame: frameNo,
					Time:         time.Now(),
					Alpha:        0, // Zero alpha indicates deletion
				}
				m.queues.MarkerStates.Push([]model.MarkerState{deleteState})

				// Update marker as deleted in DB
				m.deps.DB.Model(&model.Marker{}).Where("id = ?", markerID).Update("is_deleted", true)
			}
		}
	}()
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
