package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/mission"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/worker"

	"gorm.io/gorm"
)

// Dependencies holds all dependencies for the monitor service
type Dependencies struct {
	DB              *gorm.DB
	LogManager      *logging.SlogManager
	MissionContext  *mission.Context
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
}

// NewService creates a new monitor service
func NewService(deps Dependencies) *Service {
	return &Service{
		deps:     deps,
		stopChan: make(chan struct{}),
	}
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
	mission := s.deps.MissionContext.GetMission()

	// Buffer lengths are now tracked via OTEL metrics in the dispatcher
	buffersObj := model.BufferLengths{}

	writeQueuesObj := model.WriteQueueLengths{
		Soldiers:              uint16(s.deps.Queues.Soldiers.Len()),
		Vehicles:              uint16(s.deps.Queues.Vehicles.Len()),
		SoldierStates:         uint16(s.deps.Queues.SoldierStates.Len()),
		VehicleStates:         uint16(s.deps.Queues.VehicleStates.Len()),
		GeneralEvents:         uint16(s.deps.Queues.GeneralEvents.Len()),
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

		logger := s.deps.LogManager.Logger()
		logger.Debug("Starting status monitor goroutine", "function", "startStatusMonitor")

		statusFile, err := os.Create(s.deps.AddonFolder + "/status.txt")
		if err != nil {
			logger.Error("Error creating status file", "error", err)
		}
		defer statusFile.Close()

		for {
			select {
			case <-s.stopChan:
				return
			default:
				time.Sleep(1000 * time.Millisecond)

				mission := s.deps.MissionContext.GetMission()
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
						logger.Error("Error writing perf model to Postgres", "error", err)
					}
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
