package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
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

			assert.Equal(t, tt.wantVehicle, result.WeaponVehicle)
			assert.Equal(t, tt.wantWeapon, result.WeaponName)
			assert.Equal(t, tt.wantMag, result.WeaponMagazine)
			assert.Equal(t, tt.wantText, result.EventText)
			assert.Equal(t, uint16(5), result.VictimID)
			assert.Equal(t, uint16(5), result.KillerID)
		})
	}
}

func TestParseKillEvent_Errors(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name  string
		input []string
	}{
		{
			name:  "bad frame",
			input: []string{"abc", "5", "10", `["","",""]`, "100"},
		},
		{
			name:  "bad victimID",
			input: []string{"100", "abc", "10", `["","",""]`, "100"},
		},
		{
			name:  "bad killerID",
			input: []string{"100", "5", "abc", `["","",""]`, "100"},
		},
		{
			name:  "bad distance",
			input: []string{"100", "5", "10", `["","",""]`, "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.ParseKillEvent(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestParseProjectileEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, r ProjectileEvent)
		wantErr bool
	}{
		{
			name: "on-foot fire no hits (vehicleID=-1)",
			input: []string{
				"100",                  // 0: firedFrame
				"0",                    // 1: firedTime (unused, uses time.Now())
				"5",                    // 2: firerID
				"-1",                   // 3: vehicleID
				"",                     // 4: vehicleRole
				"5",                    // 5: remoteControllerID
				"arifle_MX_F",         // 6: weapon
				"MX 6.5 mm",           // 7: weaponDisplay
				"muzzle_snds_H",       // 8: muzzle
				"Sound Suppressor",    // 9: muzzleDisplay
				"30Rnd_65x39_caseless_mag", // 10: magazine
				"6.5 mm 30Rnd",        // 11: magazineDisplay
				"B_65x39_Caseless",    // 12: ammo
				"FullAuto",            // 13: mode
				`[[0.5,100,"6069.06,5627.81,17.81"],[0.6,101,"6070.0,5628.0,17.0"]]`, // 14: positions
				"[800,0,0]",           // 15: initialVelocity
				"[]",                  // 16: hitParts
				"shotBullet",          // 17: simType
				"false",               // 18: isSub
				"iconBullet",          // 19: magazineIcon
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Equal(t, uint(100), r.CaptureFrame)
				assert.Equal(t, uint16(5), r.FirerObjectID)
				assert.Nil(t, r.VehicleObjectID)
				assert.Equal(t, "MX 6.5 mm", r.WeaponDisplay)
				assert.Equal(t, "shotBullet", r.SimulationType)
				assert.Equal(t, "iconBullet", r.MagazineIcon)
				assert.Empty(t, r.HitParts)
				// Should have trajectory points (2 positions)
				assert.Greater(t, len(r.Trajectory), 0)
			},
		},
		{
			name: "HMG from vehicle turret with soldier hit",
			input: []string{
				"200",              // 0: firedFrame
				"0",                // 1: firedTime
				"10",               // 2: firerID
				"30",               // 3: vehicleID
				"turret",           // 4: vehicleRole
				"10",               // 5: remoteControllerID
				"HMG_127",          // 6: weapon
				"Mk30 HMG .50",    // 7: weaponDisplay
				"HMG_127",          // 8: muzzle
				"Mk30 HMG .50",    // 9: muzzleDisplay
				"200Rnd_127x99_mag_Tracer_Yellow", // 10: magazine
				".50 BMG 200Rnd",   // 11: magazineDisplay
				"B_127x99_Ball",    // 12: ammo
				"FullAuto",         // 13: mode
				`[[0.5,200,"5000.0,4000.0,100.0"],[0.6,201,"5001.0,4001.0,99.0"]]`, // 14: positions
				"[900,0,0]",        // 15: initialVelocity
				`[[42,["body","spine3"],"-200.0,300.0,50.0",205]]`, // 16: hitParts
				"shotBullet",       // 17: simType
				"false",            // 18: isSub
				"iconHMG",          // 19: magazineIcon
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.NotNil(t, r.VehicleObjectID)
				assert.Equal(t, uint16(30), *r.VehicleObjectID)
				assert.Len(t, r.HitParts, 1)
				assert.Equal(t, uint16(42), r.HitParts[0].EntityID)
				assert.Equal(t, uint(205), r.HitParts[0].CaptureFrame)
				assert.NotEqual(t, core.Position3D{}, r.HitParts[0].Position)
			},
		},
		{
			name: "grenade launcher (shotShell) hits vehicle",
			input: []string{
				"300",              // 0: firedFrame
				"0",                // 1: firedTime
				"8",                // 2: firerID
				"-1",               // 3: vehicleID
				"",                 // 4: vehicleRole
				"8",                // 5: remoteControllerID
				"GL_3GL_F",         // 6: weapon
				"3GL",              // 7: weaponDisplay
				"GL_3GL_F",         // 8: muzzle
				"3GL",              // 9: muzzleDisplay
				"1Rnd_HE_Grenade_shell", // 10: magazine
				"HE Grenade",       // 11: magazineDisplay
				"G_40mm_HE",        // 12: ammo
				"Single",           // 13: mode
				`[[0.5,300,"3000.0,2000.0,5.0"],[0.8,301,"3010.0,2010.0,3.0"]]`, // 14: positions
				"[70,0,30]",        // 15: initialVelocity
				`[[55,"hull","3010.0,2010.0,3.0",301]]`, // 16: hitParts - single component string
				"shotShell",        // 17: simType
				"false",            // 18: isSub
				"iconGrenade",      // 19: magazineIcon
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Equal(t, "shotShell", r.SimulationType)
				assert.Len(t, r.HitParts, 1)
				assert.Equal(t, uint16(55), r.HitParts[0].EntityID)
			},
		},
		{
			name: "submunition",
			input: []string{
				"400",              // 0: firedFrame
				"0",                // 1: firedTime
				"12",               // 2: firerID
				"-1",               // 3: vehicleID
				"",                 // 4: vehicleRole
				"12",               // 5: remoteControllerID
				"weapon",           // 6: weapon
				"Weapon",           // 7: weaponDisplay
				"muzzle",           // 8: muzzle
				"Muzzle",           // 9: muzzleDisplay
				"mag",              // 10: magazine
				"Magazine",         // 11: magazineDisplay
				"ammo",             // 12: ammo
				"Single",           // 13: mode
				`[[0.5,400,"1000.0,1000.0,50.0"],[1.0,401,"1010.0,1010.0,0.0"]]`, // 14: positions
				"[100,0,50]",       // 15: initialVelocity
				"[]",               // 16: hitParts
				"shotSubmunitions", // 17: simType
				"true",             // 18: isSub
				"iconSub",          // 19: magazineIcon
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Equal(t, "shotSubmunitions", r.SimulationType)
			},
		},
		{
			name: "single position (one trajectory point)",
			input: []string{
				"500", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,500,"1000.0,1000.0,50.0"]]`, // only 1 position
				"[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// Single position creates one trajectory point
				assert.Len(t, r.Trajectory, 1)
			},
		},
		{
			name: "empty positions array",
			input: []string{
				"500", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[]`, "[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.Trajectory)
			},
		},
		{
			name: "multiple hits same bullet",
			input: []string{
				"600", "0", "5", "-1", "", "5", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,600,"2000.0,3000.0,10.0"],[0.6,601,"2001.0,3001.0,9.0"]]`,
				"[800,0,0]",
				`[[10,["body","spine3"],"2001.0,3001.0,9.0",601],[10,["head"],"2001.0,3001.0,9.0",601]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Len(t, r.HitParts, 2)
				assert.Equal(t, uint16(10), r.HitParts[0].EntityID)
				assert.Equal(t, uint16(10), r.HitParts[1].EntityID)
			},
		},
		{
			name: "position with < 3 elements skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6],[0.7,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// Middle element has < 3 fields, skipped; first and third should create trajectory
				assert.Greater(t, len(r.Trajectory), 0)
			},
		},
		{
			name: "position with non-float frameNo skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,"not_a_number","1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// First position has non-float frameNo, skipped; second is valid
				assert.Len(t, r.Trajectory, 1)
			},
		},
		{
			name: "position with non-string pos skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,12345],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// First position has non-string posStr, skipped; second is valid
				assert.Len(t, r.Trajectory, 1)
			},
		},
		{
			name: "position with bad geo coords skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"bad_coord"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]", "[]", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// First position has bad coords, skipped; second is valid
				assert.Len(t, r.Trajectory, 1)
			},
		},
		{
			name: "hitParts bad JSON warns but does not error",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]", "not_json_hitparts", "shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// Bad hitParts JSON should warn but not error
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name: "hitPart with non-float entityID skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]",
				`[["not_a_number","body","1000.0,1000.0,50.0",700]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name: "hitPart with < 4 elements skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]",
				`[[42,"body","1000.0,1000.0,50.0"]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name: "hitPart with non-string position skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]",
				`[[42,"body",12345,700]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name: "hitPart with bad geo position skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]",
				`[[42,"body","bad_coord",700]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name: "hitPart with non-float frame skipped",
			input: []string{
				"700", "0", "1", "-1", "", "1", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				`[[0.5,700,"1000.0,1000.0,50.0"],[0.6,701,"1001.0,1001.0,49.0"]]`,
				"[100,0,0]",
				`[[42,"body","1000.0,1000.0,50.0","not_a_number"]]`,
				"shotBullet", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				assert.Empty(t, r.HitParts)
			},
		},
		{
			name:    "error: insufficient fields (19)",
			input:   []string{"0", "0", "0", "0", "", "0", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode", "[]", "[0,0,0]", "[]", "sim", "false"},
			wantErr: true,
		},
		{
			name: "error: bad firedFrame",
			input: []string{
				"abc", "0", "5", "-1", "", "5", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				"[]", "[0,0,0]", "[]", "sim", "false", "icon",
			},
			wantErr: true,
		},
		{
			name: "error: bad firerID",
			input: []string{
				"100", "0", "abc", "-1", "", "5", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				"[]", "[0,0,0]", "[]", "sim", "false", "icon",
			},
			wantErr: true,
		},
		{
			name: "error: bad vehicleID",
			input: []string{
				"100", "0", "5", "abc", "", "5", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				"[]", "[0,0,0]", "[]", "sim", "false", "icon",
			},
			wantErr: true,
		},
		{
			name: "bad remoteControllerID is ignored (field not used)",
			input: []string{
				"100", "0", "5", "-1", "", "abc", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				"[]", "[0,0,0]", "[]", "sim", "false", "icon",
			},
			check: func(t *testing.T, r ProjectileEvent) {
				// remoteControllerID is no longer parsed, so bad values don't error
				assert.Equal(t, uint(100), r.CaptureFrame)
			},
		},
		{
			name: "error: bad positions JSON",
			input: []string{
				"100", "0", "5", "-1", "", "5", "w", "W", "m", "M", "mag", "Mag", "ammo", "mode",
				"not_json", "[0,0,0]", "[]", "sim", "false", "icon",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseProjectileEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseGeneralEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.GeneralEvent)
		wantErr bool
	}{
		{
			name:  "recording started",
			input: []string{"0", "generalEvent", "Recording started.", "{}"},
			check: func(t *testing.T, e core.GeneralEvent) {
				assert.Equal(t, uint(0), e.CaptureFrame)
				assert.Equal(t, "generalEvent", e.Name)
				assert.Equal(t, "Recording started.", e.Message)
			},
		},
		{
			name:  "end mission with extraData",
			input: []string{"1043", "endMission", "", `{"message":"BLUFOR wins","winner":"WEST"}`},
			check: func(t *testing.T, e core.GeneralEvent) {
				assert.Equal(t, uint(1043), e.CaptureFrame)
				assert.Equal(t, "endMission", e.Name)
				assert.Equal(t, "", e.Message)
				assert.Equal(t, "BLUFOR wins", e.ExtraData["message"])
				assert.Equal(t, "WEST", e.ExtraData["winner"])
			},
		},
		{
			name:  "no extraData (3 fields)",
			input: []string{"0", "respawnTickets", "[-1,-1,-1,-1]"},
			check: func(t *testing.T, e core.GeneralEvent) {
				assert.Equal(t, "respawnTickets", e.Name)
				assert.Equal(t, "[-1,-1,-1,-1]", e.Message)
				assert.Nil(t, e.ExtraData)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "evt", "msg", "{}"},
			wantErr: true,
		},
		{
			name:    "error: bad extraData JSON",
			input:   []string{"0", "evt", "msg", "not_json"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseGeneralEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseFpsEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.ServerFpsEvent)
		wantErr bool
	}{
		{
			name:  "normal FPS",
			input: []string{"0", "35.4767", "3.55872"},
			check: func(t *testing.T, e core.ServerFpsEvent) {
				assert.Equal(t, uint(0), e.CaptureFrame)
				assert.InDelta(t, float32(35.4767), e.FpsAverage, 0.01)
				assert.InDelta(t, float32(3.55872), e.FpsMin, 0.01)
			},
		},
		{
			name:  "low FPS",
			input: []string{"16", "18.9798", "14.4928"},
			check: func(t *testing.T, e core.ServerFpsEvent) {
				assert.Equal(t, uint(16), e.CaptureFrame)
				assert.InDelta(t, float32(18.9798), e.FpsAverage, 0.01)
				assert.InDelta(t, float32(14.4928), e.FpsMin, 0.01)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "35.0", "3.0"},
			wantErr: true,
		},
		{
			name:    "error: bad fpsAvg",
			input:   []string{"0", "abc", "3.0"},
			wantErr: true,
		},
		{
			name:    "error: bad fpsMin",
			input:   []string{"0", "35.0", "abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseFpsEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseTimeState(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, ts core.TimeState)
		wantErr bool
	}{
		{
			name:  "normal",
			input: []string{"0", "2026-02-15T17:46:12.621", "2035-07-02T18:00:00", "1", "0"},
			check: func(t *testing.T, ts core.TimeState) {
				assert.Equal(t, uint(0), ts.CaptureFrame)
				assert.Equal(t, "2026-02-15T17:46:12.621", ts.SystemTimeUTC)
				assert.Equal(t, "2035-07-02T18:00:00", ts.MissionDate)
				assert.Equal(t, float32(1), ts.TimeMultiplier)
				assert.Equal(t, float32(0), ts.MissionTime)
			},
		},
		{
			name:  "with mission time",
			input: []string{"10", "2026-02-15T17:47:00", "2035-07-02T18:05:00", "1", "5.013"},
			check: func(t *testing.T, ts core.TimeState) {
				assert.Equal(t, uint(10), ts.CaptureFrame)
				assert.InDelta(t, float32(5.013), ts.MissionTime, 0.001)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "time", "date", "1", "0"},
			wantErr: true,
		},
		{
			name:    "error: bad timeMultiplier",
			input:   []string{"0", "time", "date", "abc", "0"},
			wantErr: true,
		},
		{
			name:    "error: bad missionTime",
			input:   []string{"0", "time", "date", "1", "abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseTimeState(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseTelemetryEvent(t *testing.T) {
	p := newTestParser()

	// Real data from ArmA 3 RPT: mission start (frame 0)
	// Sides order: [east, west, independent, civilian], each [[server_local], [remote]]
	// Locality: [units_total, units_alive, units_dead, groups_total, vehicles_total, vehicles_weaponholder]
	// Global: [units_alive, units_dead, groups_total, vehicles_total, vehicles_weaponholder, players_alive, players_dead, players_connected]
	// Scripts: [spawn, execVM, exec, execFSM, pfh]
	// Weather: [fog, overcast, rain, humidity, waves, windDir, windStr, gusts, lightnings, moonIntensity, moonPhase, sunOrMoon]
	// Players: [[uid, name, avgPing, avgBandwidth, desync], ...]
	missionStart := []string{
		"0",                          // frame
		"[43.3604,4.36681]",          // fps
		`[[[1,1,0,1,0,0],[0,0,0,0,0,0]],[[10,10,0,8,0,0],[0,0,0,0,0,0]],[[6,6,0,6,0,0],[0,0,0,0,0,0]],[[5,5,12,0,24,0],[0,0,0,0,0,0]]]`, // sides
		"[22,12,15,28,0,1,0,1]",      // global
		"[28,4,0,4,2]",               // scripts
		"[0.2,0.25,0,0,0.1,0,0.25,0.315,0,0.423059,0.421672,1]", // weather
		`[["76561198000074241","info",100,28,0]]`, // players
	}

	// Real data from RPT: mid-mission (frame 114) — combat started, casualties, vehicles deployed
	// Notable: bandwidth uses scientific notation (1.67772e+07)
	midMission := []string{
		"114",                        // frame
		"[122.137,90.9091]",          // fps
		`[[[32,32,0,10,0,0],[0,0,0,0,0,0]],[[6,6,0,8,1,0],[0,0,0,0,0,0]],[[6,6,0,6,0,0],[0,0,0,0,0,0]],[[4,4,19,0,23,22],[0,0,0,0,0,0]]]`, // sides
		"[48,19,24,28,22,1,0,1]",     // global
		"[17,3,0,4,3]",               // scripts
		"[0.2,0.25,0,0,0.1,160.864,0.25,0.175,0.003153,0.441581,0.421672,1]", // weather
		`[["76561198000074241","info",0,1.67772e+07,0]]`, // players
	}

	// Real data from RPT: late mission (frame 234) — more casualties, weapon holders on ground
	lateMission := []string{
		"234",                        // frame
		"[132.231,76.9231]",          // fps
		`[[[30,30,0,10,0,0],[0,0,0,0,0,0]],[[3,3,0,8,1,0],[0,0,0,0,0,0]],[[2,2,0,6,0,0],[0,0,0,0,0,0]],[[2,2,22,0,21,24],[0,0,0,0,0,0]]]`, // sides
		"[37,22,24,26,24,1,0,1]",     // global
		"[18,3,0,4,4]",               // scripts
		"[0.2,0.25,0,0,0.1,10.289,0.25,0.175,0.00649121,0.441576,0.421672,1]", // weather
		`[["76561198000074241","info",0,1.67772e+07,0]]`, // players
	}

	zeroSides := `[[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]]`

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.TelemetryEvent)
		wantErr bool
	}{
		{
			name:  "RPT mission start (frame 0)",
			input: missionStart,
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(0), e.CaptureFrame)
				assert.InDelta(t, 43.3604, float64(e.FpsAverage), 0.001)
				assert.InDelta(t, 4.36681, float64(e.FpsMin), 0.001)

				// East: 1 local unit alive, 1 group, no vehicles
				east := e.SideEntityCounts[0]
				assert.Equal(t, uint(1), east.Local.UnitsTotal)
				assert.Equal(t, uint(1), east.Local.UnitsAlive)
				assert.Equal(t, uint(0), east.Local.UnitsDead)
				assert.Equal(t, uint(1), east.Local.Groups)
				assert.Equal(t, uint(0), east.Local.Vehicles)
				assert.Equal(t, uint(0), east.Local.WeaponHolders)
				// East remote: all zero
				assert.Equal(t, uint(0), east.Remote.UnitsTotal)

				// West: 10 local units, 8 groups
				assert.Equal(t, uint(10), e.SideEntityCounts[1].Local.UnitsTotal)
				assert.Equal(t, uint(8), e.SideEntityCounts[1].Local.Groups)

				// Independent: 6 local units, 6 groups
				assert.Equal(t, uint(6), e.SideEntityCounts[2].Local.UnitsTotal)
				assert.Equal(t, uint(6), e.SideEntityCounts[2].Local.Groups)

				// Civilian: 5 alive, 12 dead, 24 vehicles, no weapon holders
				civ := e.SideEntityCounts[3].Local
				assert.Equal(t, uint(5), civ.UnitsTotal)
				assert.Equal(t, uint(5), civ.UnitsAlive)
				assert.Equal(t, uint(12), civ.UnitsDead)
				assert.Equal(t, uint(0), civ.Groups)
				assert.Equal(t, uint(24), civ.Vehicles)
				assert.Equal(t, uint(0), civ.WeaponHolders)

				// Global
				assert.Equal(t, uint(22), e.GlobalCounts.UnitsAlive)
				assert.Equal(t, uint(12), e.GlobalCounts.UnitsDead)
				assert.Equal(t, uint(15), e.GlobalCounts.Groups)
				assert.Equal(t, uint(28), e.GlobalCounts.Vehicles)
				assert.Equal(t, uint(0), e.GlobalCounts.WeaponHolders)
				assert.Equal(t, uint(1), e.GlobalCounts.PlayersAlive)
				assert.Equal(t, uint(0), e.GlobalCounts.PlayersDead)
				assert.Equal(t, uint(1), e.GlobalCounts.PlayersConnected)

				// Scripts: 28 spawn, 4 execVM, 0 exec, 4 execFSM, 2 pfh
				assert.Equal(t, uint(28), e.Scripts.Spawn)
				assert.Equal(t, uint(4), e.Scripts.ExecVM)
				assert.Equal(t, uint(0), e.Scripts.Exec)
				assert.Equal(t, uint(4), e.Scripts.ExecFSM)
				assert.Equal(t, uint(2), e.Scripts.PFH)

				// Weather: daytime (sunOrMoon=1), low wind, no rain
				assert.InDelta(t, 0.2, float64(e.Weather.Fog), 0.001)
				assert.InDelta(t, 0.25, float64(e.Weather.Overcast), 0.001)
				assert.InDelta(t, 0.0, float64(e.Weather.Rain), 0.001)
				assert.InDelta(t, 0.0, float64(e.Weather.Humidity), 0.001)
				assert.InDelta(t, 0.1, float64(e.Weather.Waves), 0.001)
				assert.InDelta(t, 0.0, float64(e.Weather.WindDir), 0.001)
				assert.InDelta(t, 0.25, float64(e.Weather.WindStr), 0.001)
				assert.InDelta(t, 0.315, float64(e.Weather.Gusts), 0.001)
				assert.InDelta(t, 0.0, float64(e.Weather.Lightnings), 0.001)
				assert.InDelta(t, 0.423059, float64(e.Weather.MoonIntensity), 0.0001)
				assert.InDelta(t, 0.421672, float64(e.Weather.MoonPhase), 0.0001)
				assert.InDelta(t, 1.0, float64(e.Weather.SunOrMoon), 0.001)

				// One player connected
				require.Len(t, e.Players, 1)
				assert.Equal(t, "76561198000074241", e.Players[0].UID)
				assert.Equal(t, "info", e.Players[0].Name)
				assert.InDelta(t, 100.0, float64(e.Players[0].Ping), 0.01)
				assert.InDelta(t, 28.0, float64(e.Players[0].BW), 0.01)
				assert.InDelta(t, 0.0, float64(e.Players[0].Desync), 0.01)
			},
		},
		{
			name:  "RPT mid-mission (frame 114) — combat, scientific notation bandwidth",
			input: midMission,
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(114), e.CaptureFrame)
				assert.InDelta(t, 122.137, float64(e.FpsAverage), 0.01)
				assert.InDelta(t, 90.9091, float64(e.FpsMin), 0.01)

				// East lost 1 unit (33→32 in later frame, but at 114: 32 local)
				assert.Equal(t, uint(32), e.SideEntityCounts[0].Local.UnitsTotal)

				// West: 1 vehicle now present
				assert.Equal(t, uint(1), e.SideEntityCounts[1].Local.Vehicles)

				// Civilian: 19 dead (was 12 at start), 23 vehicles, 22 weapon holders on ground
				civ := e.SideEntityCounts[3].Local
				assert.Equal(t, uint(4), civ.UnitsTotal)
				assert.Equal(t, uint(19), civ.UnitsDead)
				assert.Equal(t, uint(23), civ.Vehicles)
				assert.Equal(t, uint(22), civ.WeaponHolders)

				// Global: 22 weapon holders (from dropped equipment)
				assert.Equal(t, uint(48), e.GlobalCounts.UnitsAlive)
				assert.Equal(t, uint(22), e.GlobalCounts.WeaponHolders)

				// Wind direction changed to 160.864°
				assert.InDelta(t, 160.864, float64(e.Weather.WindDir), 0.01)

				// Player bandwidth in scientific notation: 1.67772e+07 ≈ 16777200
				require.Len(t, e.Players, 1)
				assert.InDelta(t, 1.67772e+07, float64(e.Players[0].BW), 100)
				assert.InDelta(t, 0.0, float64(e.Players[0].Ping), 0.01)
			},
		},
		{
			name:  "RPT late mission (frame 234) — more casualties, weapon holders",
			input: lateMission,
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(234), e.CaptureFrame)
				assert.InDelta(t, 132.231, float64(e.FpsAverage), 0.01)
				assert.InDelta(t, 76.9231, float64(e.FpsMin), 0.01)

				// East: down to 30 local units
				assert.Equal(t, uint(30), e.SideEntityCounts[0].Local.UnitsTotal)

				// West: down to 3 units, still 1 vehicle
				west := e.SideEntityCounts[1].Local
				assert.Equal(t, uint(3), west.UnitsTotal)
				assert.Equal(t, uint(1), west.Vehicles)

				// Independent: down to 2 units
				assert.Equal(t, uint(2), e.SideEntityCounts[2].Local.UnitsTotal)

				// Civilian: 22 dead, 21 vehicles, 24 weapon holders
				civ := e.SideEntityCounts[3].Local
				assert.Equal(t, uint(22), civ.UnitsDead)
				assert.Equal(t, uint(21), civ.Vehicles)
				assert.Equal(t, uint(24), civ.WeaponHolders)

				// Global: 24 weapon holders, 22 dead overall
				assert.Equal(t, uint(37), e.GlobalCounts.UnitsAlive)
				assert.Equal(t, uint(22), e.GlobalCounts.UnitsDead)
				assert.Equal(t, uint(24), e.GlobalCounts.WeaponHolders)

				// Scripts: pfh increased to 4
				assert.Equal(t, uint(18), e.Scripts.Spawn)
				assert.Equal(t, uint(4), e.Scripts.PFH)

				// Weather: wind shifted to 10.289°
				assert.InDelta(t, 10.289, float64(e.Weather.WindDir), 0.01)
			},
		},
		{
			name: "empty players (no human players connected)",
			input: []string{
				"0", "[50.0,40.0]", zeroSides,
				"[0,0,0,0,0,0,0,0]", "[0,0,0,0,0]",
				"[0,0,0,0,0,0,0,0,0,0,0,0]", "[]",
			},
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(0), e.CaptureFrame)
				assert.InDelta(t, 50.0, float64(e.FpsAverage), 0.01)
				assert.Empty(t, e.Players)
			},
		},
		{
			name:    "error: too few args",
			input:   []string{"500", "[45.5,30.0]"},
			wantErr: true,
		},
		{
			name:    "error: no data",
			input:   []string{},
			wantErr: true,
		},
		{
			name: "error: bad frame",
			input: []string{
				"abc", "[45.5,30.0]", zeroSides,
				"[0,0,0,0,0,0,0,0]", "[0,0,0,0,0]",
				"[0,0,0,0,0,0,0,0,0,0,0,0]", "[]",
			},
			wantErr: true,
		},
		{
			name: "error: bad fps JSON",
			input: []string{
				"500", "not_json", zeroSides,
				"[0,0,0,0,0,0,0,0]", "[0,0,0,0,0]",
				"[0,0,0,0,0,0,0,0,0,0,0,0]", "[]",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseTelemetryEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

// Benchmarks using real RPT data

func BenchmarkParseTelemetryEvent_1Player(b *testing.B) {
	p := newTestParser()
	// RPT frame 0: mission start, 1 player
	args := []string{
		"0",
		"[43.3604,4.36681]",
		`[[[1,1,0,1,0,0],[0,0,0,0,0,0]],[[10,10,0,8,0,0],[0,0,0,0,0,0]],[[6,6,0,6,0,0],[0,0,0,0,0,0]],[[5,5,12,0,24,0],[0,0,0,0,0,0]]]`,
		"[22,12,15,28,0,1,0,1]",
		"[28,4,0,4,2]",
		"[0.2,0.25,0,0,0.1,0,0.25,0.315,0,0.423059,0.421672,1]",
		`[["76561198000074241","info",100,28,0]]`,
	}
	b.ResetTimer()
	for b.Loop() {
		// Copy args to avoid mutation from TrimQuotes/FixEscapeQuotes
		input := make([]string, len(args))
		copy(input, args)
		p.ParseTelemetryEvent(input) //nolint:errcheck
	}
}

func BenchmarkParseTelemetryEvent_NoPlayers(b *testing.B) {
	p := newTestParser()
	args := []string{
		"0",
		"[50.0,40.0]",
		`[[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]]`,
		"[0,0,0,0,0,0,0,0]",
		"[0,0,0,0,0]",
		"[0,0,0,0,0,0,0,0,0,0,0,0]",
		"[]",
	}
	b.ResetTimer()
	for b.Loop() {
		input := make([]string, len(args))
		copy(input, args)
		p.ParseTelemetryEvent(input) //nolint:errcheck
	}
}

func BenchmarkParseTelemetryEvent_20Players(b *testing.B) {
	p := newTestParser()
	// Simulate a populated server: 20 connected players
	args := []string{
		"114",
		"[122.137,90.9091]",
		`[[[32,32,0,10,0,0],[0,0,0,0,0,0]],[[6,6,0,8,1,0],[0,0,0,0,0,0]],[[6,6,0,6,0,0],[0,0,0,0,0,0]],[[4,4,19,0,23,22],[0,0,0,0,0,0]]]`,
		"[48,19,24,28,22,1,0,1]",
		"[17,3,0,4,3]",
		"[0.2,0.25,0,0,0.1,160.864,0.25,0.175,0.003153,0.441581,0.421672,1]",
		`[["76561198000000001","Alpha1",45,512,0.1],["76561198000000002","Alpha2",52,480,0.05],` +
			`["76561198000000003","Alpha3",38,510,0],["76561198000000004","Alpha4",61,490,0.2],` +
			`["76561198000000005","Bravo1",44,505,0],["76561198000000006","Bravo2",55,495,0.1],` +
			`["76561198000000007","Bravo3",39,515,0],["76561198000000008","Bravo4",48,500,0.05],` +
			`["76561198000000009","Charlie1",42,508,0],["76561198000000010","Charlie2",57,488,0.15],` +
			`["76561198000000011","Charlie3",35,520,0],["76561198000000012","Charlie4",63,475,0.3],` +
			`["76561198000000013","Delta1",41,512,0],["76561198000000014","Delta2",50,498,0.1],` +
			`["76561198000000015","Delta3",37,516,0],["76561198000000016","Delta4",59,485,0.2],` +
			`["76561198000000017","Echo1",43,507,0],["76561198000000018","Echo2",54,492,0.1],` +
			`["76561198000000019","Echo3",36,518,0],["76561198000000020","Echo4",62,478,0.25]]`,
	}
	b.ResetTimer()
	for b.Loop() {
		input := make([]string, len(args))
		copy(input, args)
		p.ParseTelemetryEvent(input) //nolint:errcheck
	}
}
