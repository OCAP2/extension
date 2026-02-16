package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// SlogManager manages slog-based logging with optional OTel integration.
type SlogManager struct {
	logger *slog.Logger

	// OTel provider for flushing
	logProvider *sdklog.LoggerProvider
}

// NewSlogManager creates a new slog-based logging manager.
func NewSlogManager() *SlogManager {
	return &SlogManager{}
}

// parseLevel converts a string log level to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup initializes the logging system with file and optional OTel output.
// If provider is nil, OTel logging is disabled.
func (m *SlogManager) Setup(file io.Writer, level string, provider *sdklog.LoggerProvider) {
	lvl := parseLevel(level)
	m.logProvider = provider

	// Common handler options with RFC3339 time formatting
	handlerOpts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.UTC().Format(time.RFC3339))
				}
			}
			return a
		},
	}

	// Build list of handlers
	var handlers []slog.Handler

	// Console handler
	handlers = append(handlers, slog.NewTextHandler(os.Stdout, handlerOpts))

	// File handler
	if file != nil {
		handlers = append(handlers, slog.NewTextHandler(file, handlerOpts))
	}

	// OTel handler (if provider is available)
	if provider != nil {
		otelHandler := otelslog.NewHandler("ocap-recorder", otelslog.WithLoggerProvider(provider))
		handlers = append(handlers, otelHandler)
	}

	// Combine all handlers
	multiHandler := NewMultiHandler(handlers...)

	m.logger = slog.New(multiHandler)
	m.logger.Info("Logging initialized", "level", level)
}

// Logger returns the configured slog.Logger.
func (m *SlogManager) Logger() *slog.Logger {
	if m.logger == nil {
		// Return a default logger if Setup hasn't been called
		return slog.Default()
	}
	return m.logger
}

// Flush forces a flush of OTel logs if available.
func (m *SlogManager) Flush(ctx context.Context) error {
	if m.logProvider != nil {
		return m.logProvider.ForceFlush(ctx)
	}
	return nil
}

// WriteLog writes a log entry with the specified function name, data, and level.
// This provides backward compatibility with the old Manager interface.
func (m *SlogManager) WriteLog(functionName, data, level string) {
	if m.logger == nil {
		return
	}

	lvl := parseLevel(level)

	switch lvl {
	case slog.LevelDebug:
		m.logger.Debug(data, "function", functionName)
	case slog.LevelInfo:
		m.logger.Info(data, "function", functionName)
	case slog.LevelWarn:
		m.logger.Warn(data, "function", functionName)
	case slog.LevelError:
		m.logger.Error(data, "function", functionName)
	default:
		m.logger.Info(data, "function", functionName)
	}
}

