package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OCAP2/extension/internal/handlers"
	"github.com/OCAP2/extension/internal/logging"
	"github.com/OCAP2/extension/internal/model"
	"github.com/OCAP2/extension/internal/worker"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2_write "github.com/influxdata/influxdb-client-go/v2/api/write"
	"gorm.io/gorm"
)

// InfluxWriter is a function type for writing to InfluxDB
type InfluxWriter func(ctx context.Context, bucket string, point *influxdb2_write.Point) error

// Dependencies holds all dependencies for the monitor service
type Dependencies struct {
	DB              *gorm.DB
	LogManager      *logging.Manager
	HandlerService  *handlers.Service
	WorkerManager   *worker.Manager
	Queues          *worker.Queues
	AddonFolder     string
	IsDatabaseValid func() bool
}

// Service manages status monitoring
type Service struct {
	deps      Dependencies
	isRunning bool
	mu        sync.RWMutex
	stopChan  chan struct{}

	// Optional InfluxDB integration
	influxWriter  InfluxWriter
	influxEnabled func() bool
	influxDBURL   func() string
}

// NewService creates a new monitor service
func NewService(deps Dependencies) *Service {
	return &Service{
		deps:          deps,
		stopChan:      make(chan struct{}),
		influxEnabled: func() bool { return false },
		influxDBURL:   func() string { return "" },
	}
}

// SetInfluxIntegration sets up InfluxDB integration
func (s *Service) SetInfluxIntegration(writer InfluxWriter, enabledFn func() bool, dbURLFn func() string) {
	s.influxWriter = writer
	s.influxEnabled = enabledFn
	s.influxDBURL = dbURLFn
}

// IsRunning returns whether the status monitor is running
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetProgramStatus returns the current program status
func (s *Service) GetProgramStatus(
	rawBuffers bool,
	writeQueues bool,
	lastWrite bool,
) (output []string, perfModel model.OcapPerformance) {
	mission := s.deps.HandlerService.GetMissionContext().GetMission()

	// Buffer lengths are now tracked via OTEL metrics in the dispatcher
	buffersObj := model.BufferLengths{}

	writeQueuesObj := model.WriteQueueLengths{
		Soldiers:              uint16(s.deps.Queues.Soldiers.Len()),
		Vehicles:              uint16(s.deps.Queues.Vehicles.Len()),
		SoldierStates:         uint16(s.deps.Queues.SoldierStates.Len()),
		VehicleStates:         uint16(s.deps.Queues.VehicleStates.Len()),
		FiredEvents:           uint16(s.deps.Queues.FiredEvents.Len()),
		GeneralEvents:         uint16(s.deps.Queues.GeneralEvents.Len()),
		HitEvents:             uint16(s.deps.Queues.HitEvents.Len()),
		KillEvents:            uint16(s.deps.Queues.KillEvents.Len()),
		ChatEvents:            uint16(s.deps.Queues.ChatEvents.Len()),
		RadioEvents:           uint16(s.deps.Queues.RadioEvents.Len()),
		ServerFpsEvents:       uint16(s.deps.Queues.FpsEvents.Len()),
		Ace3DeathEvents:       uint16(s.deps.Queues.Ace3DeathEvents.Len()),
		Ace3UnconsciousEvents: uint16(s.deps.Queues.Ace3UnconsciousEvents.Len()),
		Markers:               uint16(s.deps.Queues.Markers.Len()),
		MarkerStates:          uint16(s.deps.Queues.MarkerStates.Len()),
	}

	perf := model.OcapPerformance{
		Time:              time.Now(),
		Mission:           *mission,
		BufferLengths:     buffersObj,
		WriteQueueLengths: writeQueuesObj,
		LastWriteDurationMs: float32(s.deps.WorkerManager.GetLastDBWriteDuration().Milliseconds()),
	}

	if rawBuffers {
		rawBuffersStr, err := json.MarshalIndent(buffersObj, "", "  ")
		if err != nil {
			rawBuffersStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(rawBuffersStr))
	}
	if writeQueues {
		writeQueuesStr, err := json.MarshalIndent(writeQueuesObj, "", "  ")
		if err != nil {
			writeQueuesStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(writeQueuesStr))
	}
	if lastWrite {
		lastWriteStr, err := json.MarshalIndent(perf.LastWriteDurationMs, "", "  ")
		if err != nil {
			lastWriteStr = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}
		output = append(output, string(lastWriteStr))
	}

	return output, perf
}

// ValidateHypertables validates and creates TimescaleDB hypertables
func (s *Service) ValidateHypertables(tables map[string][]string) error {
	functionName := "validateHypertables"

	all := []any{}
	s.deps.DB.Exec(`SELECT x.* FROM timescaledb_information.hypertables`).Scan(&all)
	for _, row := range all {
		s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`hypertable row: %v`, row), "DEBUG")
	}

	for table := range tables {
		hypertable := any(nil)
		s.deps.DB.Exec(`SELECT x.* FROM timescaledb_information.hypertables WHERE hypertable_name = ?`, table).Scan(&hypertable)
		if hypertable != nil {
			s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Table %s is already configured`, table), "INFO")
			continue
		}

		queryCreateHypertable := fmt.Sprintf(`
				SELECT create_hypertable('%s', 'time', chunk_time_interval => interval '1 day', if_not_exists => true);
			`, table)
		err := s.deps.DB.Exec(queryCreateHypertable).Error
		if err != nil {
			s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to create hypertable for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Created hypertable for %s`, table), "INFO")

		queryCompressHypertable := fmt.Sprintf(`
				ALTER TABLE %s SET (
					timescaledb.compress,
					timescaledb.compress_segmentby = ?);
			`, table)
		err = s.deps.DB.Exec(
			queryCompressHypertable,
			strings.Join(tables[table], ","),
		).Error
		if err != nil {
			s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to enable compression for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Enabled hypertable compression for %s`, table), "INFO")

		queryCompressAfterHypertable := fmt.Sprintf(`
				SELECT add_compression_policy(
					'%s',
					compress_after => interval '14 day');
			`, table)
		err = s.deps.DB.Exec(queryCompressAfterHypertable).Error
		if err != nil {
			s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Failed to set compress_after for %s. Err: %s`, table, err), "ERROR")
			return err
		}
		s.deps.LogManager.WriteLog(functionName, fmt.Sprintf(`Set compress_after for %s`, table), "INFO")
	}
	return nil
}

// Start starts the status monitor goroutine
func (s *Service) Start() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return nil
	}
	s.isRunning = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.mu.Unlock()
		}()

		s.deps.LogManager.Logger.Debug().
			Str("function", "startStatusMonitor").
			Msg("Starting status monitor goroutine")

		statusFile, err := os.Create(s.deps.AddonFolder + "/status.txt")
		if err != nil {
			s.deps.LogManager.Logger.Error().Err(err).Msg("Error creating status file")
		}
		defer statusFile.Close()

		for {
			select {
			case <-s.stopChan:
				return
			default:
				time.Sleep(1000 * time.Millisecond)

				mission := s.deps.HandlerService.GetMissionContext().GetMission()
				if mission.ID == 0 {
					continue
				}

				statusStr, perfModel := s.GetProgramStatus(true, true, true)

				if statusFile != nil {
					statusFile.Truncate(0)
					statusFile.Seek(0, 0)
					for _, line := range statusStr {
						statusFile.WriteString(line + "\n")
					}
				}

				// write model to Postgres
				if s.deps.IsDatabaseValid() {
					err = s.deps.DB.Create(&perfModel).Error
					if err != nil {
						s.deps.LogManager.Logger.Error().Err(err).Msg("Error writing perf model to Postgres")
					}
				}

				// write to influxDB
				if s.influxEnabled() && s.influxWriter != nil {
					// write buffer lengths
					p := influxdb2.NewPointWithMeasurement(
						"ext_buffer_lengths",
					).
						AddTag("db_url", s.influxDBURL()).
						AddTag("mission_name", perfModel.Mission.MissionName).
						AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
						AddField("soldiers", perfModel.BufferLengths.Soldiers).
						AddField("vehicles", perfModel.BufferLengths.Vehicles).
						AddField("soldier_states", perfModel.BufferLengths.SoldierStates).
						AddField("vehicle_states", perfModel.BufferLengths.VehicleStates).
						AddField("general_events", perfModel.BufferLengths.GeneralEvents).
						AddField("hit_events", perfModel.BufferLengths.HitEvents).
						AddField("kill_events", perfModel.BufferLengths.KillEvents).
						AddField("fired_events", perfModel.BufferLengths.FiredEvents).
						AddField("chat_events", perfModel.BufferLengths.ChatEvents).
						AddField("radio_events", perfModel.BufferLengths.RadioEvents).
						AddField("server_fps_events", perfModel.BufferLengths.ServerFpsEvents).
						SetTime(time.Now())

					s.influxWriter(
						context.Background(),
						"ocap_performance",
						p,
					)

					// write db write queue lengths
					p = influxdb2.NewPointWithMeasurement(
						"ext_db_queue_lengths",
					).
						AddTag("db_url", s.influxDBURL()).
						AddTag("mission_name", perfModel.Mission.MissionName).
						AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
						AddField("soldiers", perfModel.WriteQueueLengths.Soldiers).
						AddField("vehicles", perfModel.WriteQueueLengths.Vehicles).
						AddField("soldier_states", perfModel.WriteQueueLengths.SoldierStates).
						AddField("vehicle_states", perfModel.WriteQueueLengths.VehicleStates).
						AddField("general_events", perfModel.WriteQueueLengths.GeneralEvents).
						AddField("hit_events", perfModel.WriteQueueLengths.HitEvents).
						AddField("kill_events", perfModel.WriteQueueLengths.KillEvents).
						AddField("fired_events", perfModel.WriteQueueLengths.FiredEvents).
						AddField("chat_events", perfModel.WriteQueueLengths.ChatEvents).
						AddField("radio_events", perfModel.WriteQueueLengths.RadioEvents).
						AddField("server_fps_events", perfModel.WriteQueueLengths.ServerFpsEvents).
						SetTime(time.Now())

					s.influxWriter(
						context.Background(),
						"ocap_performance",
						p,
					)

					// write last write duration
					p = influxdb2.NewPointWithMeasurement(
						"ext_db_lastwrite_duration_ms",
					).
						AddTag("db_url", s.influxDBURL()).
						AddTag("mission_name", perfModel.Mission.MissionName).
						AddTag("mission_id", fmt.Sprintf("%d", perfModel.Mission.ID)).
						AddField("value", perfModel.LastWriteDurationMs).
						SetTime(time.Now())

					s.influxWriter(
						context.Background(),
						"ocap_performance",
						p,
					)
				}
			}
		}
	}()

	return nil
}

// Stop stops the status monitor
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		close(s.stopChan)
	}
}
