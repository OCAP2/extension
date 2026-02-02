package convert

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	geom "github.com/peterstace/simplefeatures/geom"
	"gorm.io/datatypes"
)

// Helper to create a geom.Point from coordinates
func makePoint(x, y, z float64) geom.Point {
	coords := geom.Coordinates{XY: geom.XY{X: x, Y: y}, Z: z}
	pt, _ := geom.NewPoint(coords)
	return pt
}

func TestPointToPosition3D(t *testing.T) {
	pt := makePoint(100.5, 200.5, 50.0)
	pos := pointToPosition3D(pt)

	if pos.X != 100.5 {
		t.Errorf("expected X=100.5, got %f", pos.X)
	}
	if pos.Y != 200.5 {
		t.Errorf("expected Y=200.5, got %f", pos.Y)
	}
	if pos.Z != 50.0 {
		t.Errorf("expected Z=50.0, got %f", pos.Z)
	}
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
	if coreSoldier.ID != 42 {
		t.Errorf("expected ID=42 (ObjectID), got %d", coreSoldier.ID)
	}
	if coreSoldier.MissionID != 1 {
		t.Errorf("expected MissionID=1, got %d", coreSoldier.MissionID)
	}
	if coreSoldier.UnitName != "TestUnit" {
		t.Errorf("expected UnitName=TestUnit, got %s", coreSoldier.UnitName)
	}
	if !coreSoldier.IsPlayer {
		t.Error("expected IsPlayer=true")
	}
	if coreSoldier.Side != "WEST" {
		t.Errorf("expected Side=WEST, got %s", coreSoldier.Side)
	}
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
	if coreState.SoldierID != 2 {
		t.Errorf("expected SoldierID=2 (from SoldierObjectID), got %d", coreState.SoldierID)
	}
	if coreState.CaptureFrame != 100 {
		t.Errorf("expected CaptureFrame=100, got %d", coreState.CaptureFrame)
	}
	if coreState.Position.X != 1000.0 {
		t.Errorf("expected Position.X=1000.0, got %f", coreState.Position.X)
	}
	if coreState.Bearing != 90 {
		t.Errorf("expected Bearing=90, got %d", coreState.Bearing)
	}
	if coreState.InVehicleObjectID == nil || *coreState.InVehicleObjectID != 5 {
		t.Error("expected InVehicleObjectID=5")
	}
	if coreState.Scores.InfantryKills != 5 {
		t.Errorf("expected Scores.InfantryKills=5, got %d", coreState.Scores.InfantryKills)
	}
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
	if coreVehicle.ID != 10 {
		t.Errorf("expected ID=10 (ObjectID), got %d", coreVehicle.ID)
	}
	if coreVehicle.OcapType != "car" {
		t.Errorf("expected OcapType=car, got %s", coreVehicle.OcapType)
	}
	if coreVehicle.DisplayName != "Hunter" {
		t.Errorf("expected DisplayName=Hunter, got %s", coreVehicle.DisplayName)
	}
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
	if coreState.VehicleID != 5 {
		t.Errorf("expected VehicleID=5 (from VehicleObjectID), got %d", coreState.VehicleID)
	}
	if coreState.Position.X != 500.0 {
		t.Errorf("expected Position.X=500.0, got %f", coreState.Position.X)
	}
	if coreState.Fuel != 0.8 {
		t.Errorf("expected Fuel=0.8, got %f", coreState.Fuel)
	}
	if !coreState.EngineOn {
		t.Error("expected EngineOn=true")
	}
}

func TestFiredEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.FiredEvent{
		ID:                1,
		MissionID:         1,
		SoldierObjectID:     2,
		Time:              now,
		CaptureFrame:      100,
		Weapon:            "arifle_MX_F",
		Magazine:          "30Rnd_65x39_caseless_mag",
		FiringMode:        "Single",
		StartPosition:     makePoint(100.0, 100.0, 1.5),
		StartElevationASL: 1.5,
		EndPosition:       makePoint(200.0, 200.0, 1.0),
		EndElevationASL:   1.0,
	}

	coreEvent := FiredEventToCore(gormEvent)

	// SoldierID maps from SoldierObjectID
	if coreEvent.SoldierID != 2 {
		t.Errorf("expected SoldierID=2 (from SoldierObjectID), got %d", coreEvent.SoldierID)
	}
	if coreEvent.Weapon != "arifle_MX_F" {
		t.Errorf("expected Weapon=arifle_MX_F, got %s", coreEvent.Weapon)
	}
	if coreEvent.StartPos.X != 100.0 {
		t.Errorf("expected StartPos.X=100.0, got %f", coreEvent.StartPos.X)
	}
	if coreEvent.EndPos.X != 200.0 {
		t.Errorf("expected EndPos.X=200.0, got %f", coreEvent.EndPos.X)
	}
}

func TestHitEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.HitEvent{
		ID:                   1,
		MissionID:            1,
		Time:                 now,
		CaptureFrame:         100,
		VictimSoldierObjectID:  sql.NullInt32{Int32: 5, Valid: true},
		ShooterSoldierObjectID: sql.NullInt32{Int32: 10, Valid: true},
		EventText:            "Hit event",
		Distance:             50.5,
	}

	coreEvent := HitEventToCore(gormEvent)

	if coreEvent.VictimSoldierID == nil || *coreEvent.VictimSoldierID != 5 {
		t.Error("expected VictimSoldierID=5")
	}
	if coreEvent.ShooterSoldierID == nil || *coreEvent.ShooterSoldierID != 10 {
		t.Error("expected ShooterSoldierID=10")
	}
	if coreEvent.Distance != 50.5 {
		t.Errorf("expected Distance=50.5, got %f", coreEvent.Distance)
	}
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

	if coreEvent.VictimSoldierID == nil || *coreEvent.VictimSoldierID != 5 {
		t.Error("expected VictimSoldierID=5")
	}
	if coreEvent.KillerSoldierID == nil || *coreEvent.KillerSoldierID != 10 {
		t.Error("expected KillerSoldierID=10")
	}
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

	if coreMarker.MarkerName != "TestMarker" {
		t.Errorf("expected MarkerName=TestMarker, got %s", coreMarker.MarkerName)
	}
	if coreMarker.Direction != 45.0 {
		t.Errorf("expected Direction=45.0, got %f", coreMarker.Direction)
	}
	if coreMarker.Position.X != 100.0 {
		t.Errorf("expected Position.X=100.0, got %f", coreMarker.Position.X)
	}
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

	if coreMission.ID != 1 {
		t.Errorf("expected ID=1, got %d", coreMission.ID)
	}
	if coreMission.MissionName != "Test Mission" {
		t.Errorf("expected MissionName=Test Mission, got %s", coreMission.MissionName)
	}
	if coreMission.PlayableSlots.West != 10 {
		t.Errorf("expected PlayableSlots.West=10, got %d", coreMission.PlayableSlots.West)
	}
	if len(coreMission.Addons) != 1 || coreMission.Addons[0].Name != "TestAddon" {
		t.Error("expected 1 addon with name TestAddon")
	}
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

	if coreWorld.ID != 1 {
		t.Errorf("expected ID=1, got %d", coreWorld.ID)
	}
	if coreWorld.WorldName != "altis" {
		t.Errorf("expected WorldName=altis, got %s", coreWorld.WorldName)
	}
	if coreWorld.WorldSize != 30720 {
		t.Errorf("expected WorldSize=30720, got %f", coreWorld.WorldSize)
	}
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

	if coreEvent.Name != "TestEvent" {
		t.Errorf("expected Name=TestEvent, got %s", coreEvent.Name)
	}
	if coreEvent.Message != "Test message" {
		t.Errorf("expected Message=Test message, got %s", coreEvent.Message)
	}
	if coreEvent.ExtraData["key"] != "value" {
		t.Error("expected ExtraData[key]=value")
	}
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

	if coreEvent.SoldierID == nil || *coreEvent.SoldierID != 5 {
		t.Error("expected SoldierID=5")
	}
	if coreEvent.Channel != "Global" {
		t.Errorf("expected Channel=Global, got %s", coreEvent.Channel)
	}
	if coreEvent.Message != "Hello world" {
		t.Errorf("expected Message=Hello world, got %s", coreEvent.Message)
	}
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

	if coreEvent.Radio != "AN/PRC-152" {
		t.Errorf("expected Radio=AN/PRC-152, got %s", coreEvent.Radio)
	}
	if coreEvent.Frequency != 100.0 {
		t.Errorf("expected Frequency=100.0, got %f", coreEvent.Frequency)
	}
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

	if coreEvent.FpsAverage != 50.5 {
		t.Errorf("expected FpsAverage=50.5, got %f", coreEvent.FpsAverage)
	}
	if coreEvent.FpsMin != 30.0 {
		t.Errorf("expected FpsMin=30.0, got %f", coreEvent.FpsMin)
	}
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

	if coreState.MissionID != 1 {
		t.Errorf("expected MissionID=1, got %d", coreState.MissionID)
	}
	if coreState.CaptureFrame != 100 {
		t.Errorf("expected CaptureFrame=100, got %d", coreState.CaptureFrame)
	}
	if coreState.SystemTimeUTC != "2024-01-15T14:30:45.123" {
		t.Errorf("expected SystemTimeUTC=2024-01-15T14:30:45.123, got %s", coreState.SystemTimeUTC)
	}
	if coreState.MissionDate != "2035-06-15T06:00:00" {
		t.Errorf("expected MissionDate=2035-06-15T06:00:00, got %s", coreState.MissionDate)
	}
	if coreState.TimeMultiplier != 2.0 {
		t.Errorf("expected TimeMultiplier=2.0, got %f", coreState.TimeMultiplier)
	}
	if coreState.MissionTime != 3600.5 {
		t.Errorf("expected MissionTime=3600.5, got %f", coreState.MissionTime)
	}
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

	if coreEvent.Reason != "BLOODLOSS" {
		t.Errorf("expected Reason=BLOODLOSS, got %s", coreEvent.Reason)
	}
	if coreEvent.LastDamageSourceID == nil || *coreEvent.LastDamageSourceID != 10 {
		t.Error("expected LastDamageSourceID=10")
	}
}

func TestAce3UnconsciousEventToCore(t *testing.T) {
	now := time.Now()

	gormEvent := model.Ace3UnconsciousEvent{
		ID:            1,
		MissionID:     1,
		SoldierObjectID: 5,
		Time:          now,
		CaptureFrame:  100,
		IsAwake:       false,
	}

	coreEvent := Ace3UnconsciousEventToCore(gormEvent)

	if coreEvent.SoldierID != 5 {
		t.Errorf("expected SoldierID=5, got %d", coreEvent.SoldierID)
	}
	if coreEvent.IsAwake {
		t.Error("expected IsAwake=false")
	}
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

	if coreState.MarkerID != 5 {
		t.Errorf("expected MarkerID=5, got %d", coreState.MarkerID)
	}
	if coreState.Position.X != 150.0 {
		t.Errorf("expected Position.X=150.0, got %f", coreState.Position.X)
	}
	if coreState.Alpha != 0.5 {
		t.Errorf("expected Alpha=0.5, got %f", coreState.Alpha)
	}
}

// Test with nil/invalid values
func TestSoldierStateToCore_NilInVehicleID(t *testing.T) {
	gormState := model.SoldierState{
		InVehicleObjectID: sql.NullInt32{Valid: false},
	}

	coreState := SoldierStateToCore(gormState)

	if coreState.InVehicleObjectID != nil {
		t.Error("expected InVehicleObjectID=nil")
	}
}

func TestHitEventToCore_NilIDs(t *testing.T) {
	gormEvent := model.HitEvent{
		VictimSoldierObjectID:  sql.NullInt32{Valid: false},
		ShooterSoldierObjectID: sql.NullInt32{Valid: false},
	}

	coreEvent := HitEventToCore(gormEvent)

	if coreEvent.VictimSoldierID != nil {
		t.Error("expected VictimSoldierID=nil")
	}
	if coreEvent.ShooterSoldierID != nil {
		t.Error("expected ShooterSoldierID=nil")
	}
}

// Compile-time interface checks
var (
	_ core.Soldier              = SoldierToCore(model.Soldier{})
	_ core.SoldierState         = SoldierStateToCore(model.SoldierState{})
	_ core.Vehicle              = VehicleToCore(model.Vehicle{})
	_ core.VehicleState         = VehicleStateToCore(model.VehicleState{})
	_ core.FiredEvent           = FiredEventToCore(model.FiredEvent{})
	_ core.GeneralEvent         = GeneralEventToCore(model.GeneralEvent{})
	_ core.HitEvent             = HitEventToCore(model.HitEvent{})
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
