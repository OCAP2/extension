package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewDispatcherLogger(t *testing.T) {
	logger := zerolog.Nop()
	dl := NewDispatcherLogger(logger)

	if dl == nil {
		t.Fatal("expected non-nil DispatcherLogger")
	}
}

func TestDispatcherLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.DebugLevel)
	dl := NewDispatcherLogger(logger)

	dl.Debug("test message", "key1", "value1", "key2", 42)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "debug" {
		t.Errorf("expected level 'debug', got %v", logEntry["level"])
	}
	if logEntry["message"] != "test message" {
		t.Errorf("expected message 'test message', got %v", logEntry["message"])
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
	logger := zerolog.New(&buf).Level(zerolog.InfoLevel)
	dl := NewDispatcherLogger(logger)

	dl.Info("info message", "status", "ok")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "info" {
		t.Errorf("expected level 'info', got %v", logEntry["level"])
	}
	if logEntry["message"] != "info message" {
		t.Errorf("expected message 'info message', got %v", logEntry["message"])
	}
	if logEntry["status"] != "ok" {
		t.Errorf("expected status='ok', got %v", logEntry["status"])
	}
}

func TestDispatcherLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.ErrorLevel)
	dl := NewDispatcherLogger(logger)

	dl.Error("error occurred", "code", 500, "reason", "internal")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["level"] != "error" {
		t.Errorf("expected level 'error', got %v", logEntry["level"])
	}
	if logEntry["message"] != "error occurred" {
		t.Errorf("expected message 'error occurred', got %v", logEntry["message"])
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
	logger := zerolog.New(&buf).Level(zerolog.DebugLevel)
	dl := NewDispatcherLogger(logger)

	dl.Debug("simple message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if logEntry["message"] != "simple message" {
		t.Errorf("expected message 'simple message', got %v", logEntry["message"])
	}
}

func TestToFields(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		expected map[string]any
	}{
		{
			name:     "empty input",
			input:    []any{},
			expected: map[string]any{},
		},
		{
			name:     "single pair",
			input:    []any{"key", "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "multiple pairs",
			input:    []any{"k1", "v1", "k2", 42, "k3", true},
			expected: map[string]any{"k1": "v1", "k2": 42, "k3": true},
		},
		{
			name:     "odd number of elements (trailing ignored)",
			input:    []any{"k1", "v1", "k2"},
			expected: map[string]any{"k1": "v1"},
		},
		{
			name:     "non-string key (ignored)",
			input:    []any{123, "value", "validKey", "validValue"},
			expected: map[string]any{"validKey": "validValue"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := toFields(tc.input)

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d fields, got %d", len(tc.expected), len(result))
			}

			for k, v := range tc.expected {
				if result[k] != v {
					t.Errorf("expected %s=%v, got %v", k, v, result[k])
				}
			}
		})
	}
}

func TestDispatcherLogger_ImplementsInterface(t *testing.T) {
	// Verify DispatcherLogger satisfies the dispatcher.Logger interface
	// by ensuring the methods exist with correct signatures
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	dl := NewDispatcherLogger(logger)

	// These calls would fail to compile if the interface isn't satisfied
	var _ interface {
		Debug(msg string, keysAndValues ...any)
		Info(msg string, keysAndValues ...any)
		Error(msg string, keysAndValues ...any)
	} = dl
}
