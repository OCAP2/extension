package logging

import "github.com/rs/zerolog"

// DispatcherLogger adapts zerolog.Logger to the dispatcher.Logger interface.
type DispatcherLogger struct {
	logger zerolog.Logger
}

// NewDispatcherLogger creates a new DispatcherLogger wrapping a zerolog.Logger.
func NewDispatcherLogger(logger zerolog.Logger) *DispatcherLogger {
	return &DispatcherLogger{logger: logger}
}

// Debug logs a debug message with optional key-value pairs.
func (l *DispatcherLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug().Fields(toFields(keysAndValues)).Msg(msg)
}

// Info logs an info message with optional key-value pairs.
func (l *DispatcherLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info().Fields(toFields(keysAndValues)).Msg(msg)
}

// Error logs an error message with optional key-value pairs.
func (l *DispatcherLogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error().Fields(toFields(keysAndValues)).Msg(msg)
}

// toFields converts key-value pairs to a map for zerolog.
func toFields(keysAndValues []any) map[string]any {
	fields := make(map[string]any, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		if key, ok := keysAndValues[i].(string); ok {
			fields[key] = keysAndValues[i+1]
		}
	}
	return fields
}
