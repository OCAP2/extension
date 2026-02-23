package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseVehicle parses vehicle data and returns a core Vehicle
func (p *Parser) ParseVehicle(data []string) (core.Vehicle, error) {
	var vehicle core.Vehicle

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return vehicle, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	vehicle.JoinTime = time.Now()

	vehicle.JoinFrame = core.Frame(capframe)
	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return vehicle, fmt.Errorf("error converting ocapID to uint: %w", err)
	}
	vehicle.ID = uint16(ocapID)
	vehicle.OcapType = data[2]
	vehicle.DisplayName = data[3]
	vehicle.ClassName = data[4]
	vehicle.Customization = data[5]
	vehicle.Side = data[6]

	return vehicle, nil
}

// ParseVehicleState parses vehicle state data and returns a core VehicleState.
// Sets VehicleID directly from the parsed ocapID (no cache lookup).
func (p *Parser) ParseVehicleState(data []string) (core.VehicleState, error) {
	var vehicleState core.VehicleState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[5], 64)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting capture frame to int: %w", err)
	}
	vehicleState.CaptureFrame = core.Frame(capframe)

	// parse ocapID and set directly (worker validates against cache)
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	vehicleState.VehicleID = uint16(ocapID)

	vehicleState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	pos3d, err := geo.Position3DFromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		p.logger.Error("Error converting position to Point", "data", string(jsonData), "error", err)
		return vehicleState, err
	}
	vehicleState.Position = pos3d

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	vehicleState.Bearing = uint16(bearing)
	// is alive
	isAlive, _ := strconv.ParseBool(data[3])
	vehicleState.IsAlive = isAlive
	// parse crew
	vehicleState.Crew = data[4]

	// fuel
	fuel, err := strconv.ParseFloat(data[6], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting fuel to float: %w", err)
	}
	vehicleState.Fuel = float32(fuel)

	// damage
	damage, err := strconv.ParseFloat(data[7], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting damage to float: %w", err)
	}
	vehicleState.Damage = float32(damage)

	// isEngineOn
	isEngineOn, err := strconv.ParseBool(data[8])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting isEngineOn to bool: %w", err)
	}
	vehicleState.EngineOn = isEngineOn

	// locked
	locked, err := strconv.ParseBool(data[9])
	if err != nil {
		return vehicleState, fmt.Errorf("error converting locked to bool: %w", err)
	}
	vehicleState.Locked = locked

	vehicleState.Side = data[10]
	vehicleState.VectorDir = data[11]
	vehicleState.VectorUp = data[12]

	turretAzimuth, err := strconv.ParseFloat(data[13], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting turretAzimuth to float: %w", err)
	}
	vehicleState.TurretAzimuth = float32(turretAzimuth)

	turretElevation, err := strconv.ParseFloat(data[14], 32)
	if err != nil {
		return vehicleState, fmt.Errorf("error converting turretElevation to float: %w", err)
	}
	vehicleState.TurretElevation = float32(turretElevation)

	return vehicleState, nil
}
