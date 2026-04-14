package parser

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/OCAP2/extension/v5/pkg/core"
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


// parseRadioChannel parses a radio channel value that may be either a plain
// number ("1") or an SQF array ("[0,11]"). ACRE multi-knob radios like the
// AN/PRC-77 send the channel as [knobA, knobB]; we take the last element as
// the channel number since it typically represents the selected channel.
func parseRadioChannel(s string) (int64, error) {
	s = strings.TrimSpace(s)

	// Fast path: plain scalar
	if v, err := parseIntFromFloat(s); err == nil {
		return v, nil
	}

	// Slow path: SQF array "[0,11]"
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := s[1 : len(s)-1]
		parts := strings.Split(inner, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if v, err := parseIntFromFloat(last); err == nil {
			return v, nil
		}
	}

	return 0, fmt.Errorf("parseRadioChannel: %q is not a valid channel (expected int or [int,...,int])", s)
}

// ChatChannels maps ArmA 3 channel numbers to human-readable names.
var ChatChannels = map[int]string{
	0:  "Global",
	1:  "Side",
	2:  "Command",
	3:  "Group",
	4:  "Vehicle",
	5:  "Direct",
	16: "System",
}

// Service defines the interface for parsing ArmA 3 extension arguments into model structs.
type Service interface {
	ParseMission(args []string) (core.Mission, core.World, error)
	ParseSoldier(args []string) (core.Soldier, error)
	ParseVehicle(args []string) (core.Vehicle, error)
	ParseSoldierState(args []string) (core.SoldierState, error)
	ParseVehicleState(args []string) (core.VehicleState, error)
	ParseProjectileEvent(args []string) (ProjectileEvent, error)
	ParseGeneralEvent(args []string) (core.GeneralEvent, error)
	ParseSectorEvent(args []string) (core.SectorEvent, error)
	ParseEndMissionEvent(args []string) (core.EndMissionEvent, error)
	ParseKillEvent(args []string) (KillEvent, error)
	ParseChatEvent(args []string) (core.ChatEvent, error)
	ParseRadioEvent(args []string) (core.RadioEvent, error)
	ParseTimeState(args []string) (core.TimeState, error)
	ParseAce3DeathEvent(args []string) (core.Ace3DeathEvent, error)
	ParseAce3UnconsciousEvent(args []string) (core.Ace3UnconsciousEvent, error)
	ParseMarkerCreate(args []string) (core.Marker, error)
	ParseMarkerMove(args []string) (MarkerMove, error)
	ParseMarkerDelete(args []string) (*core.DeleteMarker, error)
	ParseSoldierDelete(args []string) (uint16, core.Frame, error)
	ParseVehicleDelete(args []string) (uint16, core.Frame, error)
	ParseTelemetryEvent(args []string) (core.TelemetryEvent, error)
	ParsePlacedObject(args []string) (core.PlacedObject, error)
	ParsePlacedObjectEvent(args []string) (core.PlacedObjectEvent, error)
	ParseFocusStart(args []string) (core.Frame, error)
	ParseFocusEnd(args []string) (core.Frame, error)
}

var _ Service = (*Parser)(nil)

// Parser provides pure []string -> model struct conversion.
// It has zero external dependencies beyond a logger.
type Parser struct {
	logger *slog.Logger
}

// NewParser creates a new parser with only a logger dependency
func NewParser(logger *slog.Logger) *Parser {
	return &Parser{logger: logger}
}
