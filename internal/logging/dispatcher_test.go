package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewDispatcherLogger(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	dl := NewDispatcherLogger(logger)

	if dl == nil {
		t.Fatal("expected non-nil DispatcherLogger")
	}
}

func TestDispatcherLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dl := NewDispatcherLogger(logger)

	dl.Debug("test message", "key1", "value1", "key2", 42)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "DEBUG" {
		t.Errorf("expected level 'DEBUG', got %v", logEntry["level"])
	}
	if logEntry["msg"] != "test message" {
		t.Errorf("expected msg 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", logEntry["key1"])
	}
	if logEntry["key2"] != float64(42) { // JSON numbers are float64
		t.Errorf("expected key2=42, got %v", logEntry["key2"])
	}
}

func TestDispatcherLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	dl := NewDispatcherLogger(logger)

	dl.Info("info message", "status", "ok")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "INFO" {
		t.Errorf("expected level 'INFO', got %v", logEntry["level"])
	}
	if logEntry["msg"] != "info message" {
		t.Errorf("expected msg 'info message', got %v", logEntry["msg"])
	}
	if logEntry["status"] != "ok" {
		t.Errorf("expected status='ok', got %v", logEntry["status"])
	}
}

func TestDispatcherLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	dl := NewDispatcherLogger(logger)

	dl.Error("error occurred", "code", 500, "reason", "internal")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "ERROR" {
		t.Errorf("expected level 'ERROR', got %v", logEntry["level"])
	}
	if logEntry["msg"] != "error occurred" {
		t.Errorf("expected msg 'error occurred', got %v", logEntry["msg"])
	}
	if logEntry["code"] != float64(500) {
		t.Errorf("expected code=500, got %v", logEntry["code"])
	}
	if logEntry["reason"] != "internal" {
		t.Errorf("expected reason='internal', got %v", logEntry["reason"])
	}
}

func TestDispatcherLogger_NoKeyValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	dl := NewDispatcherLogger(logger)

	dl.Debug("simple message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["msg"] != "simple message" {
		t.Errorf("expected msg 'simple message', got %v", logEntry["msg"])
	}
}

func TestDispatcherLogger_ImplementsInterface(t *testing.T) {
	// Verify DispatcherLogger satisfies the dispatcher.Logger interface
	// by ensuring the methods exist with correct signatures
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	dl := NewDispatcherLogger(logger)

	// These calls would fail to compile if the interface isn't satisfied
	var _ interface {
		Debug(msg string, keysAndValues ...any)
		Info(msg string, keysAndValues ...any)
		Error(msg string, keysAndValues ...any)
	} = dl
}
