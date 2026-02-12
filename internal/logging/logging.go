package logging

import (
	"fmt"
	"path/filepath"
	"time"
)

// LogFilePath builds a log file path using OS-appropriate path separators.
func LogFilePath(logsDir, extensionName string, sessionStart time.Time) string {
	return filepath.Join(
		logsDir,
		fmt.Sprintf("%s.%s.log", extensionName, sessionStart.Format("20060102_150405")),
	)
}
