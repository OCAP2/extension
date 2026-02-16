package convert

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCoreToSoldier(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.Soldier{
		ID:              42,
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
		SquadParams:     []any{"test", "params"},
	}

	result := CoreToSoldier(input)

	assert.Equal(t, uint16(42), result.ObjectID)
	assert.Equal(t, now, result.JoinTime)
	assert.Equal(t, uint(10), result.JoinFrame)
	assert.Equal(t, "man", result.OcapType)
	assert.Equal(t, "TestUnit", result.UnitName)
	assert.Equal(t, "Alpha", result.GroupID)
	assert.Equal(t, "WEST", result.Side)
	assert.True(t, result.IsPlayer)
	assert.Equal(t, "Rifleman", result.RoleDescription)
	assert.Equal(t, "B_Soldier_F", result.ClassName)
	assert.Equal(t, "John Doe", result.DisplayName)
	assert.Equal(t, "12345678", result.PlayerUID)

	var params []any
	json.Unmarshal(result.SquadParams, &params)
	assert.Equal(t, []any{"test", "params"}, params)
}

func TestCoreToVehicle(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.Vehicle{
		ID:            10,
		JoinTime:      now,
		JoinFrame:     20,
		OcapType:      "car",
		ClassName:     "B_MRAP_01_F",
		DisplayName:   "Hunter",
		Customization: "default",
	}

	result := CoreToVehicle(input)

	assert.Equal(t, uint16(10), result.ObjectID)
	assert.Equal(t, now, result.JoinTime)
	assert.Equal(t, uint(20), result.JoinFrame)
	assert.Equal(t, "car", result.OcapType)
	assert.Equal(t, "B_MRAP_01_F", result.ClassName)
	assert.Equal(t, "Hunter", result.DisplayName)
	assert.Equal(t, "default", result.Customization)
}

func TestCoreToSoldierState(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	inVehicleID := uint16(5)

	input := core.SoldierState{
		SoldierID:         2,
		Time:              now,
		CaptureFrame:      100,
		Position:          core.Position3D{X: 1000.0, Y: 2000.0, Z: 10.0},
		Bearing:           90,
		Lifestate:         1,
		InVehicle:         true,
		InVehicleObjectID: &inVehicleID,
		VehicleRole:       "driver",
		UnitName:          "TestUnit",
		IsPlayer:          true,
		CurrentRole:       "Rifleman",
		HasStableVitals:   true,
		IsDraggedCarried:  false,
		Stance:            "Up",
		GroupID:           "Alpha 1-1",
		Side:              "WEST",
		Scores: core.SoldierScores{
			InfantryKills: 5,
			VehicleKills:  2,
			ArmorKills:    1,
			AirKills:      0,
			Deaths:        1,
			TotalScore:    25,
		},
	}

	result := CoreToSoldierState(input)

	assert.Equal(t, uint16(2), result.SoldierObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, float32(10.0), result.ElevationASL)
	assert.Equal(t, uint16(90), result.Bearing)
	assert.Equal(t, uint8(1), result.Lifestate)
	assert.True(t, result.InVehicle)
	assert.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, result.InVehicleObjectID)
	assert.Equal(t, "driver", result.VehicleRole)
	assert.Equal(t, "TestUnit", result.UnitName)
	assert.True(t, result.IsPlayer)
	assert.Equal(t, "Rifleman", result.CurrentRole)
	assert.True(t, result.HasStableVitals)
	assert.False(t, result.IsDraggedCarried)
	assert.Equal(t, "Up", result.Stance)
	assert.Equal(t, "Alpha 1-1", result.GroupID)
	assert.Equal(t, "WEST", result.Side)
	assert.Equal(t, model.SoldierScores{InfantryKills: 5, VehicleKills: 2, ArmorKills: 1, Deaths: 1, TotalScore: 25}, result.Scores)

	coord, ok := result.Position.Coordinates()
	require.True(t, ok)
	assert.Equal(t, 1000.0, coord.XY.X)
	assert.Equal(t, 2000.0, coord.XY.Y)
	assert.Equal(t, 10.0, coord.Z)
}

func TestCoreToSoldierState_NilInVehicleID(t *testing.T) {
	result := CoreToSoldierState(core.SoldierState{})
	assert.False(t, result.InVehicleObjectID.Valid)
}

func TestCoreToVehicleState(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.VehicleState{
		VehicleID:       5,
		Time:            now,
		CaptureFrame:    50,
		Position:        core.Position3D{X: 500.0, Y: 600.0, Z: 0.0},
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

	result := CoreToVehicleState(input)

	assert.Equal(t, uint16(5), result.VehicleObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(50), result.CaptureFrame)
	assert.Equal(t, float32(0.0), result.ElevationASL)
	assert.Equal(t, uint16(180), result.Bearing)
	assert.True(t, result.IsAlive)
	assert.Equal(t, "1,2,3", result.Crew)
	assert.InDelta(t, 0.8, result.Fuel, 0.001)
	assert.InDelta(t, 0.1, result.Damage, 0.001)
	assert.False(t, result.Locked)
	assert.True(t, result.EngineOn)
	assert.Equal(t, "WEST", result.Side)
	assert.Equal(t, float32(45.0), result.TurretAzimuth)
	assert.Equal(t, float32(-10.0), result.TurretElevation)
}

func TestCoreToMarker(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.Marker{
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
		Position:     core.Position3D{X: 100.0, Y: 200.0, Z: 0.0},
		Shape:        "ICON",
		Alpha:        1.0,
		Brush:        "Solid",
		IsDeleted:    false,
	}

	result := CoreToMarker(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(50), result.CaptureFrame)
	assert.Equal(t, "TestMarker", result.MarkerName)
	assert.Equal(t, float32(45.0), result.Direction)
	assert.Equal(t, "mil_dot", result.MarkerType)
	assert.Equal(t, "Test", result.Text)
	assert.Equal(t, int(2), result.OwnerID)
	assert.Equal(t, "ColorRed", result.Color)
	assert.Equal(t, "WEST", result.Side)
	assert.Equal(t, "ICON", result.Shape)
	assert.Equal(t, float32(1.0), result.Alpha)
	assert.Equal(t, "Solid", result.Brush)
	assert.False(t, result.IsDeleted)
}

func TestCoreToMarkerState(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.MarkerState{
		ID:           1,
		MarkerID:     5,
		Time:         now,
		CaptureFrame: 100,
		Position:     core.Position3D{X: 150.0, Y: 250.0, Z: 0.0},
		Direction:    90.0,
		Alpha:        0.5,
	}

	result := CoreToMarkerState(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, uint(5), result.MarkerID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, float32(90.0), result.Direction)
	assert.Equal(t, float32(0.5), result.Alpha)
}

func TestCoreToGeneralEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.GeneralEvent{
		ID:           1,
		Time:         now,
		CaptureFrame: 100,
		Name:         "TestEvent",
		Message:      "Test message",
		ExtraData:    map[string]any{"key": "value"},
	}

	result := CoreToGeneralEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, "TestEvent", result.Name)
	assert.Equal(t, "Test message", result.Message)

	var extra map[string]any
	json.Unmarshal(result.ExtraData, &extra)
	assert.Equal(t, "value", extra["key"])
}

func TestCoreToKillEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	victimID := uint(5)
	killerID := uint(30)

	input := core.KillEvent{
		ID:                1,
		Time:              now,
		CaptureFrame:      100,
		VictimSoldierID:   &victimID,
		KillerVehicleID:   &killerID,
		EventText:         "Kill event",
		Distance:          100.0,
	}

	result := CoreToKillEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, result.VictimSoldierObjectID)
	assert.Equal(t, sql.NullInt32{Int32: 30, Valid: true}, result.KillerVehicleObjectID)
	assert.False(t, result.VictimVehicleObjectID.Valid)
	assert.False(t, result.KillerSoldierObjectID.Valid)
	assert.Equal(t, "Kill event", result.EventText)
	assert.Equal(t, float32(100.0), result.Distance)
}

func TestCoreToChatEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	soldierID := uint(5)

	input := core.ChatEvent{
		ID:           1,
		SoldierID:    &soldierID,
		Time:         now,
		CaptureFrame: 100,
		Channel:      "Global",
		FromName:     "Player1",
		SenderName:   "John",
		Message:      "Hello world",
		PlayerUID:    "12345678",
	}

	result := CoreToChatEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, result.SoldierObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, "Global", result.Channel)
	assert.Equal(t, "Player1", result.FromName)
	assert.Equal(t, "John", result.SenderName)
	assert.Equal(t, "Hello world", result.Message)
	assert.Equal(t, "12345678", result.PlayerUID)
}

func TestCoreToRadioEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	soldierID := uint(5)

	input := core.RadioEvent{
		ID:           1,
		SoldierID:    &soldierID,
		Time:         now,
		CaptureFrame: 100,
		Radio:        "AN/PRC-152",
		RadioType:    "SW",
		StartEnd:     "start",
		Channel:      1,
		IsAdditional: false,
		Frequency:    100.0,
		Code:         "ABC",
	}

	result := CoreToRadioEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, sql.NullInt32{Int32: 5, Valid: true}, result.SoldierObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, "AN/PRC-152", result.Radio)
	assert.Equal(t, "SW", result.RadioType)
	assert.Equal(t, int8(1), result.Channel)
	assert.Equal(t, float32(100.0), result.Frequency)
	assert.Equal(t, "ABC", result.Code)
}

func TestCoreToServerFpsEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.ServerFpsEvent{
		Time:         now,
		CaptureFrame: 100,
		FpsAverage:   50.5,
		FpsMin:       30.0,
	}

	result := CoreToServerFpsEvent(input)

	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, float32(50.5), result.FpsAverage)
	assert.Equal(t, float32(30.0), result.FpsMin)
}

func TestCoreToAce3DeathEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	sourceID := uint(10)

	input := core.Ace3DeathEvent{
		ID:                 1,
		SoldierID:          5,
		Time:               now,
		CaptureFrame:       100,
		Reason:             "BLOODLOSS",
		LastDamageSourceID: &sourceID,
	}

	result := CoreToAce3DeathEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, uint16(5), result.SoldierObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, "BLOODLOSS", result.Reason)
	assert.Equal(t, sql.NullInt32{Int32: 10, Valid: true}, result.LastDamageSourceObjectID)
}

func TestCoreToAce3UnconsciousEvent(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.Ace3UnconsciousEvent{
		ID:            1,
		SoldierID:     5,
		Time:          now,
		CaptureFrame:  100,
		IsUnconscious: true,
	}

	result := CoreToAce3UnconsciousEvent(input)

	assert.Equal(t, uint(1), result.ID)
	assert.Equal(t, uint16(5), result.SoldierObjectID)
	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.True(t, result.IsUnconscious)
}

func TestCoreToTimeState(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.TimeState{
		Time:           now,
		CaptureFrame:   100,
		SystemTimeUTC:  "2024-01-15T14:30:45.123",
		MissionDate:    "2035-06-15T06:00:00",
		TimeMultiplier: 2.0,
		MissionTime:    3600.5,
	}

	result := CoreToTimeState(input)

	assert.Equal(t, now, result.Time)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, "2024-01-15T14:30:45.123", result.SystemTimeUTC)
	assert.Equal(t, "2035-06-15T06:00:00", result.MissionDate)
	assert.Equal(t, float32(2.0), result.TimeMultiplier)
	assert.Equal(t, float32(3600.5), result.MissionTime)
}

func TestCoreToProjectileEvent(t *testing.T) {
	soldierHitID := uint16(10)
	vehicleHitID := uint16(20)
	vehicleObjID := uint16(99)

	input := core.ProjectileEvent{
		FirerObjectID:   42,
		VehicleObjectID: &vehicleObjID,
		CaptureFrame:    100,
		WeaponDisplay:   "Cannon 120mm",
		MagazineDisplay: "APFSDS-T",
		MuzzleDisplay:   "Cannon 120mm",
		SimulationType:  "shotShell",
		MagazineIcon:    `\A3\weapons_f\data\ui\icon_shell.paa`,
		Trajectory: []core.TrajectoryPoint{
			{Position: core.Position3D{X: 100.0, Y: 200.0, Z: 10.0}, Frame: 50},
			{Position: core.Position3D{X: 150.0, Y: 250.0, Z: 15.0}, Frame: 52},
			{Position: core.Position3D{X: 200.0, Y: 300.0, Z: 5.0}, Frame: 55},
		},
		Hits: []core.ProjectileHit{
			{SoldierID: &soldierHitID, CaptureFrame: 55, Position: core.Position3D{X: 200.0, Y: 300.0, Z: 5.0}, ComponentsHit: []string{"head"}},
			{VehicleID: &vehicleHitID, CaptureFrame: 55, Position: core.Position3D{X: 200.0, Y: 300.0, Z: 5.0}},
		},
	}

	result := CoreToProjectileEvent(input)

	assert.Equal(t, uint16(42), result.FirerObjectID)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, sql.NullInt32{Int32: 99, Valid: true}, result.VehicleObjectID)
	assert.Equal(t, "Cannon 120mm", result.WeaponDisplay)
	assert.Equal(t, "APFSDS-T", result.MagazineDisplay)
	assert.Equal(t, "shotShell", result.SimulationType)

	// Verify trajectory
	require.False(t, result.Positions.IsEmpty())
	ls, ok := result.Positions.AsLineString()
	require.True(t, ok)
	seq := ls.Coordinates()
	require.Equal(t, 3, seq.Length())
	assert.Equal(t, 100.0, seq.Get(0).X)
	assert.Equal(t, 200.0, seq.Get(0).Y)
	assert.Equal(t, 10.0, seq.Get(0).Z)
	assert.Equal(t, 50.0, seq.Get(0).M)

	// Verify hits split into soldiers and vehicles
	require.Len(t, result.HitSoldiers, 1)
	require.Len(t, result.HitVehicles, 1)
	assert.Equal(t, soldierHitID, result.HitSoldiers[0].SoldierObjectID)
	assert.JSONEq(t, `["head"]`, string(result.HitSoldiers[0].ComponentsHit))
	assert.Equal(t, vehicleHitID, result.HitVehicles[0].VehicleObjectID)
}

func TestCoreToProjectileEvent_NoTrajectory(t *testing.T) {
	input := core.ProjectileEvent{
		FirerObjectID: 42,
		CaptureFrame:  100,
	}

	result := CoreToProjectileEvent(input)

	assert.True(t, result.Positions.IsEmpty())
}

func TestCoreToKillEvent_AllPointers(t *testing.T) {
	victimSoldierID := uint(1)
	victimVehicleID := uint(2)
	killerSoldierID := uint(3)
	killerVehicleID := uint(4)

	input := core.KillEvent{
		VictimSoldierID: &victimSoldierID,
		VictimVehicleID: &victimVehicleID,
		KillerSoldierID: &killerSoldierID,
		KillerVehicleID: &killerVehicleID,
	}

	result := CoreToKillEvent(input)

	assert.Equal(t, sql.NullInt32{Int32: 1, Valid: true}, result.VictimSoldierObjectID)
	assert.Equal(t, sql.NullInt32{Int32: 2, Valid: true}, result.VictimVehicleObjectID)
	assert.Equal(t, sql.NullInt32{Int32: 3, Valid: true}, result.KillerSoldierObjectID)
	assert.Equal(t, sql.NullInt32{Int32: 4, Valid: true}, result.KillerVehicleObjectID)
}

func TestCoreToMission(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	input := core.Mission{
		MissionName:      "TestMission",
		BriefingName:     "Briefing",
		Author:           "TestAuthor",
		ServerName:       "TestServer",
		StartTime:        now,
		WorldID:          1,
		CaptureDelay:     5,
		Tag:              "TvT",
		ExtensionVersion: "5.0.0",
		PlayableSlots: core.PlayableSlots{
			West:        10,
			East:        10,
			Independent: 5,
			Civilian:    0,
			Logic:       2,
		},
		SideFriendly: core.SideFriendly{
			EastWest:        false,
			EastIndependent: true,
			WestIndependent: false,
		},
		Addons: []core.Addon{
			{Name: "ace", WorkshopID: "123"},
			{Name: "tfar", WorkshopID: "456"},
		},
	}

	result := CoreToMission(input)

	assert.Equal(t, "TestMission", result.MissionName)
	assert.Equal(t, "Briefing", result.BriefingName)
	assert.Equal(t, "TestAuthor", result.Author)
	assert.Equal(t, "TestServer", result.ServerName)
	assert.Equal(t, now, result.StartTime)
	assert.Equal(t, uint(1), result.WorldID)
	assert.Equal(t, float32(5), result.CaptureDelay)
	assert.Equal(t, "TvT", result.Tag)
	assert.Equal(t, uint8(10), result.PlayableSlots.West)
	assert.Equal(t, uint8(10), result.PlayableSlots.East)
	assert.Equal(t, uint8(5), result.PlayableSlots.Independent)
	assert.False(t, result.SideFriendly.EastWest)
	assert.True(t, result.SideFriendly.EastIndependent)
	require.Len(t, result.Addons, 2)
	assert.Equal(t, "ace", result.Addons[0].Name)
	assert.Equal(t, "456", result.Addons[1].WorkshopID)
}

func TestCoreToWorld(t *testing.T) {
	input := core.World{
		Author:            "BIS",
		WorkshopID:        "12345",
		DisplayName:       "Altis",
		WorldName:         "altis",
		WorldNameOriginal: "Altis",
		WorldSize:         30720,
		Latitude:          -40.0,
		Longitude:         30.0,
		Location:          core.Position3D{X: 100.0, Y: 200.0, Z: 0.0},
	}

	result := CoreToWorld(input)

	assert.Equal(t, "BIS", result.Author)
	assert.Equal(t, "12345", result.WorkshopID)
	assert.Equal(t, "Altis", result.DisplayName)
	assert.Equal(t, "altis", result.WorldName)
	assert.Equal(t, float32(30720), result.WorldSize)
	assert.Equal(t, float32(-40.0), result.Latitude)
	assert.Equal(t, float32(30.0), result.Longitude)

	coord, ok := result.Location.Coordinates()
	require.True(t, ok)
	assert.Equal(t, 100.0, coord.XY.X)
	assert.Equal(t, 200.0, coord.XY.Y)
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
