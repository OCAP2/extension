package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVehicle(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, v core.Vehicle)
		wantErr bool
	}{
		{
			name: "helicopter",
			input: []string{
				"0",                         // 0: frame
				"30",                        // 1: ocapID
				"heli",                      // 2: ocapType
				"UH-80 Ghost Hawk",          // 3: displayName
				"B_Heli_Transport_01_F",     // 4: className
				`[["Black",1],[]]`,          // 5: customization
			},
			check: func(t *testing.T, v core.Vehicle) {
				assert.Equal(t, uint(0), v.JoinFrame)
				assert.Equal(t, uint16(30), v.ID)
				assert.Equal(t, "heli", v.OcapType)
				assert.Equal(t, "UH-80 Ghost Hawk", v.DisplayName)
				assert.Equal(t, "B_Heli_Transport_01_F", v.ClassName)
				assert.Equal(t, `[["Black",1],[]]`, v.Customization)
			},
		},
		{
			name: "APC",
			input: []string{
				"0",                          // 0: frame
				"33",                         // 1: ocapID
				"apc",                        // 2: ocapType
				"MSE-3 Marid",                // 3: displayName
				"O_APC_Wheeled_02_rcws_F",    // 4: className
				`[["Hex",1],[]]`,             // 5: customization
			},
			check: func(t *testing.T, v core.Vehicle) {
				assert.Equal(t, uint16(33), v.ID)
				assert.Equal(t, "apc", v.OcapType)
				assert.Equal(t, "MSE-3 Marid", v.DisplayName)
			},
		},
		{
			name: "car",
			input: []string{
				"0",                     // 0: frame
				"7",                     // 1: ocapID
				"car",                   // 2: ocapType
				"Hatchback",             // 3: displayName
				"C_Hatchback_01_F",      // 4: className
				`[["Yellow",1],[]]`,     // 5: customization
			},
			check: func(t *testing.T, v core.Vehicle) {
				assert.Equal(t, uint16(7), v.ID)
				assert.Equal(t, "car", v.OcapType)
			},
		},
		{
			name: "float IDs",
			input: []string{
				"10.00", "50.00", "boat", "Speedboat", "C_Boat_Civil_01_F", "[]",
			},
			check: func(t *testing.T, v core.Vehicle) {
				assert.Equal(t, uint(10), v.JoinFrame)
				assert.Equal(t, uint16(50), v.ID)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "0", "car", "Name", "Class", "[]"},
			wantErr: true,
		},
		{
			name:    "error: bad objectID",
			input:   []string{"0", "abc", "car", "Name", "Class", "[]"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseVehicle(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

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
			assert.Equal(t, uint16(10), state.VehicleID)
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
	assert.Equal(t, uint16(32), state.VehicleID)
}

func TestParseVehicleState_AllFields(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, v core.VehicleState)
		wantErr bool
	}{
		{
			name: "active with crew",
			input: []string{
				"10",                 // 0: ocapId
				"[5000.0,4000.0,100.0]", // 1: position
				"270",                // 2: bearing
				"true",               // 3: alive
				"[20,21,22]",         // 4: crew
				"500",                // 5: frame
				"0.95",               // 6: fuel
				"0.0",                // 7: damage
				"true",               // 8: engineOn
				"false",              // 9: locked
				"WEST",               // 10: side
				"[0,0.99,0.1]",       // 11: vectorDir
				"[0,-0.1,0.99]",      // 12: vectorUp
				"90.5",               // 13: turretAzimuth
				"-5.2",               // 14: turretElevation
			},
			check: func(t *testing.T, v core.VehicleState) {
				assert.Equal(t, uint16(10), v.VehicleID)
				assert.Equal(t, uint(500), v.CaptureFrame)
				assert.True(t, v.IsAlive)
				assert.Equal(t, "[20,21,22]", v.Crew)
				assert.InDelta(t, float32(0.95), v.Fuel, 0.01)
				assert.Equal(t, float32(0), v.Damage)
				assert.True(t, v.EngineOn)
				assert.False(t, v.Locked)
				assert.Equal(t, "WEST", v.Side)
				assert.Equal(t, "[0,0.99,0.1]", v.VectorDir)
				assert.Equal(t, "[0,-0.1,0.99]", v.VectorUp)
				assert.InDelta(t, float32(90.5), v.TurretAzimuth, 0.01)
				assert.InDelta(t, float32(-5.2), v.TurretElevation, 0.01)
				assert.Equal(t, uint16(270), v.Bearing)
				assert.NotEqual(t, core.Position3D{}, v.Position)
			},
		},
		{
			name: "destroyed vehicle",
			input: []string{
				"33",                  // 0: ocapId
				"[3000.0,2000.0,0.0]", // 1: position
				"0",                   // 2: bearing
				"false",               // 3: alive (destroyed)
				"[]",                  // 4: crew (empty)
				"1000",                // 5: frame
				"0",                   // 6: fuel
				"1.0",                 // 7: damage
				"false",               // 8: engineOn
				"false",               // 9: locked
				"EAST",                // 10: side
				"[0,1,0]",             // 11: vectorDir
				"[0,0,1]",             // 12: vectorUp
				"0",                   // 13: turretAzimuth
				"0",                   // 14: turretElevation
			},
			check: func(t *testing.T, v core.VehicleState) {
				assert.False(t, v.IsAlive)
				assert.Equal(t, float32(0), v.Fuel)
				assert.Equal(t, float32(1.0), v.Damage)
				assert.False(t, v.EngineOn)
			},
		},
		{
			name: "in-flight helicopter",
			input: []string{
				"30",                       // 0: ocapId
				"[6000.0,7000.0,500.0]",    // 1: position (high altitude)
				"45",                       // 2: bearing
				"true",                     // 3: alive
				"[10,11]",                  // 4: crew
				"200",                      // 5: frame
				"0.72",                     // 6: fuel
				"0.1",                      // 7: damage
				"true",                     // 8: engineOn
				"true",                     // 9: locked
				"WEST",                     // 10: side
				"[0.1,0.9,0.2]",            // 11: vectorDir
				"[-0.2,0.1,0.9]",           // 12: vectorUp
				"180.0",                    // 13: turretAzimuth
				"-30.0",                    // 14: turretElevation
			},
			check: func(t *testing.T, v core.VehicleState) {
				assert.True(t, v.IsAlive)
				assert.True(t, v.Locked)
				assert.Equal(t, uint16(45), v.Bearing)
				assert.InDelta(t, float32(0.72), v.Fuel, 0.01)
				assert.InDelta(t, float32(0.1), v.Damage, 0.01)
				assert.InDelta(t, float32(180.0), v.TurretAzimuth, 0.01)
				assert.InDelta(t, float32(-30.0), v.TurretElevation, 0.01)
				assert.True(t, v.Position.Z > 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseVehicleState(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseVehicleState_Errors(t *testing.T) {
	p := newTestParser()

	// base valid data, we'll override specific fields for error cases
	base := func() []string {
		return []string{
			"10",                 // 0: ocapId
			"[100.0,200.0,50.0]", // 1: position
			"90",                 // 2: bearing
			"true",               // 3: alive
			"[]",                 // 4: crew
			"5",                  // 5: frame
			"0.85",               // 6: fuel
			"0.0",                // 7: damage
			"true",               // 8: engineOn
			"false",              // 9: locked
			"EAST",               // 10: side
			"[0,1,0]",            // 11: vectorDir
			"[0,0,1]",            // 12: vectorUp
			"45.0",               // 13: turretAzimuth
			"10.0",               // 14: turretElevation
		}
	}

	tests := []struct {
		name  string
		tweak func(d []string) []string
	}{
		{"bad frame", func(d []string) []string { d[5] = "abc"; return d }},
		{"bad ocapID", func(d []string) []string { d[0] = "abc"; return d }},
		{"bad position", func(d []string) []string { d[1] = "not_a_position"; return d }},
		{"bad fuel", func(d []string) []string { d[6] = "abc"; return d }},
		{"bad damage", func(d []string) []string { d[7] = "abc"; return d }},
		{"bad engineOn", func(d []string) []string { d[8] = "maybe"; return d }},
		{"bad locked", func(d []string) []string { d[9] = "maybe"; return d }},
		{"bad turretAzimuth", func(d []string) []string { d[13] = "abc"; return d }},
		{"bad turretElevation", func(d []string) []string { d[14] = "abc"; return d }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.tweak(base())
			_, err := p.ParseVehicleState(data)
			assert.Error(t, err)
		})
	}
}
