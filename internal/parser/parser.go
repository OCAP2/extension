package parser

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/OCAP2/extension/v5/internal/model"
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


// Service defines the interface for parsing ArmA 3 extension arguments into model structs.
type Service interface {
	ParseMission(args []string) (model.Mission, model.World, error)
	ParseSoldier(args []string) (model.Soldier, error)
	ParseVehicle(args []string) (model.Vehicle, error)
	ParseSoldierState(args []string) (model.SoldierState, error)
	ParseVehicleState(args []string) (model.VehicleState, error)
	ParseProjectileEvent(args []string) (ParsedProjectileEvent, error)
	ParseGeneralEvent(args []string) (model.GeneralEvent, error)
	ParseKillEvent(args []string) (ParsedKillEvent, error)
	ParseChatEvent(args []string) (model.ChatEvent, error)
	ParseRadioEvent(args []string) (model.RadioEvent, error)
	ParseFpsEvent(args []string) (model.ServerFpsEvent, error)
	ParseTimeState(args []string) (model.TimeState, error)
	ParseAce3DeathEvent(args []string) (model.Ace3DeathEvent, error)
	ParseAce3UnconsciousEvent(args []string) (model.Ace3UnconsciousEvent, error)
	ParseMarkerCreate(args []string) (model.Marker, error)
	ParseMarkerMove(args []string) (ParsedMarkerMove, error)
	ParseMarkerDelete(args []string) (string, uint, error)
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

