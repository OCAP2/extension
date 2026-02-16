package convert

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/pkg/core"
	geom "github.com/peterstace/simplefeatures/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestPosition3DToPoint(t *testing.T) {
	pos := core.Position3D{X: 100.5, Y: 200.5, Z: 50.0}
	pt := position3DToPoint(pos)

	coord, ok := pt.Coordinates()
	require.True(t, ok)
	assert.Equal(t, 100.5, coord.XY.X)
	assert.Equal(t, 200.5, coord.XY.Y)
	assert.Equal(t, 50.0, coord.Z)
}

func TestPolylineToLineString(t *testing.T) {
	poly := core.Polyline{
		{X: 100.0, Y: 200.0},
		{X: 300.0, Y: 400.0},
		{X: 500.0, Y: 600.0},
	}
	ls := polylineToLineString(poly)

	seq := ls.Coordinates()
	require.Equal(t, 3, seq.Length())
	assert.Equal(t, 100.0, seq.GetXY(0).X)
	assert.Equal(t, 200.0, seq.GetXY(0).Y)
	assert.Equal(t, 500.0, seq.GetXY(2).X)
	assert.Equal(t, 600.0, seq.GetXY(2).Y)
}

func TestPolylineToLineString_Empty(t *testing.T) {
	ls := polylineToLineString(nil)
	assert.True(t, ls.IsEmpty())
}

// Round-trip: GORM → Core → GORM
func TestSoldierRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	squadParams, _ := json.Marshal([]any{"test", "params"})

	original := model.Soldier{
		ObjectID:        42,
		JoinTime:        now,
		JoinFrame:       10,
		OcapType:        "man",
		UnitName:        "TestUnit",
		GroupID:         "Alpha",
		Side:            "WEST",
		IsPlayer:        true,
		RoleDescription: "Rifleman",
		ClassName:       "B_Soldier_F",
		DisplayName:     "John Doe",
		PlayerUID:       "12345678",
		SquadParams:     datatypes.JSON(squadParams),
	}

	coreObj := SoldierToCore(original)
	roundTripped := CoreToSoldier(coreObj)

	assert.Equal(t, original.ObjectID, roundTripped.ObjectID)
	assert.Equal(t, original.JoinTime, roundTripped.JoinTime)
	assert.Equal(t, original.JoinFrame, roundTripped.JoinFrame)
	assert.Equal(t, original.OcapType, roundTripped.OcapType)
	assert.Equal(t, original.UnitName, roundTripped.UnitName)
	assert.Equal(t, original.GroupID, roundTripped.GroupID)
	assert.Equal(t, original.Side, roundTripped.Side)
	assert.Equal(t, original.IsPlayer, roundTripped.IsPlayer)
	assert.Equal(t, original.RoleDescription, roundTripped.RoleDescription)
	assert.Equal(t, original.ClassName, roundTripped.ClassName)
	assert.Equal(t, original.DisplayName, roundTripped.DisplayName)
	assert.Equal(t, original.PlayerUID, roundTripped.PlayerUID)
	// SquadParams: compare unmarshalled values (JSON serialization may differ in whitespace)
	var origParams, rtParams []any
	json.Unmarshal(original.SquadParams, &origParams)
	json.Unmarshal(roundTripped.SquadParams, &rtParams)
	assert.Equal(t, origParams, rtParams)
}

func TestVehicleRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.Vehicle{
		ObjectID:      10,
		JoinTime:      now,
		JoinFrame:     20,
		OcapType:      "car",
		ClassName:     "B_MRAP_01_F",
		DisplayName:   "Hunter",
		Customization: "default",
	}

	coreObj := VehicleToCore(original)
	roundTripped := CoreToVehicle(coreObj)

	assert.Equal(t, original.ObjectID, roundTripped.ObjectID)
	assert.Equal(t, original.JoinTime, roundTripped.JoinTime)
	assert.Equal(t, original.JoinFrame, roundTripped.JoinFrame)
	assert.Equal(t, original.OcapType, roundTripped.OcapType)
	assert.Equal(t, original.ClassName, roundTripped.ClassName)
	assert.Equal(t, original.DisplayName, roundTripped.DisplayName)
	assert.Equal(t, original.Customization, roundTripped.Customization)
}

func TestSoldierStateRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.SoldierState{
		SoldierObjectID:  2,
		Time:             now,
		CaptureFrame:     100,
		Position:         makePoint(1000.0, 2000.0, 10.0),
		Bearing:          90,
		Lifestate:        1,
		InVehicle:        true,
		InVehicleObjectID: sql.NullInt32{Int32: 5, Valid: true},
		VehicleRole:      "driver",
		UnitName:         "TestUnit",
		IsPlayer:         true,
		CurrentRole:      "Rifleman",
		HasStableVitals:  true,
		IsDraggedCarried: false,
		Stance:           "Up",
		GroupID:          "Alpha 1-1",
		Side:             "WEST",
		Scores: model.SoldierScores{
			InfantryKills: 5,
			VehicleKills:  2,
			ArmorKills:    1,
			AirKills:      0,
			Deaths:        1,
			TotalScore:    25,
		},
	}

	coreObj := SoldierStateToCore(original)
	roundTripped := CoreToSoldierState(coreObj)

	assert.Equal(t, original.SoldierObjectID, roundTripped.SoldierObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Bearing, roundTripped.Bearing)
	assert.Equal(t, original.Lifestate, roundTripped.Lifestate)
	assert.Equal(t, original.InVehicle, roundTripped.InVehicle)
	assert.Equal(t, original.InVehicleObjectID, roundTripped.InVehicleObjectID)
	assert.Equal(t, original.VehicleRole, roundTripped.VehicleRole)
	assert.Equal(t, original.UnitName, roundTripped.UnitName)
	assert.Equal(t, original.IsPlayer, roundTripped.IsPlayer)
	assert.Equal(t, original.CurrentRole, roundTripped.CurrentRole)
	assert.Equal(t, original.HasStableVitals, roundTripped.HasStableVitals)
	assert.Equal(t, original.IsDraggedCarried, roundTripped.IsDraggedCarried)
	assert.Equal(t, original.Stance, roundTripped.Stance)
	assert.Equal(t, original.GroupID, roundTripped.GroupID)
	assert.Equal(t, original.Side, roundTripped.Side)
	assert.Equal(t, original.Scores, roundTripped.Scores)

	// Verify position round-trips through Point
	origCoord, _ := original.Position.Coordinates()
	rtCoord, _ := roundTripped.Position.Coordinates()
	assert.Equal(t, origCoord.XY.X, rtCoord.XY.X)
	assert.Equal(t, origCoord.XY.Y, rtCoord.XY.Y)
	assert.Equal(t, origCoord.Z, rtCoord.Z)
}

func TestSoldierStateRoundTrip_NilInVehicleID(t *testing.T) {
	original := model.SoldierState{
		InVehicleObjectID: sql.NullInt32{Valid: false},
	}

	coreObj := SoldierStateToCore(original)
	roundTripped := CoreToSoldierState(coreObj)

	assert.False(t, roundTripped.InVehicleObjectID.Valid)
}

func TestVehicleStateRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.VehicleState{
		VehicleObjectID: 5,
		Time:            now,
		CaptureFrame:    50,
		Position:        makePoint(500.0, 600.0, 0.0),
		Bearing:         180,
		IsAlive:         true,
		Crew:            "1,2,3",
		Fuel:            0.8,
		Damage:          0.1,
		Locked:          false,
		EngineOn:        true,
		Side:            "WEST",
		VectorDir:       "[0,1,0]",
		VectorUp:        "[0,0,1]",
		TurretAzimuth:   45.0,
		TurretElevation: -10.0,
	}

	coreObj := VehicleStateToCore(original)
	roundTripped := CoreToVehicleState(coreObj)

	assert.Equal(t, original.VehicleObjectID, roundTripped.VehicleObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Bearing, roundTripped.Bearing)
	assert.Equal(t, original.IsAlive, roundTripped.IsAlive)
	assert.Equal(t, original.Crew, roundTripped.Crew)
	assert.Equal(t, original.Fuel, roundTripped.Fuel)
	assert.Equal(t, original.Damage, roundTripped.Damage)
	assert.Equal(t, original.Locked, roundTripped.Locked)
	assert.Equal(t, original.EngineOn, roundTripped.EngineOn)
	assert.Equal(t, original.Side, roundTripped.Side)
	assert.Equal(t, original.VectorDir, roundTripped.VectorDir)
	assert.Equal(t, original.VectorUp, roundTripped.VectorUp)
	assert.Equal(t, original.TurretAzimuth, roundTripped.TurretAzimuth)
	assert.Equal(t, original.TurretElevation, roundTripped.TurretElevation)
}

func TestMarkerRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.Marker{
		ID:           1,
		Time:         now,
		CaptureFrame: 50,
		MarkerName:   "TestMarker",
		Direction:    45.0,
		MarkerType:   "mil_dot",
		Text:         "Test",
		OwnerID:      2,
		Color:        "ColorRed",
		Size:         "[1,1]",
		Side:         "WEST",
		Position:     makePoint(100.0, 200.0, 0.0),
		Shape:        "ICON",
		Alpha:        1.0,
		Brush:        "Solid",
		IsDeleted:    false,
	}

	coreObj := MarkerToCore(original)
	roundTripped := CoreToMarker(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.MarkerName, roundTripped.MarkerName)
	assert.Equal(t, original.Direction, roundTripped.Direction)
	assert.Equal(t, original.MarkerType, roundTripped.MarkerType)
	assert.Equal(t, original.Text, roundTripped.Text)
	assert.Equal(t, original.OwnerID, roundTripped.OwnerID)
	assert.Equal(t, original.Color, roundTripped.Color)
	assert.Equal(t, original.Size, roundTripped.Size)
	assert.Equal(t, original.Side, roundTripped.Side)
	assert.Equal(t, original.Shape, roundTripped.Shape)
	assert.Equal(t, original.Alpha, roundTripped.Alpha)
	assert.Equal(t, original.Brush, roundTripped.Brush)
	assert.Equal(t, original.IsDeleted, roundTripped.IsDeleted)
}

func TestMarkerStateRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.MarkerState{
		ID:           1,
		MarkerID:     5,
		Time:         now,
		CaptureFrame: 100,
		Position:     makePoint(150.0, 250.0, 0.0),
		Direction:    90.0,
		Alpha:        0.5,
	}

	coreObj := MarkerStateToCore(original)
	roundTripped := CoreToMarkerState(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.MarkerID, roundTripped.MarkerID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Direction, roundTripped.Direction)
	assert.Equal(t, original.Alpha, roundTripped.Alpha)
}

func TestGeneralEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	extraData, _ := json.Marshal(map[string]any{"key": "value"})

	original := model.GeneralEvent{
		ID:           1,
		Time:         now,
		CaptureFrame: 100,
		Name:         "TestEvent",
		Message:      "Test message",
		ExtraData:    datatypes.JSON(extraData),
	}

	coreObj := GeneralEventToCore(original)
	roundTripped := CoreToGeneralEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Name, roundTripped.Name)
	assert.Equal(t, original.Message, roundTripped.Message)
	// Compare unmarshalled ExtraData
	var origExtra, rtExtra map[string]any
	json.Unmarshal(original.ExtraData, &origExtra)
	json.Unmarshal(roundTripped.ExtraData, &rtExtra)
	assert.Equal(t, origExtra, rtExtra)
}

func TestKillEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.KillEvent{
		ID:                    1,
		Time:                  now,
		CaptureFrame:          100,
		VictimSoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		KillerVehicleObjectID: sql.NullInt32{Int32: 30, Valid: true},
		EventText:             "Kill event",
		Distance:              100.0,
	}

	coreObj := KillEventToCore(original)
	roundTripped := CoreToKillEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.VictimSoldierObjectID, roundTripped.VictimSoldierObjectID)
	assert.Equal(t, original.KillerVehicleObjectID, roundTripped.KillerVehicleObjectID)
	assert.False(t, roundTripped.VictimVehicleObjectID.Valid)
	assert.False(t, roundTripped.KillerSoldierObjectID.Valid)
	assert.Equal(t, original.EventText, roundTripped.EventText)
	assert.Equal(t, original.Distance, roundTripped.Distance)
}

func TestChatEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.ChatEvent{
		ID:              1,
		SoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		Time:            now,
		CaptureFrame:    100,
		Channel:         "Global",
		FromName:        "Player1",
		SenderName:      "John",
		Message:         "Hello world",
		PlayerUID:       "12345678",
	}

	coreObj := ChatEventToCore(original)
	roundTripped := CoreToChatEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SoldierObjectID, roundTripped.SoldierObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Channel, roundTripped.Channel)
	assert.Equal(t, original.FromName, roundTripped.FromName)
	assert.Equal(t, original.SenderName, roundTripped.SenderName)
	assert.Equal(t, original.Message, roundTripped.Message)
	assert.Equal(t, original.PlayerUID, roundTripped.PlayerUID)
}

func TestRadioEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.RadioEvent{
		ID:              1,
		SoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		Time:            now,
		CaptureFrame:    100,
		Radio:           "AN/PRC-152",
		RadioType:       "SW",
		StartEnd:        "start",
		Channel:         1,
		IsAdditional:    false,
		Frequency:       100.0,
		Code:            "ABC",
	}

	coreObj := RadioEventToCore(original)
	roundTripped := CoreToRadioEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SoldierObjectID, roundTripped.SoldierObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.Radio, roundTripped.Radio)
	assert.Equal(t, original.RadioType, roundTripped.RadioType)
	assert.Equal(t, original.Channel, roundTripped.Channel)
	assert.Equal(t, original.Frequency, roundTripped.Frequency)
	assert.Equal(t, original.Code, roundTripped.Code)
}

func TestServerFpsEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.ServerFpsEvent{
		Time:         now,
		CaptureFrame: 100,
		FpsAverage:   50.5,
		FpsMin:       30.0,
	}

	coreObj := ServerFpsEventToCore(original)
	roundTripped := CoreToServerFpsEvent(coreObj)

	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.FpsAverage, roundTripped.FpsAverage)
	assert.Equal(t, original.FpsMin, roundTripped.FpsMin)
}

func TestAce3DeathEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.Ace3DeathEvent{
		ID:                       1,
		SoldierObjectID:          5,
		Time:                     now,
		CaptureFrame:             100,
		Reason:                   "BLOODLOSS",
		LastDamageSourceObjectID: sql.NullInt32{Int32: 10, Valid: true},
	}

	coreObj := Ace3DeathEventToCore(original)
	roundTripped := CoreToAce3DeathEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SoldierObjectID, roundTripped.SoldierObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.Reason, roundTripped.Reason)
	assert.Equal(t, original.LastDamageSourceObjectID, roundTripped.LastDamageSourceObjectID)
}

func TestAce3UnconsciousEventRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.Ace3UnconsciousEvent{
		ID:              1,
		SoldierObjectID: 5,
		Time:            now,
		CaptureFrame:    100,
		IsUnconscious:   true,
	}

	coreObj := Ace3UnconsciousEventToCore(original)
	roundTripped := CoreToAce3UnconsciousEvent(coreObj)

	assert.Equal(t, original.ID, roundTripped.ID)
	assert.Equal(t, original.SoldierObjectID, roundTripped.SoldierObjectID)
	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.IsUnconscious, roundTripped.IsUnconscious)
}

func TestTimeStateRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	original := model.TimeState{
		Time:           now,
		CaptureFrame:   100,
		SystemTimeUTC:  "2024-01-15T14:30:45.123",
		MissionDate:    "2035-06-15T06:00:00",
		TimeMultiplier: 2.0,
		MissionTime:    3600.5,
	}

	coreObj := TimeStateToCore(original)
	roundTripped := CoreToTimeState(coreObj)

	assert.Equal(t, original.Time, roundTripped.Time)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.SystemTimeUTC, roundTripped.SystemTimeUTC)
	assert.Equal(t, original.MissionDate, roundTripped.MissionDate)
	assert.Equal(t, original.TimeMultiplier, roundTripped.TimeMultiplier)
	assert.Equal(t, original.MissionTime, roundTripped.MissionTime)
}

func TestProjectileEventRoundTrip(t *testing.T) {
	// Create a LineStringZM with 3 points
	coords := []float64{
		100.0, 200.0, 10.0, 50.0,
		150.0, 250.0, 15.0, 52.0,
		200.0, 300.0, 5.0, 55.0,
	}
	seq := geom.NewSequence(coords, geom.DimXYZM)
	ls := geom.NewLineString(seq)

	vehicleID := sql.NullInt32{Int32: 99, Valid: true}
	soldierHitID := uint16(10)
	vehicleHitID := uint16(20)

	original := model.ProjectileEvent{
		FirerObjectID:   42,
		VehicleObjectID: vehicleID,
		CaptureFrame:    100,
		WeaponDisplay:   "Cannon 120mm",
		MagazineDisplay: "APFSDS-T",
		MuzzleDisplay:   "Cannon 120mm",
		SimulationType:  "shotShell",
		MagazineIcon:    `\A3\weapons_f\data\ui\icon_shell.paa`,
		Positions:       ls.AsGeometry(),
		HitSoldiers: []model.ProjectileHitsSoldier{
			{SoldierObjectID: soldierHitID, CaptureFrame: 55, Position: makePoint(200.0, 300.0, 5.0)},
		},
		HitVehicles: []model.ProjectileHitsVehicle{
			{VehicleObjectID: vehicleHitID, CaptureFrame: 55, Position: makePoint(200.0, 300.0, 5.0)},
		},
	}

	// GORM → Core
	coreObj := ProjectileEventToCore(original)

	// Core → GORM
	roundTripped := CoreToProjectileEvent(coreObj)

	assert.Equal(t, original.FirerObjectID, roundTripped.FirerObjectID)
	assert.Equal(t, original.CaptureFrame, roundTripped.CaptureFrame)
	assert.Equal(t, original.VehicleObjectID, roundTripped.VehicleObjectID)
	assert.Equal(t, original.WeaponDisplay, roundTripped.WeaponDisplay)
	assert.Equal(t, original.MagazineDisplay, roundTripped.MagazineDisplay)
	assert.Equal(t, original.MuzzleDisplay, roundTripped.MuzzleDisplay)
	assert.Equal(t, original.SimulationType, roundTripped.SimulationType)
	assert.Equal(t, original.MagazineIcon, roundTripped.MagazineIcon)

	// Verify trajectory positions round-trip
	require.False(t, roundTripped.Positions.IsEmpty())
	rtLs, ok := roundTripped.Positions.AsLineString()
	require.True(t, ok)
	rtSeq := rtLs.Coordinates()
	require.Equal(t, 3, rtSeq.Length())
	assert.Equal(t, 100.0, rtSeq.Get(0).X)
	assert.Equal(t, 200.0, rtSeq.Get(0).Y)
	assert.Equal(t, 10.0, rtSeq.Get(0).Z)
	assert.Equal(t, 50.0, rtSeq.Get(0).M)

	// Verify hits round-trip (merged → split)
	require.Len(t, roundTripped.HitSoldiers, 1)
	require.Len(t, roundTripped.HitVehicles, 1)
	assert.Equal(t, soldierHitID, roundTripped.HitSoldiers[0].SoldierObjectID)
	assert.Equal(t, vehicleHitID, roundTripped.HitVehicles[0].VehicleObjectID)
}

func TestProjectileEventRoundTrip_NoTrajectory(t *testing.T) {
	original := model.ProjectileEvent{
		FirerObjectID: 42,
		CaptureFrame:  100,
		Positions:     geom.Geometry{},
	}

	coreObj := ProjectileEventToCore(original)
	roundTripped := CoreToProjectileEvent(coreObj)

	assert.True(t, roundTripped.Positions.IsEmpty())
}

// Compile-time interface checks for CoreToX functions
var (
	_ model.Soldier              = CoreToSoldier(core.Soldier{})
	_ model.Vehicle              = CoreToVehicle(core.Vehicle{})
	_ model.Marker               = CoreToMarker(core.Marker{})
	_ model.SoldierState         = CoreToSoldierState(core.SoldierState{})
	_ model.VehicleState         = CoreToVehicleState(core.VehicleState{})
	_ model.MarkerState          = CoreToMarkerState(core.MarkerState{})
	_ model.GeneralEvent         = CoreToGeneralEvent(core.GeneralEvent{})
	_ model.KillEvent            = CoreToKillEvent(core.KillEvent{})
	_ model.ChatEvent            = CoreToChatEvent(core.ChatEvent{})
	_ model.RadioEvent           = CoreToRadioEvent(core.RadioEvent{})
	_ model.ServerFpsEvent       = CoreToServerFpsEvent(core.ServerFpsEvent{})
	_ model.Ace3DeathEvent       = CoreToAce3DeathEvent(core.Ace3DeathEvent{})
	_ model.Ace3UnconsciousEvent = CoreToAce3UnconsciousEvent(core.Ace3UnconsciousEvent{})
	_ model.ProjectileEvent      = CoreToProjectileEvent(core.ProjectileEvent{})
	_ model.TimeState            = CoreToTimeState(core.TimeState{})
)
