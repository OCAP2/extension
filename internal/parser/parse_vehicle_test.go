package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVehicleState_CrewPreservesBrackets(t *testing.T) {
	p := newTestParser()

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

func TestParseVehicleState_FloatOcapId(t *testing.T) {
	p := newTestParser()

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
