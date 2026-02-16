package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSoldier(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, s core.Soldier)
		wantErr bool
	}{
		{
			name: "AI soldier EAST",
			input: []string{
				"0",              // 0: frame
				"0",              // 1: ocapID
				"Farid Amin",     // 2: unitName
				"Alpha 1-1",      // 3: groupID
				"EAST",           // 4: side
				"0",              // 5: isPlayer (false)
				"",               // 6: roleDescription
				"O_Soldier_F",    // 7: className
				"Rifleman",       // 8: displayName
				"",               // 9: playerUID
				"[]",             // 10: squadParams
			},
			check: func(t *testing.T, s core.Soldier) {
				assert.Equal(t, uint(0), s.JoinFrame)
				assert.Equal(t, uint16(0), s.ID)
				assert.Equal(t, "Farid Amin", s.UnitName)
				assert.Equal(t, "Alpha 1-1", s.GroupID)
				assert.Equal(t, "EAST", s.Side)
				assert.False(t, s.IsPlayer)
				assert.Equal(t, "", s.RoleDescription)
				assert.Equal(t, "O_Soldier_F", s.ClassName)
				assert.Equal(t, "Rifleman", s.DisplayName)
				assert.Equal(t, "", s.PlayerUID)
				assert.Empty(t, s.SquadParams)
			},
		},
		{
			name: "player WEST with squad params",
			input: []string{
				"0",                          // 0: frame
				"1",                          // 1: ocapID
				"info",                       // 2: unitName
				"Alpha 1-1",                  // 3: groupID
				"WEST",                       // 4: side
				"1",                          // 5: isPlayer (true)
				"",                           // 6: roleDescription
				"B_Soldier_F",                // 7: className
				"Rifleman",                   // 8: displayName
				"76561198000074241",           // 9: playerUID
				`[["SquadName",1],["Tag",2]]`, // 10: squadParams
			},
			check: func(t *testing.T, s core.Soldier) {
				assert.True(t, s.IsPlayer)
				assert.Equal(t, "76561198000074241", s.PlayerUID)
				assert.Equal(t, "WEST", s.Side)
				assert.Len(t, s.SquadParams, 2)
			},
		},
		{
			name: "wildlife UNKNOWN",
			input: []string{
				"415",            // 0: frame
				"78",             // 1: ocapID
				"",               // 2: unitName (empty)
				"",               // 3: groupID
				"UNKNOWN",        // 4: side
				"0",              // 5: isPlayer
				"",               // 6: roleDescription
				"Snake_random_F", // 7: className
				"Snake",          // 8: displayName
				"",               // 9: playerUID
				"[]",             // 10: squadParams
			},
			check: func(t *testing.T, s core.Soldier) {
				assert.Equal(t, uint(415), s.JoinFrame)
				assert.Equal(t, uint16(78), s.ID)
				assert.Equal(t, "", s.UnitName)
				assert.Equal(t, "UNKNOWN", s.Side)
			},
		},
		{
			name: "late spawn EAST with float IDs",
			input: []string{
				"21.00",                  // 0: frame (float)
				"70.00",                  // 1: ocapID (float)
				"Abdul-Qadir Ajani",      // 2: unitName
				"Alpha 1-2",              // 3: groupID
				"EAST",                   // 4: side
				"0",                      // 5: isPlayer
				"",                       // 6: roleDescription
				"O_Soldier_TL_F",         // 7: className
				"Team Leader",            // 8: displayName
				"",                       // 9: playerUID
				"[]",                     // 10: squadParams
			},
			check: func(t *testing.T, s core.Soldier) {
				assert.Equal(t, uint(21), s.JoinFrame)
				assert.Equal(t, uint16(70), s.ID)
				assert.Equal(t, "Team Leader", s.DisplayName)
			},
		},
		{
			name: "error: bad frame",
			input: []string{
				"abc", "0", "Name", "Group", "EAST", "0", "", "Class", "Display", "", "[]",
			},
			wantErr: true,
		},
		{
			name: "error: bad objectID",
			input: []string{
				"0", "abc", "Name", "Group", "EAST", "0", "", "Class", "Display", "", "[]",
			},
			wantErr: true,
		},
		{
			name: "error: bad isPlayer",
			input: []string{
				"0", "0", "Name", "Group", "EAST", "maybe", "", "Class", "Display", "", "[]",
			},
			wantErr: true,
		},
		{
			name: "error: bad squadParams JSON",
			input: []string{
				"0", "0", "Name", "Group", "EAST", "0", "", "Class", "Display", "", "not_json",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseSoldier(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseSoldierState_ScoresParsingPanic(t *testing.T) {
	p := newTestParser()

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
		expectScores core.SoldierScores
	}{
		{
			name:     "valid 6 scores",
			isPlayer: "true",
			scores:   "1,2,3,4,5,100",
			expectScores: core.SoldierScores{
				InfantryKills: 1, VehicleKills: 2, ArmorKills: 3,
				AirKills: 4, Deaths: 5, TotalScore: 100,
			},
		},
		{
			name:         "single score value should not panic",
			isPlayer:     "true",
			scores:       "0",
			expectScores: core.SoldierScores{},
		},
		{
			name:         "empty scores string should not panic",
			isPlayer:     "true",
			scores:       "",
			expectScores: core.SoldierScores{},
		},
		{
			name:         "partial scores should not panic",
			isPlayer:     "true",
			scores:       "1,2,3",
			expectScores: core.SoldierScores{},
		},
		{
			name:         "non-player ignores scores",
			isPlayer:     "false",
			scores:       "garbage",
			expectScores: core.SoldierScores{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeData(tt.isPlayer, tt.scores)

			state, err := p.ParseSoldierState(data)
			require.NoError(t, err)
			assert.Equal(t, tt.expectScores, state.Scores)
			assert.Equal(t, uint16(42), state.SoldierID)
		})
	}
}

func TestParseSoldierState_GroupAndSide(t *testing.T) {
	p := newTestParser()

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

func TestParseSoldierState_FloatOcapId(t *testing.T) {
	p := newTestParser()

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
	assert.Equal(t, uint16(30), state.SoldierID)
}

func TestParseSoldierState_InVehicle(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, s core.SoldierState)
	}{
		{
			name: "in vehicle as driver",
			input: []string{
				"42",                 // 0: ocapId
				"[100.0,200.0,50.0]", // 1: position
				"180",                // 2: bearing
				"1",                  // 3: lifestate
				"true",               // 4: inVehicle
				"Player1",            // 5: name
				"true",               // 6: isPlayer
				"rifleman",           // 7: currentRole
				"500",                // 8: frame
				"true",               // 9: hasStableVitals
				"false",              // 10: isDraggedCarried
				"2,0,0,0,0,50",       // 11: scores
				"driver",             // 12: vehicleRole
				"30",                 // 13: inVehicleID
				"STAND",              // 14: stance
			},
			check: func(t *testing.T, s core.SoldierState) {
				assert.True(t, s.InVehicle)
				assert.Equal(t, "driver", s.VehicleRole)
				assert.NotNil(t, s.InVehicleObjectID)
				assert.Equal(t, uint16(30), *s.InVehicleObjectID)
				assert.True(t, s.IsPlayer)
				assert.Equal(t, core.SoldierScores{
					InfantryKills: 2, TotalScore: 50,
				}, s.Scores)
			},
		},
		{
			name: "in vehicle as gunner",
			input: []string{
				"10",                 // 0: ocapId
				"[200.0,300.0,60.0]", // 1: position
				"90",                 // 2: bearing
				"1",                  // 3: lifestate
				"true",               // 4: inVehicle
				"Gunner1",            // 5: name
				"false",              // 6: isPlayer
				"gunner",             // 7: currentRole
				"100",                // 8: frame
				"true",               // 9: hasStableVitals
				"false",              // 10: isDraggedCarried
				"",                   // 11: scores
				"gunner",             // 12: vehicleRole
				"30",                 // 13: inVehicleID
				"STAND",              // 14: stance
			},
			check: func(t *testing.T, s core.SoldierState) {
				assert.True(t, s.InVehicle)
				assert.Equal(t, "gunner", s.VehicleRole)
				assert.NotNil(t, s.InVehicleObjectID)
				assert.Equal(t, uint16(30), *s.InVehicleObjectID)
			},
		},
		{
			name: "dead on foot",
			input: []string{
				"10",                 // 0: ocapId
				"[300.0,400.0,0.0]",  // 1: position
				"0",                  // 2: bearing
				"0",                  // 3: lifestate (dead)
				"false",              // 4: inVehicle
				"DeadUnit",           // 5: name
				"false",              // 6: isPlayer
				"rifleman",           // 7: currentRole
				"200",                // 8: frame
				"false",              // 9: hasStableVitals
				"false",              // 10: isDraggedCarried
				"",                   // 11: scores
				"",                   // 12: vehicleRole
				"-1",                 // 13: inVehicleID
				"PRONE",              // 14: stance
			},
			check: func(t *testing.T, s core.SoldierState) {
				assert.Equal(t, uint8(0), s.Lifestate)
				assert.False(t, s.InVehicle)
				assert.Equal(t, "PRONE", s.Stance)
				assert.Nil(t, s.InVehicleObjectID)
				assert.False(t, s.HasStableVitals)
			},
		},
		{
			name: "dragged/carried",
			input: []string{
				"10",                 // 0: ocapId
				"[300.0,400.0,0.0]",  // 1: position
				"0",                  // 2: bearing
				"1",                  // 3: lifestate
				"false",              // 4: inVehicle
				"WoundedUnit",        // 5: name
				"false",              // 6: isPlayer
				"rifleman",           // 7: currentRole
				"300",                // 8: frame
				"false",              // 9: hasStableVitals
				"true",               // 10: isDraggedCarried
				"",                   // 11: scores
				"",                   // 12: vehicleRole
				"-1",                 // 13: inVehicleID
				"PRONE",              // 14: stance
			},
			check: func(t *testing.T, s core.SoldierState) {
				assert.True(t, s.IsDraggedCarried)
				assert.False(t, s.HasStableVitals)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := p.ParseSoldierState(tt.input)
			require.NoError(t, err)
			tt.check(t, state)
		})
	}
}

func TestParseSoldierState_Errors(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name  string
		input []string
	}{
		{
			name: "bad frame",
			input: []string{
				"42", "[100.0,200.0,50.0]", "90", "1", "false", "Unit", "false", "role",
				"abc", // bad frame
				"true", "false", "", "", "-1", "STAND",
			},
		},
		{
			name: "bad ocapID",
			input: []string{
				"abc", // bad ocapID
				"[100.0,200.0,50.0]", "90", "1", "false", "Unit", "false", "role",
				"10", "true", "false", "", "", "-1", "STAND",
			},
		},
		{
			name: "bad position",
			input: []string{
				"42", "not_a_position", "90", "1", "false", "Unit", "false", "role",
				"10", "true", "false", "", "", "-1", "STAND",
			},
		},
		{
			name: "bad isPlayer",
			input: []string{
				"42", "[100.0,200.0,50.0]", "90", "1", "false", "Unit", "maybe", "role",
				"10", "true", "false", "", "", "-1", "STAND",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.ParseSoldierState(tt.input)
			assert.Error(t, err)
		})
	}
}
