package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDispatcherLogger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	dl := NewDispatcherLogger(logger)

	require.NotNil(t, dl)
}

func TestDispatcherLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dl := NewDispatcherLogger(logger)

	dl.Debug("test message", "key1", "value1", "key2", 42)

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry))

	assert.Equal(t, "DEBUG", logEntry["level"])
	assert.Equal(t, "test message", logEntry["msg"])
	assert.Equal(t, "value1", logEntry["key1"])
	assert.Equal(t, float64(42), logEntry["key2"])
}

func TestDispatcherLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	dl := NewDispatcherLogger(logger)

	dl.Info("info message", "status", "ok")

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry))

	assert.Equal(t, "INFO", logEntry["level"])
	assert.Equal(t, "info message", logEntry["msg"])
	assert.Equal(t, "ok", logEntry["status"])
}

func TestDispatcherLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	dl := NewDispatcherLogger(logger)

	dl.Error("error occurred", "code", 500, "reason", "internal")

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry))

	assert.Equal(t, "ERROR", logEntry["level"])
	assert.Equal(t, "error occurred", logEntry["msg"])
	assert.Equal(t, float64(500), logEntry["code"])
	assert.Equal(t, "internal", logEntry["reason"])
}

func TestDispatcherLogger_NoKeyValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dl := NewDispatcherLogger(logger)

	dl.Debug("simple message")

	var logEntry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &logEntry))

	assert.Equal(t, "simple message", logEntry["msg"])
}

func TestDispatcherLogger_ImplementsInterface(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	dl := NewDispatcherLogger(logger)

	var _ interface {
		Debug(msg string, keysAndValues ...any)
		Info(msg string, keysAndValues ...any)
		Error(msg string, keysAndValues ...any)
	} = dl
}
