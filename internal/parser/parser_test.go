package parser

import (
	"log/slog"
	"testing"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestParser() *Parser {
	p := NewParser(slog.Default(), "1.0.0", "2.0.0")
	return p
}

func TestNewParser(t *testing.T) {
	p := newTestParser()
	require.NotNil(t, p)
}

func TestParseMission(t *testing.T) {
	p := newTestParser()

	worldData := `{"worldName":"Altis","displayName":"Altis","worldSize":30720,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"Test Mission",
		"missionNameSource":"file",
		"briefingName":"Test Briefing",
		"serverName":"Test Server",
		"serverProfile":"TestProfile",
		"onLoadName":"Loading Test",
		"author":"Tester",
		"tag":"TvT",
		"captureDelay":1.0,
		"addons":[["addon1","12345"],["addon2",67890]],
		"playableSlots":[10,10,5,0,2],
		"sideFriendly":[false,false,true]
	}`

	mission, world, err := p.ParseMission([]string{worldData, missionData})

	require.NoError(t, err)
	assert.Equal(t, "Test Mission", mission.MissionName)
	assert.Equal(t, "Altis", world.WorldName)
	assert.Len(t, mission.Addons, 2)
	assert.Equal(t, "1.0.0", mission.AddonVersion)
	assert.Equal(t, "2.0.0", mission.ExtensionVersion)
}

func TestParseMission_EmptyAddons(t *testing.T) {
	p := newTestParser()

	worldData := `{"worldName":"Stratis","displayName":"Stratis","worldSize":8192,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"No Addons",
		"missionNameSource":"file",
		"briefingName":"Test",
		"serverName":"Server",
		"serverProfile":"Profile",
		"onLoadName":"Loading",
		"author":"Author",
		"tag":"Coop",
		"captureDelay":0.5,
		"addons":[],
		"playableSlots":[5,5,0,0,0],
		"sideFriendly":[false,false,false]
	}`

	mission, _, err := p.ParseMission([]string{worldData, missionData})
	require.NoError(t, err)
	assert.Equal(t, "No Addons", mission.MissionName)
	assert.Len(t, mission.Addons, 0)
}

func TestParseMission_AddonsWithMixedTypes(t *testing.T) {
	p := newTestParser()

	worldData := `{"worldName":"Tanoa","displayName":"Tanoa","worldSize":15360,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"Addon Test",
		"missionNameSource":"file",
		"briefingName":"Test",
		"serverName":"Server",
		"serverProfile":"Profile",
		"onLoadName":"Loading",
		"author":"Author",
		"tag":"TvT",
		"captureDelay":1.0,
		"addons":[["CBA_A3","450814997"],["ACE3",463939057],["TFAR","894678801"]],
		"playableSlots":[10,10,0,0,0],
		"sideFriendly":[false,false,false]
	}`

	mission, _, err := p.ParseMission([]string{worldData, missionData})
	require.NoError(t, err)
	assert.Len(t, mission.Addons, 3)
}

func TestParseVehicleState_CrewPreservesBrackets(t *testing.T) {
	p := newTestParser()

	// Set up mission context
	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	tests := []struct {
		name         string
		crewInput    string
		expectedCrew string
	}{
		{"multi-crew array", "[20,21]", "[20,21]"},
		{"single-crew array", "[108]", "[108]"},
		{"empty crew array", "[]", "[]"},
		{"large crew", "[1,2,3,4,5]", "[1,2,3,4,5]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []string{
				"10",                 // 0: ocapId
				"[100.0,200.0,50.0]", // 1: position
				"90",                 // 2: bearing
				"true",               // 3: alive
				tt.crewInput,         // 4: crew
				"5",                  // 5: frame
				"0.85",              // 6: fuel
				"0.0",               // 7: damage
				"true",              // 8: engineOn
				"false",             // 9: locked
				"EAST",              // 10: side
				"[0,1,0]",           // 11: vectorDir
				"[0,0,1]",           // 12: vectorUp
				"45.0",              // 13: turretAzimuth
				"10.0",              // 14: turretElevation
			}

			state, err := p.ParseVehicleState(data)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCrew, state.Crew)
			assert.Equal(t, uint16(10), state.VehicleObjectID)
		})
	}
}

func TestParseSoldierState_ScoresParsingPanic(t *testing.T) {
	p := newTestParser()

	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	makeData := func(isPlayer string, scores string) []string {
		return []string{
			"42",                 // 0: ocapId
			"[100.0,200.0,50.0]", // 1: position
			"90",                 // 2: bearing
			"1",                  // 3: lifestate
			"false",              // 4: inVehicle
			"TestUnit",           // 5: name
			isPlayer,             // 6: isPlayer
			"rifleman",           // 7: currentRole
			"10",                 // 8: frame
			"true",               // 9: hasStableVitals
			"false",              // 10: isDraggedCarried
			scores,               // 11: scores
			"",                   // 12: vehicleRole
			"-1",                 // 13: inVehicleID
			"STAND",              // 14: stance
		}
	}

	tests := []struct {
		name         string
		isPlayer     string
		scores       string
		expectScores model.SoldierScores
	}{
		{
			name:     "valid 6 scores",
			isPlayer: "true",
			scores:   "1,2,3,4,5,100",
			expectScores: model.SoldierScores{
				InfantryKills: 1, VehicleKills: 2, ArmorKills: 3,
				AirKills: 4, Deaths: 5, TotalScore: 100,
			},
		},
		{
			name:         "single score value should not panic",
			isPlayer:     "true",
			scores:       "0",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "empty scores string should not panic",
			isPlayer:     "true",
			scores:       "",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "partial scores should not panic",
			isPlayer:     "true",
			scores:       "1,2,3",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "non-player ignores scores",
			isPlayer:     "false",
			scores:       "garbage",
			expectScores: model.SoldierScores{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeData(tt.isPlayer, tt.scores)

			state, err := p.ParseSoldierState(data)
			require.NoError(t, err)
			assert.Equal(t, tt.expectScores, state.Scores)
			assert.Equal(t, uint16(42), state.SoldierObjectID)
		})
	}
}

func TestParseUintFromFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"integer", "32", 32, false},
		{"zero", "0", 0, false},
		{"float with decimals", "32.00", 32, false},
		{"float with trailing zero", "30.0", 30, false},
		{"large integer", "65535", 65535, false},
		{"large float", "65535.00", 65535, false},
		{"fractional rejects", "10.99", 0, true},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"negative", "-1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUintFromFloat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseIntFromFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"integer", "32", 32, false},
		{"zero", "0", 0, false},
		{"negative integer", "-1", -1, false},
		{"float with decimals", "32.00", 32, false},
		{"negative float", "-1.00", -1, false},
		{"large integer", "65535", 65535, false},
		{"fractional rejects", "10.99", 0, true},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIntFromFloat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseSoldierState_GroupAndSide(t *testing.T) {
	p := newTestParser()

	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	baseData := []string{
		"42",                  // 0: ocapId
		"[100.0,200.0,50.0]", // 1: position
		"90",                  // 2: bearing
		"1",                   // 3: lifestate
		"false",               // 4: inVehicle
		"TestUnit",            // 5: name
		"false",               // 6: isPlayer
		"rifleman",            // 7: currentRole
		"10",                  // 8: frame
		"true",                // 9: hasStableVitals
		"false",               // 10: isDraggedCarried
		"",                    // 11: scores
		"",                    // 12: vehicleRole
		"-1",                  // 13: inVehicleID
		"STAND",               // 14: stance
	}

	t.Run("17 fields parses group and side", func(t *testing.T) {
		data := append(append([]string{}, baseData...), "Alpha 1-1", "EAST")
		state, err := p.ParseSoldierState(data)
		require.NoError(t, err)
		assert.Equal(t, "Alpha 1-1", state.GroupID)
		assert.Equal(t, "EAST", state.Side)
	})

	t.Run("15 fields leaves group and side empty for worker to fill", func(t *testing.T) {
		state, err := p.ParseSoldierState(append([]string{}, baseData...))
		require.NoError(t, err)
		assert.Equal(t, "", state.GroupID)
		assert.Equal(t, "", state.Side)
	})
}

func TestParseVehicleState_FloatOcapId(t *testing.T) {
	p := newTestParser()

	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	data := []string{
		"32.00",              // 0: ocapId (float format from ArmA)
		"[100.0,200.0,50.0]", // 1: position
		"90",                 // 2: bearing
		"true",               // 3: alive
		"[]",                 // 4: crew
		"5",                  // 5: frame
		"0.85",               // 6: fuel
		"0.0",                // 7: damage
		"true",               // 8: engineOn
		"false",              // 9: locked
		"CIV",                // 10: side
		"[0,1,0]",            // 11: vectorDir
		"[0,0,1]",            // 12: vectorUp
		"0",                  // 13: turretAzimuth
		"0",                  // 14: turretElevation
	}

	state, err := p.ParseVehicleState(data)
	require.NoError(t, err)
	assert.Equal(t, uint16(32), state.VehicleObjectID)
}

func TestParseSoldierState_FloatOcapId(t *testing.T) {
	p := newTestParser()

	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	data := []string{
		"30.00",              // 0: ocapId (float format from ArmA)
		"[100.0,200.0,50.0]", // 1: position
		"90",                 // 2: bearing
		"1",                  // 3: lifestate
		"false",              // 4: inVehicle
		"TestUnit",           // 5: name
		"false",              // 6: isPlayer
		"rifleman",           // 7: currentRole
		"10",                 // 8: frame
		"true",               // 9: hasStableVitals
		"false",              // 10: isDraggedCarried
		"0,0,0,0,0,0",       // 11: scores
		"",                   // 12: vehicleRole
		"-1",                 // 13: inVehicleID
		"STAND",              // 14: stance
	}

	state, err := p.ParseSoldierState(data)
	require.NoError(t, err)
	assert.Equal(t, uint16(30), state.SoldierObjectID)
}

func TestParseKillEvent_WeaponParsing(t *testing.T) {
	p := newTestParser()

	mission := &model.Mission{}
	mission.ID = 1
	p.SetMission(mission)

	tests := []struct {
		name        string
		weaponArg   string
		wantVehicle string
		wantWeapon  string
		wantMag     string
		wantText    string
	}{
		{
			name:        "suicide - all empty strings",
			weaponArg:   `["","",""]`,
			wantVehicle: "", wantWeapon: "", wantMag: "",
			wantText: "",
		},
		{
			name:        "on-foot kill with weapon",
			weaponArg:   `["","MX 6.5 mm","6.5 mm 30Rnd Sand Mag"]`,
			wantVehicle: "", wantWeapon: "MX 6.5 mm", wantMag: "6.5 mm 30Rnd Sand Mag",
			wantText: "MX 6.5 mm [6.5 mm 30Rnd Sand Mag]",
		},
		{
			name:        "vehicle turret kill",
			weaponArg:   `["Hunter HMG","Mk30 HMG .50",".50 BMG 200Rnd"]`,
			wantVehicle: "Hunter HMG", wantWeapon: "Mk30 HMG .50", wantMag: ".50 BMG 200Rnd",
			wantText: "Hunter HMG: Mk30 HMG .50 [.50 BMG 200Rnd]",
		},
		{
			name:        "explosive - no magazine",
			weaponArg:   `["","M6 SLAM Mine",""]`,
			wantVehicle: "", wantWeapon: "M6 SLAM Mine", wantMag: "",
			wantText: "M6 SLAM Mine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []string{
				"100",        // 0: frame
				"5",          // 1: victimID
				"5",          // 2: killerID
				tt.weaponArg, // 3: weapon array
				"0",          // 4: distance
			}

			result, err := p.ParseKillEvent(data)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVehicle, result.Event.WeaponVehicle)
			assert.Equal(t, tt.wantWeapon, result.Event.WeaponName)
			assert.Equal(t, tt.wantMag, result.Event.WeaponMagazine)
			assert.Equal(t, tt.wantText, result.Event.EventText)
			assert.Equal(t, uint16(5), result.VictimID)
			assert.Equal(t, uint16(5), result.KillerID)
		})
	}
}

func TestMissionContext_ThreadSafe(t *testing.T) {
	ctx := NewMissionContext()

	mission := ctx.GetMission()
	assert.Equal(t, "No mission loaded", mission.MissionName)

	world := ctx.GetWorld()
	assert.Equal(t, "No world loaded", world.WorldName)
}
