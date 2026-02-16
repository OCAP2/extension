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

	// A valid telemetry JSON payload
	validJSON := `[500,[45.5,30.0],[[[10,8,2,3,5,1],[4,3,1,1,2,0]],[[12,10,2,4,6,2],[5,4,1,2,3,1]],[[2,2,0,1,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]],[40,5,8,13,3,16,2,18],[3,2,1,0,5],[0.1,0.5,0.0,0.6,0.2,180.0,3.5,0.1,0.0,0.8,0.25,1.0],[["uid1","Player1",25.5,512.0,0.1],["uid2","Player2",30.0,480.0,0.05]]]`

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.TelemetryEvent)
		wantErr bool
	}{
		{
			name:  "valid telemetry",
			input: []string{validJSON},
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(500), e.CaptureFrame)
				assert.InDelta(t, float32(45.5), e.FpsAverage, 0.01)
				assert.InDelta(t, float32(30.0), e.FpsMin, 0.01)

				// Side data - east local
				assert.Equal(t, uint(10), e.SideEntityCounts[0].Local.UnitsTotal)
				assert.Equal(t, uint(8), e.SideEntityCounts[0].Local.UnitsAlive)
				// Side data - west remote
				assert.Equal(t, uint(5), e.SideEntityCounts[1].Remote.UnitsTotal)

				// Global
				assert.Equal(t, uint(40), e.GlobalCounts.UnitsAlive)
				assert.Equal(t, uint(18), e.GlobalCounts.PlayersConnected)

				// Scripts
				assert.Equal(t, uint(3), e.Scripts.Spawn)
				assert.Equal(t, uint(5), e.Scripts.PFH)

				// Weather
				assert.InDelta(t, float32(0.5), e.Weather.Overcast, 0.01)
				assert.InDelta(t, float32(180.0), e.Weather.WindDir, 0.1)

				// Players
				require.Len(t, e.Players, 2)
				assert.Equal(t, "uid1", e.Players[0].UID)
				assert.Equal(t, "Player1", e.Players[0].Name)
				assert.InDelta(t, float32(25.5), e.Players[0].Ping, 0.01)
			},
		},
		{
			name:  "empty players array",
			input: []string{`[0,[50.0,40.0],[[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]],[0,0,0,0,0,0,0,0],[0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0],[]]`},
			check: func(t *testing.T, e core.TelemetryEvent) {
				assert.Equal(t, uint(0), e.CaptureFrame)
				assert.InDelta(t, float32(50.0), e.FpsAverage, 0.01)
				assert.Empty(t, e.Players)
			},
		},
		{
			name:    "error: no data",
			input:   []string{},
			wantErr: true,
		},
		{
			name:    "error: bad JSON",
			input:   []string{"not_json"},
			wantErr: true,
		},
		{
			name:    "error: too few elements",
			input:   []string{`[500,[45.5,30.0]]`},
			wantErr: true,
		},
		{
			name:    "error: bad frame",
			input:   []string{`["abc",[45.5,30.0],[[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]],[0,0,0,0,0,0,0,0],[0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0],[]]`},
			wantErr: true,
		},
		{
			name:    "error: bad fps array",
			input:   []string{`[500,"bad",[[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]],[[0,0,0,0,0,0],[0,0,0,0,0,0]]],[0,0,0,0,0,0,0,0],[0,0,0,0,0],[0,0,0,0,0,0,0,0,0,0,0,0],[]]`},
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
