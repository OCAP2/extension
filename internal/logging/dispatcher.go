package logging

import "log/slog"

// DispatcherLogger adapts *slog.Logger to the dispatcher.Logger interface.
type DispatcherLogger struct {
	logger *slog.Logger
}

// NewDispatcherLogger creates a new DispatcherLogger wrapping a *slog.Logger.
func NewDispatcherLogger(logger *slog.Logger) *DispatcherLogger {
	return &DispatcherLogger{logger: logger}
}

// Debug logs a debug message with optional key-value pairs.
func (l *DispatcherLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(msg, keysAndValues...)
}

// Info logs an info message with optional key-value pairs.
func (l *DispatcherLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

// Error logs an error message with optional key-value pairs.
func (l *DispatcherLogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(msg, keysAndValues...)
}
