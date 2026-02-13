package logging

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogFilePath(t *testing.T) {
	sessionStart := time.Date(2026, 2, 12, 21, 38, 36, 0, time.UTC)

	tests := []struct {
		name          string
		logsDir       string
		extensionName string
		want          string
	}{
		{
			name:          "basic path",
			logsDir:       "ocaplogs",
			extensionName: "ocap_recorder",
			want:          filepath.Join("ocaplogs", "ocap_recorder.20260212_213836.log"),
		},
		{
			name:          "relative path with dot",
			logsDir:       "./ocaplogs",
			extensionName: "ocap_recorder",
			want:          filepath.Join(".", "ocaplogs", "ocap_recorder.20260212_213836.log"),
		},
		{
			name:          "absolute path",
			logsDir:       filepath.Join("/var", "log", "ocap"),
			extensionName: "ocap_recorder",
			want:          filepath.Join("/var", "log", "ocap", "ocap_recorder.20260212_213836.log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LogFilePath(tt.logsDir, tt.extensionName, sessionStart)
			assert.Equal(t, tt.want, got)
		})
	}
}
