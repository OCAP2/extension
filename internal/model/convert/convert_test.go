package convert

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	geom "github.com/peterstace/simplefeatures/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// Helper to create a geom.Point from coordinates
func makePoint(x, y, z float64) geom.Point {
	coords := geom.Coordinates{XY: geom.XY{X: x, Y: y}, Z: z}
	pt := geom.NewPoint(coords)
	return pt
}

func TestPointToPosition3D(t *testing.T) {
	pt := makePoint(100.5, 200.5, 50.0)
	pos := pointToPosition3D(pt)

	assert.Equal(t, 100.5, pos.X)
	assert.Equal(t, 200.5, pos.Y)
	assert.Equal(t, 50.0, pos.Z)
}

func TestSoldierToCore(t *testing.T) {
	now := time.Now()
	squadParams, _ := json.Marshal([]any{"test", "params"})

	gormSoldier := model.Soldier{
		MissionID:       1,
		JoinTime:        now,
		JoinFrame:       10,
		ObjectID:          42,
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

	coreSoldier := SoldierToCore(gormSoldier)

	// Core ID = GORM ObjectID (not GORM ID)
	assert.Equal(t, uint16(42), coreSoldier.ID)
	assert.Equal(t, "TestUnit", coreSoldier.UnitName)
	assert.True(t, coreSoldier.IsPlayer)
	assert.Equal(t, "WEST", coreSoldier.Side)
}

func TestSoldierStateToCore(t *testing.T) {
	now := time.Now()
	inVehicleID := sql.NullInt32{Int32: 5, Valid: true}

	gormState := model.SoldierState{
		ID:              1,
		MissionID:       1,
		SoldierObjectID:   2,
		Time:            now,
		CaptureFrame:    100,
		Position:        makePoint(1000.0, 2000.0, 10.0),
		ElevationASL:    10.0,
		Bearing:         90,
		Lifestate:       1,
		InVehicle:       true,
		InVehicleObjectID: inVehicleID,
		VehicleRole:     "driver",
		UnitName:        "TestUnit",
		IsPlayer:        true,
		CurrentRole:     "Rifleman",
		HasStableVitals: true,
		IsDraggedCarried: false,
		Stance:          "Up",
		GroupID:         "Alpha 1-1",
		Side:            "WEST",
		Scores: model.SoldierScores{
			InfantryKills: 5,
			VehicleKills:  2,
			ArmorKills:    1,
			AirKills:      0,
			Deaths:        1,
			TotalScore:    25,
		},
	}

	coreState := SoldierStateToCore(gormState)

	// SoldierID maps from SoldierObjectID
	assert.Equal(t, uint16(2), coreState.SoldierID)
	assert.Equal(t, uint(100), coreState.CaptureFrame)
	assert.Equal(t, 1000.0, coreState.Position.X)
	assert.Equal(t, uint16(90), coreState.Bearing)
	require.NotNil(t, coreState.InVehicleObjectID)
	assert.Equal(t, uint16(5), *coreState.InVehicleObjectID)
	assert.Equal(t, uint8(5), coreState.Scores.InfantryKills)
	assert.Equal(t, "Alpha 1-1", coreState.GroupID)
	assert.Equal(t, "WEST", coreState.Side)
}

func TestVehicleToCore(t *testing.T) {
	now := time.Now()

	gormVehicle := model.Vehicle{
		MissionID:     1,
		JoinTime:      now,
		JoinFrame:     20,
		ObjectID:        10,
		OcapType:      "car",
		ClassName:     "B_MRAP_01_F",
		DisplayName:   "Hunter",
		Customization: "default",
	}

	coreVehicle := VehicleToCore(gormVehicle)

	// Core ID = GORM ObjectID (not GORM ID)
	assert.Equal(t, uint16(10), coreVehicle.ID)
	assert.Equal(t, "car", coreVehicle.OcapType)
	assert.Equal(t, "Hunter", coreVehicle.DisplayName)
}

func TestVehicleStateToCore(t *testing.T) {
	now := time.Now()

	gormState := model.VehicleState{
		ID:              1,
		MissionID:       1,
		VehicleObjectID:   5,
		Time:            now,
		CaptureFrame:    50,
		Position:        makePoint(500.0, 600.0, 0.0),
		ElevationASL:    0.0,
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

	coreState := VehicleStateToCore(gormState)

	// VehicleID maps from VehicleObjectID
	assert.Equal(t, uint16(5), coreState.VehicleID)
	assert.Equal(t, 500.0, coreState.Position.X)
	assert.Equal(t, float32(0.8), coreState.Fuel)
	assert.True(t, coreState.EngineOn)
}

func TestKillEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.KillEvent{
		ID:                  1,
		MissionID:           1,
		Time:                now,
		CaptureFrame:        100,
		VictimSoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		KillerSoldierObjectID: sql.NullInt32{Int32: 10, Valid: true},
		EventText:           "Kill event",
		Distance:            100.0,
	}

	coreEvent := KillEventToCore(gormEvent)

	require.NotNil(t, coreEvent.VictimSoldierID)
	assert.Equal(t, uint(5), *coreEvent.VictimSoldierID)
	require.NotNil(t, coreEvent.KillerSoldierID)
	assert.Equal(t, uint(10), *coreEvent.KillerSoldierID)
}

func TestKillEventToCore_VehicleIDs(t *testing.T) {
	now := time.Now()

	gormEvent := model.KillEvent{
		ID:                    2,
		MissionID:             1,
		Time:                  now,
		CaptureFrame:          200,
		VictimVehicleObjectID: sql.NullInt32{Int32: 20, Valid: true},
		KillerVehicleObjectID: sql.NullInt32{Int32: 30, Valid: true},
		EventText:             "Vehicle kill",
		Distance:              500.0,
	}

	coreEvent := KillEventToCore(gormEvent)

	require.NotNil(t, coreEvent.VictimVehicleID)
	assert.Equal(t, uint(20), *coreEvent.VictimVehicleID)
	require.NotNil(t, coreEvent.KillerVehicleID)
	assert.Equal(t, uint(30), *coreEvent.KillerVehicleID)
}

func TestLineStringToPolyline(t *testing.T) {
	coords := []float64{
		100.0, 200.0,
		300.0, 400.0,
		500.0, 600.0,
	}
	seq := geom.NewSequence(coords, geom.DimXY)
	ls := geom.NewLineString(seq)

	polyline := lineStringToPolyline(ls)

	require.Len(t, polyline, 3)
	assert.Equal(t, 100.0, polyline[0].X)
	assert.Equal(t, 200.0, polyline[0].Y)
	assert.Equal(t, 300.0, polyline[1].X)
	assert.Equal(t, 400.0, polyline[1].Y)
	assert.Equal(t, 500.0, polyline[2].X)
	assert.Equal(t, 600.0, polyline[2].Y)
}

func TestMarkerToCore(t *testing.T) {
	now := time.Now()

	gormMarker := model.Marker{
		ID:           1,
		MissionID:    1,
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

	coreMarker := MarkerToCore(gormMarker)

	assert.Equal(t, "TestMarker", coreMarker.MarkerName)
	assert.Equal(t, float32(45.0), coreMarker.Direction)
	assert.Equal(t, 100.0, coreMarker.Position.X)
}

func TestMissionToCore(t *testing.T) {
	now := time.Now()

	gormMission := &model.Mission{
		MissionName:   "Test Mission",
		BriefingName:  "Test Briefing",
		Author:        "Test Author",
		ServerName:    "Test Server",
		StartTime:     now,
		CaptureDelay:  1.0,
		AddonVersion:  "1.0.0",
		Tag:           "TvT",
		PlayableSlots: model.PlayableSlots{West: 10, East: 10, Independent: 5, Civilian: 2, Logic: 1},
		SideFriendly:  model.SideFriendly{EastWest: false, EastIndependent: true, WestIndependent: true},
		Addons:        []model.Addon{{Name: "TestAddon", WorkshopID: "12345"}},
	}
	gormMission.ID = 1

	coreMission := MissionToCore(gormMission)

	assert.Equal(t, uint(1), coreMission.ID)
	assert.Equal(t, "Test Mission", coreMission.MissionName)
	assert.Equal(t, uint8(10), coreMission.PlayableSlots.West)
	require.Len(t, coreMission.Addons, 1)
	assert.Equal(t, "TestAddon", coreMission.Addons[0].Name)
}

func TestWorldToCore(t *testing.T) {
	gormWorld := &model.World{
		Author:            "BIS",
		WorkshopID:        "",
		DisplayName:       "Altis",
		WorldName:         "altis",
		WorldNameOriginal: "Altis",
		WorldSize:         30720,
		Latitude:          40.0,
		Longitude:         20.0,
		Location:          makePoint(100.0, 200.0, 0.0),
	}
	gormWorld.ID = 1

	coreWorld := WorldToCore(gormWorld)

	assert.Equal(t, uint(1), coreWorld.ID)
	assert.Equal(t, "altis", coreWorld.WorldName)
	assert.Equal(t, float32(30720), coreWorld.WorldSize)
}

func TestGeneralEventToCore(t *testing.T) {
	now := time.Now()
	extraData, _ := json.Marshal(map[string]any{"key": "value"})

	gormEvent := model.GeneralEvent{
		ID:           1,
		MissionID:    1,
		Time:         now,
		CaptureFrame: 100,
		Name:         "TestEvent",
		Message:      "Test message",
		ExtraData:    datatypes.JSON(extraData),
	}

	coreEvent := GeneralEventToCore(gormEvent)

	assert.Equal(t, "TestEvent", coreEvent.Name)
	assert.Equal(t, "Test message", coreEvent.Message)
	assert.Equal(t, "value", coreEvent.ExtraData["key"])
}

func TestChatEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.ChatEvent{
		ID:            1,
		MissionID:     1,
		SoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		Time:          now,
		CaptureFrame:  100,
		Channel:       "Global",
		FromName:      "Player1",
		SenderName:    "John",
		Message:       "Hello world",
		PlayerUID:     "12345678",
	}

	coreEvent := ChatEventToCore(gormEvent)

	require.NotNil(t, coreEvent.SoldierID)
	assert.Equal(t, uint(5), *coreEvent.SoldierID)
	assert.Equal(t, "Global", coreEvent.Channel)
	assert.Equal(t, "Hello world", coreEvent.Message)
}

func TestRadioEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.RadioEvent{
		ID:            1,
		MissionID:     1,
		SoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
		Time:          now,
		CaptureFrame:  100,
		Radio:         "AN/PRC-152",
		RadioType:     "SW",
		StartEnd:      "start",
		Channel:       1,
		IsAdditional:  false,
		Frequency:     100.0,
		Code:          "ABC",
	}

	coreEvent := RadioEventToCore(gormEvent)

	assert.Equal(t, "AN/PRC-152", coreEvent.Radio)
	assert.Equal(t, float32(100.0), coreEvent.Frequency)
}

func TestServerFpsEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.ServerFpsEvent{
		MissionID:    1,
		Time:         now,
		CaptureFrame: 100,
		FpsAverage:   50.5,
		FpsMin:       30.0,
	}

	coreEvent := ServerFpsEventToCore(gormEvent)

	assert.Equal(t, float32(50.5), coreEvent.FpsAverage)
	assert.Equal(t, float32(30.0), coreEvent.FpsMin)
}

func TestTimeStateToCore(t *testing.T) {
	now := time.Now()

	gormState := model.TimeState{
		MissionID:      1,
		Time:           now,
		CaptureFrame:   100,
		SystemTimeUTC:  "2024-01-15T14:30:45.123",
		MissionDate:    "2035-06-15T06:00:00",
		TimeMultiplier: 2.0,
		MissionTime:    3600.5,
	}

	coreState := TimeStateToCore(gormState)

	assert.Equal(t, uint(100), coreState.CaptureFrame)
	assert.Equal(t, "2024-01-15T14:30:45.123", coreState.SystemTimeUTC)
	assert.Equal(t, "2035-06-15T06:00:00", coreState.MissionDate)
	assert.Equal(t, float32(2.0), coreState.TimeMultiplier)
	assert.Equal(t, float32(3600.5), coreState.MissionTime)
}

func TestAce3DeathEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.Ace3DeathEvent{
		ID:                     1,
		MissionID:              1,
		SoldierObjectID:          5,
		Time:                   now,
		CaptureFrame:           100,
		Reason:                 "BLOODLOSS",
		LastDamageSourceObjectID: sql.NullInt32{Int32: 10, Valid: true},
	}

	coreEvent := Ace3DeathEventToCore(gormEvent)

	assert.Equal(t, "BLOODLOSS", coreEvent.Reason)
	require.NotNil(t, coreEvent.LastDamageSourceID)
	assert.Equal(t, uint(10), *coreEvent.LastDamageSourceID)
}

func TestAce3UnconsciousEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.Ace3UnconsciousEvent{
		ID:              1,
		MissionID:       1,
		SoldierObjectID: 5,
		Time:            now,
		CaptureFrame:    100,
		IsUnconscious:   true, // Unit went unconscious
	}

	coreEvent := Ace3UnconsciousEventToCore(gormEvent)

	assert.Equal(t, uint(5), coreEvent.SoldierID)
	assert.True(t, coreEvent.IsUnconscious)
}

func TestMarkerStateToCore(t *testing.T) {
	now := time.Now()

	gormState := model.MarkerState{
		ID:           1,
		MissionID:    1,
		MarkerID:     5,
		Time:         now,
		CaptureFrame: 100,
		Position:     makePoint(150.0, 250.0, 0.0),
		Direction:    90.0,
		Alpha:        0.5,
	}

	coreState := MarkerStateToCore(gormState)

	assert.Equal(t, uint(5), coreState.MarkerID)
	assert.Equal(t, 150.0, coreState.Position.X)
	assert.Equal(t, float32(0.5), coreState.Alpha)
}

// Test with nil/invalid values
func TestSoldierStateToCore_NilInVehicleID(t *testing.T) {
	gormState := model.SoldierState{
		InVehicleObjectID: sql.NullInt32{Valid: false},
	}

	coreState := SoldierStateToCore(gormState)

	assert.Nil(t, coreState.InVehicleObjectID)
}

func TestProjectileEventToCore(t *testing.T) {
	// Create a LineStringZM with 3 points
	coords := []float64{
		100.0, 200.0, 10.0, 50.0,
		150.0, 250.0, 15.0, 52.0,
		200.0, 300.0, 5.0, 55.0,
	}
	seq := geom.NewSequence(coords, geom.DimXYZM)
	ls := geom.NewLineString(seq)

	vehicleID := sql.NullInt32{Int32: 99, Valid: true}

	gormEvent := model.ProjectileEvent{
		MissionID:       1,
		FirerObjectID:   42,
		VehicleObjectID: vehicleID,
		CaptureFrame:    100,
		Weapon:          "cannon_120mm",
		WeaponDisplay:   "Cannon 120mm",
		Magazine:        "120mm_APFSDS",
		MagazineDisplay: "APFSDS-T",
		Muzzle:          "cannon_120mm",
		MuzzleDisplay:   "Cannon 120mm",
		Mode:            "Single",
		SimulationType:  "shotShell",
		MagazineIcon:    `\A3\weapons_f\data\ui\icon_shell.paa`,
		Positions:       ls.AsGeometry(),
		HitSoldiers: []model.ProjectileHitsSoldier{
			{SoldierObjectID: 10, CaptureFrame: 55, Position: makePoint(200.0, 300.0, 5.0)},
		},
		HitVehicles: []model.ProjectileHitsVehicle{
			{VehicleObjectID: 20, CaptureFrame: 55, Position: makePoint(200.0, 300.0, 5.0)},
		},
	}

	result := ProjectileEventToCore(gormEvent)

	assert.Equal(t, uint16(42), result.FirerObjectID)
	assert.Equal(t, uint(100), result.CaptureFrame)
	assert.Equal(t, "Cannon 120mm", result.WeaponDisplay)
	assert.Equal(t, "APFSDS-T", result.MagazineDisplay)
	assert.Equal(t, "Cannon 120mm", result.MuzzleDisplay)
	assert.Equal(t, "shotShell", result.SimulationType)
	assert.Equal(t, `\A3\weapons_f\data\ui\icon_shell.paa`, result.MagazineIcon)

	// VehicleObjectID conversion
	require.NotNil(t, result.VehicleObjectID)
	assert.Equal(t, uint16(99), *result.VehicleObjectID)

	// Trajectory conversion
	require.Len(t, result.Trajectory, 3)
	assert.Equal(t, 100.0, result.Trajectory[0].Position.X)
	assert.Equal(t, 200.0, result.Trajectory[0].Position.Y)
	assert.Equal(t, 10.0, result.Trajectory[0].Position.Z)
	assert.Equal(t, uint(50), result.Trajectory[0].Frame)
	assert.Equal(t, 200.0, result.Trajectory[2].Position.X)
	assert.Equal(t, uint(55), result.Trajectory[2].Frame)

	// Hits merging
	require.Len(t, result.Hits, 2)
	// First hit is soldier
	require.NotNil(t, result.Hits[0].SoldierID)
	assert.Equal(t, uint16(10), *result.Hits[0].SoldierID)
	assert.Nil(t, result.Hits[0].VehicleID)
	assert.Equal(t, uint(55), result.Hits[0].CaptureFrame)
	assert.Equal(t, 200.0, result.Hits[0].Position.X)
	// Second hit is vehicle
	require.NotNil(t, result.Hits[1].VehicleID)
	assert.Equal(t, uint16(20), *result.Hits[1].VehicleID)
	assert.Nil(t, result.Hits[1].SoldierID)
}

func TestProjectileEventToCore_EmptyPositions(t *testing.T) {
	gormEvent := model.ProjectileEvent{
		MissionID:     1,
		FirerObjectID: 42,
		CaptureFrame:  100,
		Weapon:        "throw",
		Positions:     geom.Geometry{}, // empty geometry
	}

	result := ProjectileEventToCore(gormEvent)

	assert.Nil(t, result.Trajectory)
	assert.Nil(t, result.VehicleObjectID)
	assert.Empty(t, result.Hits)
}

func TestProjectileEventToCore_NullVehicleObjectID(t *testing.T) {
	gormEvent := model.ProjectileEvent{
		MissionID:       1,
		FirerObjectID:   42,
		VehicleObjectID: sql.NullInt32{Valid: false},
	}

	result := ProjectileEventToCore(gormEvent)
	assert.Nil(t, result.VehicleObjectID)
}

// Compile-time interface checks
var (
	_ core.Soldier              = SoldierToCore(model.Soldier{})
	_ core.SoldierState         = SoldierStateToCore(model.SoldierState{})
	_ core.Vehicle              = VehicleToCore(model.Vehicle{})
	_ core.VehicleState         = VehicleStateToCore(model.VehicleState{})
	_ core.ProjectileEvent      = ProjectileEventToCore(model.ProjectileEvent{})
	_ core.GeneralEvent         = GeneralEventToCore(model.GeneralEvent{})
	_ core.KillEvent            = KillEventToCore(model.KillEvent{})
	_ core.ChatEvent            = ChatEventToCore(model.ChatEvent{})
	_ core.RadioEvent           = RadioEventToCore(model.RadioEvent{})
	_ core.ServerFpsEvent       = ServerFpsEventToCore(model.ServerFpsEvent{})
	_ core.TimeState            = TimeStateToCore(model.TimeState{})
	_ core.Ace3DeathEvent       = Ace3DeathEventToCore(model.Ace3DeathEvent{})
	_ core.Ace3UnconsciousEvent = Ace3UnconsciousEventToCore(model.Ace3UnconsciousEvent{})
	_ core.Marker               = MarkerToCore(model.Marker{})
	_ core.MarkerState          = MarkerStateToCore(model.MarkerState{})
)
