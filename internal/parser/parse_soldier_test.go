package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Equal(t, uint16(30), state.SoldierObjectID)
}
