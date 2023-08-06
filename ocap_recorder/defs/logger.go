package defs

import (
	"github.com/rs/zerolog"
)

var (
	// Logger writes to console and file in line format
	Logger zerolog.Logger
	// JSONLogger is hooked from Logger and writes to JSONL file
	JSONLogger zerolog.Logger
)

// InitLogger initializes the logger
func InitLogger() {
	// Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
