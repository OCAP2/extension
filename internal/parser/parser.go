package parser

import (
	"fmt"
	"log/slog"
	"strconv"
)

// parseUintFromFloat parses a string that may be an integer ("32") or float ("32.00") into uint64.
// ArmA 3's SQF has no integer type, so the extension API may serialize numbers as floats.
func parseUintFromFloat(s string) (uint64, error) {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f < 0 || f != float64(uint64(f)) {
		return 0, fmt.Errorf("parseUintFromFloat: %q is not a valid uint64", s)
	}
	return uint64(f), nil
}

// parseIntFromFloat parses a string that may be an integer or float into int64.
func parseIntFromFloat(s string) (int64, error) {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f != float64(int64(f)) {
		return 0, fmt.Errorf("parseIntFromFloat: %q is not a valid int64", s)
	}
	return int64(f), nil
}


// Parser provides pure []string -> model struct conversion.
// It has zero external dependencies beyond a logger.
type Parser struct {
	logger *slog.Logger

	// Static config set at creation time
	addonVersion     string
	extensionVersion string
}

// NewParser creates a new parser with only a logger dependency
func NewParser(logger *slog.Logger, addonVersion, extensionVersion string) *Parser {
	p := &Parser{
		logger:           logger,
		addonVersion:     addonVersion,
		extensionVersion: extensionVersion,
	}
	return p
}

