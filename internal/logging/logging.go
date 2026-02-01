package logging

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/rs/zerolog"
)

// Config holds logging configuration
type Config struct {
	LogLevel       string
	LogsDir        string
	CurrentMission string
	MissionID      uint
	UsingLocalDB   bool
}

// Manager manages all logging operations
type Manager struct {
	Logger      zerolog.Logger
	JSONLogger  zerolog.Logger
	TraceSample zerolog.Logger

	logIoConn     net.Conn
	graylogWriter *gelf.Writer
	config        *Config
	nullByte      byte

	// Callbacks for dynamic state
	GetMissionName    func() string
	GetMissionID      func() uint
	IsUsingLocalDB    func() bool
	IsStatusRunning   func() bool
}

// NewManager creates a new logging manager
func NewManager() *Manager {
	return &Manager{
		nullByte: 0x00,
		config:   &Config{},
		// Default callbacks return empty/false values
		GetMissionName:  func() string { return "" },
		GetMissionID:    func() uint { return 0 },
		IsUsingLocalDB:  func() bool { return false },
		IsStatusRunning: func() bool { return false },
	}
}

// Setup initializes the logging system
func (m *Manager) Setup(file *os.File, logLevel string) {
	// get log level
	var logLevelActual zerolog.Level
	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		logLevelActual = zerolog.DebugLevel
	case "INFO":
		logLevelActual = zerolog.InfoLevel
	case "WARN":
		logLevelActual = zerolog.WarnLevel
	case "ERROR":
		logLevelActual = zerolog.ErrorLevel
	case "TRACE":
		logLevelActual = zerolog.TraceLevel
	default:
		logLevelActual = zerolog.InfoLevel
	}

	// set up logging
	zerolog.SetGlobalLevel(logLevelActual)
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}

	// set up multi-level writer
	mlw := zerolog.MultiLevelWriter(
		// write console format with colors to console
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
		// write console format without colors to file
		zerolog.ConsoleWriter{
			Out:        file,
			TimeFormat: time.RFC3339,
			NoColor:    true,
		},
	)

	m.Logger = zerolog.New(mlw).With().Timestamp().Logger().
		Hook(
			zerolog.HookFunc(
				func(e *zerolog.Event, level zerolog.Level, msg string) {
					// add current mission
					// add runtime info
					e.Bool("usingLocalDB", m.IsUsingLocalDB()).
						Str("currentMission", m.GetMissionName()).
						Uint("currentMissionID", m.GetMissionID()).
						Bool("statusMonitorActive", m.IsStatusRunning())
				}))

	m.TraceSample = m.Logger.With().
		Bool("sampled", true).Logger().Sample(&zerolog.BurstSampler{
		// allow max 5 entries per 10 seconds
		// once reached, sample 1 in 100
		Burst:       5,
		Period:      10 * time.Second,
		NextSampler: &zerolog.BasicSampler{N: 100},
	})

	m.Logger.Info().Str("loglevel", m.Logger.GetLevel().String()).Msg("Logging set up")
}

// SetupJSONLogger configures the JSON logger with the provided writer
func (m *Manager) SetupJSONLogger(writer *zerolog.Logger) {
	m.JSONLogger = *writer
}

// SetLogIoConn sets the log.io connection
func (m *Manager) SetLogIoConn(conn net.Conn) {
	m.logIoConn = conn
}

// SetGraylogWriter sets the Graylog GELF writer
func (m *Manager) SetGraylogWriter(writer *gelf.Writer) {
	m.graylogWriter = writer
}

// WriteToLogIo sends a log message to the log.io service
func (m *Manager) WriteToLogIo(level zerolog.Level, msg string) {
	if m.logIoConn == nil {
		return
	}
	data := make([]byte, 0)
	message := fmt.Sprintf(
		`%s %s %s`,
		time.Now().UTC().Format(time.RFC3339),
		level.String(),
		msg,
	)
	data = append(data, []byte(
		"+msg|ocap2|ocap_recorder|"+message,
	)...)
	data = append(data, m.nullByte)

	_, err := m.logIoConn.Write(data)
	if err != nil {
		m.Logger.Debug().Err(err).Msg("Failed to send log to log.io")
	}
}

// RemoveOldLogs removes .log and .jsonl files older than daysDelta days
func (m *Manager) RemoveOldLogs(path string, daysDelta int) {
	files, err := os.ReadDir(path)
	if err != nil {
		m.Logger.Warn().Err(err).Msg("Failed to read logs dir")
		return
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		// get file info
		r, err := f.Info()
		if err != nil {
			m.Logger.Warn().Err(err).Msg("Failed to get file info")
			continue
		}
		// check if file is a log file and if it's older than daysDelta days
		if filepath.Ext(f.Name()) == ".log" || filepath.Ext(f.Name()) == ".jsonl" {
			if time.Since(r.ModTime()).Hours() > float64(daysDelta*24) {
				os.Remove(filepath.Join(path, f.Name()))
			}
		}
	}
}

// WriteLog writes a log entry with the specified function name, data, and level
func (m *Manager) WriteLog(functionName string, data string, level string) {
	var logLevelActual zerolog.Level
	switch level {
	case "DEBUG":
		logLevelActual = zerolog.DebugLevel
	case "INFO":
		logLevelActual = zerolog.InfoLevel
	case "WARN":
		logLevelActual = zerolog.WarnLevel
	case "ERROR":
		logLevelActual = zerolog.ErrorLevel
	case "FATAL":
		logLevelActual = zerolog.FatalLevel
	}

	// debug limits configured in global defs based on config
	m.Logger.WithLevel(logLevelActual).
		Str("function", functionName).
		Msg(data)

	m.JSONLogger.WithLevel(logLevelActual).
		Str("function", functionName).
		Msg(data)
}

// Close cleans up logging resources
func (m *Manager) Close() error {
	if m.logIoConn != nil {
		return m.logIoConn.Close()
	}
	return nil
}
