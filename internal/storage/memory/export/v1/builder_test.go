package v1

import (
	"testing"

	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoolToInt(t *testing.T) {
	assert.Equal(t, 1, boolToInt(true))
	assert.Equal(t, 0, boolToInt(false))
}

func TestSideToIndex(t *testing.T) {
	tests := []struct {
		side     string
		expected int
	}{
		{"EAST", 0},
		{"east", 0},
		{"OPFOR", 0},
		{"opfor", 0},
		{"WEST", 1},
		{"west", 1},
		{"BLUFOR", 1},
		{"blufor", 1},
		{"GUER", 2},
		{"guer", 2},
		{"INDEPENDENT", 2},
		{"independent", 2},
		{"CIV", 3},
		{"civ", 3},
		{"CIVILIAN", 3},
		{"civilian", 3},
		{"EMPTY", -1},
		{"LOGIC", -1},
		{"UNKNOWN", -1},
		{"", -1},
		{"GLOBAL", -1},
	}

	for _, tt := range tests {
		t.Run(tt.side, func(t *testing.T) {
			assert.Equal(t, tt.expected, sideToIndex(tt.side))
		})
	}
}

func TestParseMarkerSize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []float64
	}{
		{"valid size", "[2.5,3.0]", []float64{2.5, 3.0}},
		{"integer size", "[1,2]", []float64{1, 2}},
		{"empty string", "", []float64{1.0, 1.0}},
		{"invalid json", "[1,2,3", []float64{1.0, 1.0}},
		{"wrong length", "[1]", []float64{1.0, 1.0}},
		{"too many elements", "[1,2,3]", []float64{1.0, 1.0}},
		{"not an array", "1.5", []float64{1.0, 1.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseMarkerSize(tt.input))
		})
	}
}

func TestBuildEmptyMission(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Empty", Author: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
	}

	export := Build(data)

	assert.Equal(t, "Empty", export.MissionName)
	assert.Equal(t, "Test", export.MissionAuthor)
	assert.Equal(t, "Altis", export.WorldName)
	assert.Empty(t, export.Entities)
	assert.Empty(t, export.Events)
	assert.Empty(t, export.Markers)
	assert.Empty(t, export.Times)
	assert.Equal(t, uint(0), export.EndFrame)
}

func TestBuildWithMissionMetadata(t *testing.T) {
	data := &MissionData{
		Mission: &core.Mission{
			MissionName:      "Test Mission",
			Author:           "Test Author",
			AddonVersion:     "1.0.0",
			ExtensionVersion: "2.0.0",
			ExtensionBuild:   "Build 123",
			CaptureDelay:     1.5,
			Tag:              "TvT",
		},
		World:    &core.World{WorldName: "Tanoa"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
	}

	export := Build(data)

	assert.Equal(t, "Test Mission", export.MissionName)
	assert.Equal(t, "Test Author", export.MissionAuthor)
	assert.Equal(t, "Tanoa", export.WorldName)
	assert.Equal(t, "1.0.0", export.AddonVersion)
	assert.Equal(t, "2.0.0", export.ExtensionVersion)
	assert.Equal(t, "Build 123", export.ExtensionBuild)
	assert.Equal(t, float32(1.5), export.CaptureDelay)
	assert.Equal(t, "TvT", export.Tags)
}

func TestBuildWithTimeStates(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		TimeStates: []core.TimeState{
			{CaptureFrame: 0, MissionDate: "2035-06-24", SystemTimeUTC: "2024-01-01T10:00:00", MissionTime: 0, TimeMultiplier: 1.0},
			{CaptureFrame: 100, MissionDate: "2035-06-24", SystemTimeUTC: "2024-01-01T10:01:00", MissionTime: 60, TimeMultiplier: 2.0},
		},
	}

	export := Build(data)

	require.Len(t, export.Times, 2)
	assert.Equal(t, uint(0), export.Times[0].FrameNum)
	assert.Equal(t, "2035-06-24", export.Times[0].Date)
	assert.Equal(t, "2024-01-01T10:00:00", export.Times[0].SystemTimeUTC)
	assert.Equal(t, float32(0), export.Times[0].Time)
	assert.Equal(t, float32(1.0), export.Times[0].TimeMultiplier)

	assert.Equal(t, uint(100), export.Times[1].FrameNum)
	assert.Equal(t, float32(60), export.Times[1].Time)
	assert.Equal(t, float32(2.0), export.Times[1].TimeMultiplier)
}

func TestBuildWithSoldier(t *testing.T) {
	data := &MissionData{
		Mission: &core.Mission{MissionName: "Test"},
		World:   &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			5: {
				Soldier: core.Soldier{
					ID: 5, UnitName: "Player1", GroupID: "Alpha", Side: "WEST", IsPlayer: true, JoinFrame: 10,
				},
				States: []core.SoldierState{
					{SoldierID: 5, CaptureFrame: 10, Position: core.Position3D{X: 1000, Y: 2000}, Bearing: 90, Lifestate: 1, UnitName: "Player1", IsPlayer: true, CurrentRole: "Rifleman"},
					{SoldierID: 5, CaptureFrame: 20, Position: core.Position3D{X: 1100, Y: 2100}, Bearing: 95, Lifestate: 1, UnitName: "Player1", IsPlayer: true, CurrentRole: "Rifleman"},
				},
				FiredEvents: []core.FiredEvent{
					{SoldierID: 5, CaptureFrame: 15, Weapon: "arifle_MX_F", Magazine: "30Rnd_65x39", FiringMode: "Single", StartPos: core.Position3D{X: 1050, Y: 2050}, EndPos: core.Position3D{X: 1200, Y: 2200}},
				},
			},
		},
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
	}

	export := Build(data)

	// Sparse array: entity at index 5
	require.Len(t, export.Entities, 6)
	entity := export.Entities[5]

	assert.Equal(t, uint16(5), entity.ID)
	assert.Equal(t, "Player1", entity.Name)
	assert.Equal(t, "Alpha", entity.Group)
	assert.Equal(t, "WEST", entity.Side)
	assert.Equal(t, 1, entity.IsPlayer)
	assert.Equal(t, "unit", entity.Type)
	assert.Equal(t, uint(10), entity.StartFrameNum)

	// Check positions
	require.Len(t, entity.Positions, 2)
	pos := entity.Positions[0]
	coords := pos[0].([]float64)
	assert.Equal(t, 1000.0, coords[0])
	assert.Equal(t, 2000.0, coords[1])
	assert.Equal(t, uint16(90), pos[1])  // bearing
	assert.Equal(t, uint8(1), pos[2])    // lifestate
	assert.Equal(t, 0, pos[3])           // inVehicleID (nil -> 0)
	assert.Equal(t, "Player1", pos[4])   // unitName
	assert.Equal(t, 1, pos[5])           // isPlayer
	assert.Equal(t, "Rifleman", pos[6])  // currentRole

	// Check fired events - v1 format: [frameNum, [x, y, z]]
	require.Len(t, entity.FramesFired, 1)
	ff := entity.FramesFired[0]
	require.Len(t, ff, 2)
	assert.Equal(t, uint(15), ff[0]) // captureFrame
	endPos := ff[1].([]float64)
	require.Len(t, endPos, 3)
	assert.Equal(t, 1200.0, endPos[0]) // X
	assert.Equal(t, 2200.0, endPos[1]) // Y
	assert.Equal(t, 0.0, endPos[2])    // Z

	// EndFrame should be max state frame
	assert.Equal(t, uint(20), export.EndFrame)
}

func TestBuildWithSoldierInVehicle(t *testing.T) {
	inVehicleID := uint16(100)
	data := &MissionData{
		Mission: &core.Mission{MissionName: "Test"},
		World:   &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			1: {
				Soldier: core.Soldier{ID: 1, UnitName: "Driver"},
				States: []core.SoldierState{
					{SoldierID: 1, CaptureFrame: 0, InVehicleObjectID: &inVehicleID},
				},
			},
		},
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
	}

	export := Build(data)

	require.Len(t, export.Entities, 2)
	pos := export.Entities[1].Positions[0]
	assert.Equal(t, uint16(100), pos[3]) // inVehicleID should be set
}

func TestBuildWithVehicle(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: map[uint16]*VehicleRecord{
			10: {
				Vehicle: core.Vehicle{
					ID: 10, DisplayName: "Hunter", ClassName: "B_MRAP_01_F", OcapType: "car", JoinFrame: 5,
				},
				States: []core.VehicleState{
					{VehicleID: 10, CaptureFrame: 5, Position: core.Position3D{X: 3000, Y: 4000}, Bearing: 180, IsAlive: true, Crew: "[[1,\"driver\"]]"},
					{VehicleID: 10, CaptureFrame: 15, Position: core.Position3D{X: 3100, Y: 4100}, Bearing: 185, IsAlive: true, Crew: "[[1,\"driver\"],[2,\"gunner\"]]"},
				},
			},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	require.Len(t, export.Entities, 11) // indices 0-10
	entity := export.Entities[10]

	assert.Equal(t, uint16(10), entity.ID)
	assert.Equal(t, "Hunter", entity.Name)
	assert.Equal(t, "B_MRAP_01_F", entity.Class)
	assert.Equal(t, "car", entity.Type)
	assert.Equal(t, "UNKNOWN", entity.Side)
	assert.Equal(t, 0, entity.IsPlayer)
	assert.Equal(t, uint(5), entity.StartFrameNum)
	assert.Empty(t, entity.FramesFired)

	// Check positions
	require.Len(t, entity.Positions, 2)
	pos := entity.Positions[0]
	coords := pos[0].([]float64)
	assert.Equal(t, 3000.0, coords[0])
	assert.Equal(t, 4000.0, coords[1])
	assert.Equal(t, uint16(180), pos[1]) // bearing
	assert.Equal(t, 1, pos[2])           // isAlive

	// Crew should be parsed as array
	crew := pos[3].([]any)
	require.Len(t, crew, 1)
	crewEntry := crew[0].([]any)
	assert.Equal(t, float64(1), crewEntry[0])

	assert.Equal(t, uint(15), export.EndFrame)
}

func TestBuildWithVehicleEmptyCrew(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: map[uint16]*VehicleRecord{
			1: {
				Vehicle: core.Vehicle{ID: 1, OcapType: "car"},
				States: []core.VehicleState{
					{VehicleID: 1, CaptureFrame: 0, Crew: ""},
				},
			},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	pos := export.Entities[1].Positions[0]
	crew := pos[3].([]any)
	assert.Empty(t, crew)
}

func TestBuildWithVehicleInvalidCrewJSON(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: map[uint16]*VehicleRecord{
			1: {
				Vehicle: core.Vehicle{ID: 1, OcapType: "car"},
				States: []core.VehicleState{
					{VehicleID: 1, CaptureFrame: 0, Crew: "invalid json"},
				},
			},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	pos := export.Entities[1].Positions[0]
	crew := pos[3].([]any)
	assert.Empty(t, crew) // Falls back to empty array
}

func TestBuildWithDeadVehicle(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: map[uint16]*VehicleRecord{
			1: {
				Vehicle: core.Vehicle{ID: 1, OcapType: "tank"},
				States: []core.VehicleState{
					{VehicleID: 1, CaptureFrame: 50, IsAlive: false},
				},
			},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	pos := export.Entities[1].Positions[0]
	assert.Equal(t, 0, pos[2]) // isAlive = false -> 0
}

func TestBuildWithGeneralEvents(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		GeneralEvents: []core.GeneralEvent{
			{CaptureFrame: 10, Name: "connected", Message: "Player joined"},
			{CaptureFrame: 20, Name: "custom", Message: "[-1,-1,-1,-1]"},      // JSON array
			{CaptureFrame: 30, Name: "data", Message: `{"key":"value"}`},       // JSON object
			{CaptureFrame: 40, Name: "invalid", Message: "[1,2,3"},             // Invalid JSON
		},
	}

	export := Build(data)

	require.Len(t, export.Events, 4)

	// Plain string message
	assert.Equal(t, uint(10), export.Events[0][0])
	assert.Equal(t, "connected", export.Events[0][1])
	assert.Equal(t, "Player joined", export.Events[0][2])

	// JSON array should be parsed
	assert.Equal(t, uint(20), export.Events[1][0])
	parsedArray := export.Events[1][2].([]any)
	assert.Len(t, parsedArray, 4)

	// JSON object should be parsed
	parsedObj := export.Events[2][2].(map[string]any)
	assert.Equal(t, "value", parsedObj["key"])

	// Invalid JSON stays as string
	assert.Equal(t, "[1,2,3", export.Events[3][2])
}

func TestBuildWithHitEvents(t *testing.T) {
	soldierVictim := uint(5)
	soldierShooter := uint(10)
	vehicleVictim := uint(20)
	vehicleShooter := uint(25)

	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		HitEvents: []core.HitEvent{
			// Soldier hits soldier
			{CaptureFrame: 10, VictimSoldierID: &soldierVictim, ShooterSoldierID: &soldierShooter, EventText: "rifle", Distance: 50},
			// Vehicle hits vehicle
			{CaptureFrame: 20, VictimVehicleID: &vehicleVictim, ShooterVehicleID: &vehicleShooter, EventText: "cannon", Distance: 200},
		},
	}

	export := Build(data)

	require.Len(t, export.Events, 2)

	// Soldier hit
	evt1 := export.Events[0]
	assert.Equal(t, uint(10), evt1[0])
	assert.Equal(t, "hit", evt1[1])
	assert.Equal(t, uint(5), evt1[2])  // victimID
	causedBy1 := evt1[3].([]any)
	assert.Equal(t, uint(10), causedBy1[0]) // shooterID
	assert.Equal(t, "rifle", causedBy1[1])
	assert.Equal(t, float32(50), evt1[4])

	// Vehicle hit
	evt2 := export.Events[1]
	assert.Equal(t, uint(20), evt2[2]) // vehicleVictim takes precedence
	causedBy2 := evt2[3].([]any)
	assert.Equal(t, uint(25), causedBy2[0])
}

func TestBuildWithKillEvents(t *testing.T) {
	soldierVictim := uint(5)
	soldierKiller := uint(10)

	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		KillEvents: []core.KillEvent{
			{CaptureFrame: 100, VictimSoldierID: &soldierVictim, KillerSoldierID: &soldierKiller, EventText: "explosion", Distance: 10},
		},
	}

	export := Build(data)

	require.Len(t, export.Events, 1)
	evt := export.Events[0]
	assert.Equal(t, uint(100), evt[0])
	assert.Equal(t, "killed", evt[1])
	assert.Equal(t, uint(5), evt[2])
	causedBy := evt[3].([]any)
	assert.Equal(t, uint(10), causedBy[0])
	assert.Equal(t, "explosion", causedBy[1])
	assert.Equal(t, float32(10), evt[4])
}

func TestBuildWithMarker(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers: map[string]*MarkerRecord{
			"obj_alpha": {
				Marker: core.Marker{
					ID: 1, MarkerName: "obj_alpha", Text: "Objective", MarkerType: "mil_objective",
					Color: "#800000", Side: "WEST", Shape: "ICON", OwnerID: 42, Size: "[2.0,3.0]", Brush: "Solid",
					CaptureFrame: 0, Position: core.Position3D{X: 5000, Y: 6000}, Direction: 45, Alpha: 1.0,
				},
				States: []core.MarkerState{
					{MarkerID: 1, CaptureFrame: 50, Position: core.Position3D{X: 5100, Y: 6100}, Direction: 90, Alpha: 0.8},
				},
			},
		},
	}

	export := Build(data)

	require.Len(t, export.Markers, 1)
	marker := export.Markers[0]

	assert.Equal(t, "mil_objective", marker[0])  // type
	assert.Equal(t, "Objective", marker[1])      // text
	assert.Equal(t, uint(0), marker[2])          // startFrame
	assert.Equal(t, -1, marker[3])               // endFrame
	assert.Equal(t, 42, marker[4])               // playerId
	assert.Equal(t, "800000", marker[5])         // color (# stripped)
	assert.Equal(t, 1, marker[6])                // sideIndex (WEST = 1)

	// Positions
	positions := marker[7].([][]any)
	require.Len(t, positions, 2)
	assert.Equal(t, uint(0), positions[0][0])    // initial frame
	assert.Equal(t, uint(50), positions[1][0])   // state change frame

	assert.Equal(t, []float64{2.0, 3.0}, marker[8]) // size
	assert.Equal(t, "ICON", marker[9])              // shape
	assert.Equal(t, "Solid", marker[10])            // brush
}

func TestBuildWithPolylineMarker(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers: map[string]*MarkerRecord{
			"route_1": {
				Marker: core.Marker{
					MarkerName: "route_1", Shape: "POLYLINE", CaptureFrame: 10,
					Polyline: core.Polyline{
						{X: 100, Y: 200},
						{X: 300, Y: 400},
						{X: 500, Y: 600},
					},
					Direction: 0, Alpha: 1.0,
				},
			},
		},
	}

	export := Build(data)

	require.Len(t, export.Markers, 1)
	marker := export.Markers[0]

	assert.Equal(t, "POLYLINE", marker[9]) // shape

	positions := marker[7].([][]any)
	require.Len(t, positions, 1) // Polylines have single frame entry

	frameEntry := positions[0]
	assert.Equal(t, uint(10), frameEntry[0]) // frameNum

	// Coordinates array
	coords := frameEntry[1].([][]float64)
	require.Len(t, coords, 3)
	assert.Equal(t, []float64{100, 200}, coords[0])
	assert.Equal(t, []float64{300, 400}, coords[1])
	assert.Equal(t, []float64{500, 600}, coords[2])
}

func TestBuildWithNamedColor(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers: map[string]*MarkerRecord{
			"marker1": {
				Marker: core.Marker{MarkerName: "marker1", Color: "ColorRed", Shape: "ICON"},
			},
		},
	}

	export := Build(data)

	marker := export.Markers[0]
	assert.Equal(t, "ColorRed", marker[5]) // Named colors unchanged
}

func TestBuildEntitySparseArray(t *testing.T) {
	// Test that entity array is sparse with correct indexing
	data := &MissionData{
		Mission: &core.Mission{MissionName: "Test"},
		World:   &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			3:  {Soldier: core.Soldier{ID: 3, UnitName: "Soldier3"}},
			7:  {Soldier: core.Soldier{ID: 7, UnitName: "Soldier7"}},
			15: {Soldier: core.Soldier{ID: 15, UnitName: "Soldier15"}},
		},
		Vehicles: map[uint16]*VehicleRecord{
			10: {Vehicle: core.Vehicle{ID: 10, DisplayName: "Vehicle10", OcapType: "car"}},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	// Max ID is 15, so array length should be 16
	require.Len(t, export.Entities, 16)

	// Check entities at their correct indices
	assert.Equal(t, "Soldier3", export.Entities[3].Name)
	assert.Equal(t, "Soldier7", export.Entities[7].Name)
	assert.Equal(t, "Vehicle10", export.Entities[10].Name)
	assert.Equal(t, "Soldier15", export.Entities[15].Name)

	// Placeholder entries should be empty
	assert.Equal(t, "", export.Entities[0].Name)
	assert.Equal(t, "", export.Entities[5].Name)
}

func TestBuildMaxFrameFromMultipleSources(t *testing.T) {
	data := &MissionData{
		Mission: &core.Mission{MissionName: "Test"},
		World:   &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			1: {
				Soldier: core.Soldier{ID: 1},
				States: []core.SoldierState{
					{SoldierID: 1, CaptureFrame: 50},
					{SoldierID: 1, CaptureFrame: 100},
				},
			},
		},
		Vehicles: map[uint16]*VehicleRecord{
			2: {
				Vehicle: core.Vehicle{ID: 2, OcapType: "car"},
				States: []core.VehicleState{
					{VehicleID: 2, CaptureFrame: 75},
					{VehicleID: 2, CaptureFrame: 150}, // Max frame
				},
			},
		},
		Markers: make(map[string]*MarkerRecord),
	}

	export := Build(data)

	assert.Equal(t, uint(150), export.EndFrame)
}

func TestBuildWithNoEntitiesButEvents(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		GeneralEvents: []core.GeneralEvent{
			{CaptureFrame: 10, Name: "test", Message: "msg"},
		},
	}

	export := Build(data)

	assert.Empty(t, export.Entities)
	require.Len(t, export.Events, 1)
}

func TestBuildKillEventWithVehicleIDs(t *testing.T) {
	vehicleVictim := uint(20)
	vehicleKiller := uint(30)

	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		KillEvents: []core.KillEvent{
			{CaptureFrame: 100, VictimVehicleID: &vehicleVictim, KillerVehicleID: &vehicleKiller, EventText: "missile", Distance: 500},
		},
	}

	export := Build(data)

	require.Len(t, export.Events, 1)
	evt := export.Events[0]
	assert.Equal(t, uint(20), evt[2]) // victimID (vehicle)
	causedBy := evt[3].([]any)
	assert.Equal(t, uint(30), causedBy[0]) // killerID (vehicle)
}

func TestBuildHitEventPrioritizesVehicleOverSoldier(t *testing.T) {
	soldierID := uint(5)
	vehicleID := uint(10)

	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		HitEvents: []core.HitEvent{
			// Both soldier and vehicle IDs set - vehicle should take precedence
			{CaptureFrame: 10, VictimSoldierID: &soldierID, VictimVehicleID: &vehicleID, ShooterSoldierID: &soldierID, ShooterVehicleID: &vehicleID, EventText: "test", Distance: 10},
		},
	}

	export := Build(data)

	evt := export.Events[0]
	assert.Equal(t, uint(10), evt[2]) // Vehicle victim ID takes precedence
	causedBy := evt[3].([]any)
	assert.Equal(t, uint(10), causedBy[0]) // Vehicle shooter ID takes precedence
}
