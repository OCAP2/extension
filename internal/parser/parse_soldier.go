package parser

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseSoldier parses soldier data and returns a Soldier model
func (p *Parser) ParseSoldier(data []string) (model.Soldier, error) {
	var soldier model.Soldier

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return soldier, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	soldier.MissionID = p.getMissionID()
	soldier.JoinFrame = uint(capframe)
	soldier.JoinTime = time.Now()

	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return soldier, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	soldier.ObjectID = uint16(ocapID)
	soldier.UnitName = data[2]
	soldier.GroupID = data[3]
	soldier.Side = data[4]
	soldier.IsPlayer, err = strconv.ParseBool(data[5])
	if err != nil {
		return soldier, fmt.Errorf("error converting isPlayer to bool: %w", err)
	}
	soldier.RoleDescription = data[6]
	soldier.ClassName = data[7]
	soldier.DisplayName = data[8]
	soldier.PlayerUID = data[9]

	// marshal squadparams
	err = json.Unmarshal([]byte(data[10]), &soldier.SquadParams)
	if err != nil {
		return soldier, fmt.Errorf("error unmarshalling squadParams: %w", err)
	}

	return soldier, nil
}

// ParseSoldierState parses soldier state data and returns a SoldierState model.
// Sets SoldierObjectID directly from the parsed ocapID (no cache lookup).
// If groupID/side fields are not in the data (len < 17), they are left empty
// for the worker layer to fill from cache.
func (p *Parser) ParseSoldierState(data []string) (model.SoldierState, error) {
	var soldierState model.SoldierState

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	soldierState.MissionID = p.getMissionID()

	frameStr := data[8]
	capframe, err := strconv.ParseFloat(frameStr, 64)
	if err != nil {
		return soldierState, fmt.Errorf("error converting capture frame to int: %w", err)
	}
	soldierState.CaptureFrame = uint(capframe)

	// parse ocapID and set directly (worker validates against cache)
	ocapID, err := parseUintFromFloat(data[0])
	if err != nil {
		return soldierState, fmt.Errorf("error converting ocapId to uint: %w", err)
	}
	soldierState.SoldierObjectID = uint16(ocapID)

	soldierState.Time = time.Now()

	// parse pos from an arma string
	pos := data[1]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	point, elev, err := geo.Coord3857FromString(pos)
	if err != nil {
		jsonData, _ := json.Marshal(data)
		p.logger.Error("Error converting position to Point", "data", string(jsonData), "error", err)
		return soldierState, err
	}
	soldierState.Position = point
	soldierState.ElevationASL = float32(elev)

	// bearing
	bearing, _ := strconv.Atoi(data[2])
	soldierState.Bearing = uint16(bearing)
	// lifestate
	lifeState, _ := strconv.Atoi(data[3])
	soldierState.Lifestate = uint8(lifeState)
	// in vehicle
	soldierState.InVehicle, _ = strconv.ParseBool(data[4])
	// name
	soldierState.UnitName = data[5]
	// is player
	isPlayer, err := strconv.ParseBool(data[6])
	if err != nil {
		return soldierState, fmt.Errorf("error converting isPlayer to bool: %w", err)
	}
	soldierState.IsPlayer = isPlayer
	// current role
	soldierState.CurrentRole = data[7]

	// parse ace3/medical data
	hasStableVitals, _ := strconv.ParseBool(data[9])
	soldierState.HasStableVitals = hasStableVitals
	isDraggedCarried, _ := strconv.ParseBool(data[10])
	soldierState.IsDraggedCarried = isDraggedCarried

	// player scores come in as an array
	if isPlayer {
		scoresStr := data[11]
		scoresArr := strings.Split(scoresStr, ",")
		if len(scoresArr) >= 6 {
			scoresInt := make([]uint8, len(scoresArr))
			for i, v := range scoresArr {
				num, _ := strconv.Atoi(v)
				scoresInt[i] = uint8(num)
			}

			soldierState.Scores = model.SoldierScores{
				InfantryKills: scoresInt[0],
				VehicleKills:  scoresInt[1],
				ArmorKills:    scoresInt[2],
				AirKills:      scoresInt[3],
				Deaths:        scoresInt[4],
				TotalScore:    scoresInt[5],
			}
		} else {
			p.logger.Warn("Unexpected score count", "expected", 6, "got", len(scoresArr), "data", scoresStr)
		}
	}

	// seat in vehicle
	soldierState.VehicleRole = data[12]

	// InVehicleObjectID, if -1 not in a vehicle
	inVehicleID, _ := strconv.Atoi(data[13])
	if inVehicleID == -1 {
		soldierState.InVehicleObjectID = sql.NullInt32{Int32: 0, Valid: false}
	} else {
		soldierState.InVehicleObjectID = sql.NullInt32{Int32: int32(inVehicleID), Valid: true}
	}

	// stance
	soldierState.Stance = data[14]

	// groupID and side (added by addon PR#55, backward compatible)
	// If not present, leave empty - worker fills from cached soldier
	if len(data) >= 17 {
		soldierState.GroupID = data[15]
		soldierState.Side = data[16]
	}

	return soldierState, nil
}
