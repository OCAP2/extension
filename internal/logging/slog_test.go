package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestSetup_FileOnly_NoStdout(t *testing.T) {
	// Capture stdout to verify nothing is written there
	origStdout := captureStdout(t)

	var fileBuf bytes.Buffer
	m := NewSlogManager()
	m.Setup(&fileBuf, "info", nil)
	m.Logger().Info("hello file")

	stdout := origStdout()

	assert.Contains(t, fileBuf.String(), "hello file", "log should appear in file")
	// The "Logging initialized" message from Setup also goes to file, not stdout
	assert.Empty(t, stdout, "nothing should be written to stdout when file is provided")
}

func TestSetup_NoFile_WritesToStdout(t *testing.T) {
	origStdout := captureStdout(t)

	m := NewSlogManager()
	m.Setup(nil, "info", nil)
	m.Logger().Info("hello console")

	stdout := origStdout()

	assert.Contains(t, stdout, "hello console", "log should appear on stdout")
}

func TestSetup_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	m := NewSlogManager()
	m.Setup(&buf, "debug", nil)

	m.Logger().Debug("debug msg")
	m.Logger().Info("info msg")

	output := buf.String()
	assert.Contains(t, output, "debug msg")
	assert.Contains(t, output, "info msg")
}

func TestSetup_InfoLevel_FiltersDebug(t *testing.T) {
	var buf bytes.Buffer
	m := NewSlogManager()
	m.Setup(&buf, "info", nil)

	m.Logger().Debug("should be filtered")
	m.Logger().Info("should appear")

	output := buf.String()
	assert.NotContains(t, output, "should be filtered")
	assert.Contains(t, output, "should appear")
}

func TestSetup_ReplacesLogger(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	m := NewSlogManager()

	m.Setup(&buf1, "info", nil)
	m.Logger().Info("first")

	m.Setup(&buf2, "info", nil)
	m.Logger().Info("second")

	assert.Contains(t, buf1.String(), "first")
	assert.NotContains(t, buf1.String(), "second", "old file should not receive new logs")
	assert.Contains(t, buf2.String(), "second")
}

func TestLogger_DefaultBeforeSetup(t *testing.T) {
	m := NewSlogManager()
	logger := m.Logger()
	assert.Equal(t, slog.Default(), logger)
}

func TestFlush_NilProvider(t *testing.T) {
	m := NewSlogManager()
	err := m.Flush(context.Background())
	assert.NoError(t, err)
}

func TestWriteLog_AllLevels(t *testing.T) {
	levels := []struct {
		level    string
		contains string
	}{
		{"debug", "debug message"},
		{"info", "info message"},
		{"warn", "warn message"},
		{"error", "error message"},
		{"unknown", "unknown message"}, // defaults to info
	}

	for _, tt := range levels {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			m := NewSlogManager()
			m.Setup(&buf, "debug", nil)

			m.WriteLog("testFunc", tt.level+" message", tt.level)

			output := buf.String()
			assert.Contains(t, output, tt.contains)
			assert.Contains(t, output, "testFunc")
		})
	}
}

func TestWriteLog_NilLogger(t *testing.T) {
	m := NewSlogManager()
	// Should not panic
	m.WriteLog("fn", "data", "info")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"invalid", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLevel(tt.input))
		})
	}
}

func TestMultiHandler_FansOut(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h1 := slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	multi := NewMultiHandler(h1, h2)
	logger := slog.New(multi)
	logger.Info("fanned out")

	assert.Contains(t, buf1.String(), "fanned out")
	assert.Contains(t, buf2.String(), "fanned out")
}

func TestMultiHandler_FiltersNilHandlers(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, nil)

	multi := NewMultiHandler(nil, h, nil)
	require.Len(t, multi.handlers, 1)

	logger := slog.New(multi)
	logger.Info("works")
	assert.Contains(t, buf.String(), "works")
}

func TestMultiHandler_Enabled(t *testing.T) {
	infoHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	debugHandler := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})

	// Multi with only info handler: debug should be disabled
	infoOnly := NewMultiHandler(infoHandler)
	assert.False(t, infoOnly.Enabled(context.Background(), slog.LevelDebug))
	assert.True(t, infoOnly.Enabled(context.Background(), slog.LevelInfo))

	// Multi with both: debug should be enabled (any handler enables it)
	both := NewMultiHandler(infoHandler, debugHandler)
	assert.True(t, both.Enabled(context.Background(), slog.LevelDebug))
}

func TestMultiHandler_Empty(t *testing.T) {
	multi := NewMultiHandler()
	assert.False(t, multi.Enabled(context.Background(), slog.LevelInfo))
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	multi := NewMultiHandler(h)

	withAttrs := multi.WithAttrs([]slog.Attr{slog.String("component", "test")})
	logger := slog.New(withAttrs)
	logger.Info("with attrs")

	assert.Contains(t, buf.String(), "component=test")
}

func TestMultiHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	multi := NewMultiHandler(h)

	withGroup := multi.WithGroup("grp")
	logger := slog.New(withGroup)
	logger.Info("grouped", "key", "val")

	assert.Contains(t, buf.String(), "grp.key=val")
}

func TestMultiHandler_WithGroupEmpty(t *testing.T) {
	h := slog.NewTextHandler(&bytes.Buffer{}, nil)
	multi := NewMultiHandler(h)

	same := multi.WithGroup("")
	assert.Equal(t, multi, same, "empty group name should return same handler")
}

func TestFlush_WithProvider(t *testing.T) {
	provider := sdklog.NewLoggerProvider() // no exporter, just validates non-nil path
	m := NewSlogManager()

	var buf bytes.Buffer
	m.Setup(&buf, "info", provider)

	err := m.Flush(context.Background())
	assert.NoError(t, err)
}

// errorHandler is a slog.Handler that always returns an error from Handle.
type errorHandler struct {
	slog.Handler
}

func (h *errorHandler) Handle(_ context.Context, _ slog.Record) error {
	return errors.New("handler error")
}

func (h *errorHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func TestMultiHandler_HandleError(t *testing.T) {
	var buf bytes.Buffer
	spy := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})

	// First handler errors, second (spy) should still receive the record.
	multi := NewMultiHandler(&errorHandler{}, spy)
	logger := slog.New(multi)
	logger.Info("should reach spy")

	assert.Contains(t, buf.String(), "should reach spy")
}

func TestSetup_WithOTelProvider(t *testing.T) {
	provider := sdklog.NewLoggerProvider()

	var buf bytes.Buffer
	m := NewSlogManager()
	m.Setup(&buf, "info", provider)

	m.Logger().Info("otel integrated")
	assert.Contains(t, buf.String(), "otel integrated")
}

// captureStdout redirects os.Stdout to a pipe and returns a function
// that restores stdout and returns what was captured.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	r, w, err := osPipe()
	require.NoError(t, err)

	origStdout := osStdout
	osStdout = w

	return func() string {
		w.Close()
		osStdout = origStdout
		var buf bytes.Buffer
		buf.ReadFrom(r)
		r.Close()
		return buf.String()
	}
}
