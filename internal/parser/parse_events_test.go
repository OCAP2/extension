package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKillEvent_WeaponParsing(t *testing.T) {
	p := newTestParser()

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
