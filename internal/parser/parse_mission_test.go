package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Empty(t, mission.AddonVersion, "parser should not set version")
	assert.Empty(t, mission.ExtensionVersion, "parser should not set version")
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

func TestParseMission_Errors(t *testing.T) {
	p := newTestParser()

	validWorld := `{"worldName":"Altis","displayName":"Altis","worldSize":30720,"latitude":-40.0,"longitude":30.0}`

	// helper: build valid mission JSON, overridable per test
	validMission := func(overrides map[string]string) string {
		base := map[string]string{
			"missionName":       `"Test"`,
			"missionNameSource": `"file"`,
			"briefingName":      `"Brief"`,
			"serverName":        `"Server"`,
			"serverProfile":     `"Profile"`,
			"onLoadName":        `"Loading"`,
			"author":            `"Author"`,
			"tag":               `"TvT"`,
			"captureDelay":      `1.0`,
			"addons":            `[]`,
			"playableSlots":     `[10,10,5,0,2]`,
			"sideFriendly":      `[false,false,false]`,
		}
		for k, v := range overrides {
			base[k] = v
		}
		result := "{"
		first := true
		for k, v := range base {
			if !first {
				result += ","
			}
			result += `"` + k + `":` + v
			first = false
		}
		result += "}"
		return result
	}

	tests := []struct {
		name  string
		world string
		miss  string
	}{
		{
			name:  "bad world JSON",
			world: "not_json",
			miss:  validMission(nil),
		},
		{
			name:  "bad mission JSON",
			world: validWorld,
			miss:  "not_json",
		},
		{
			name:  "addon with fractional workshopID",
			world: validWorld,
			miss:  validMission(map[string]string{"addons": `[["mod",450814.5]]`}),
		},
		{
			name:  "addon with invalid format (not array)",
			world: validWorld,
			miss:  validMission(map[string]string{"addons": `["not_an_array"]`}),
		},
		{
			name:  "addon with non-string name",
			world: validWorld,
			miss:  validMission(map[string]string{"addons": `[[123,"456"]]`}),
		},
		{
			name:  "addon with invalid workshopID type (bool)",
			world: validWorld,
			miss:  validMission(map[string]string{"addons": `[["mod",true]]`}),
		},
		{
			name:  "addons field missing",
			world: validWorld,
			miss: `{
				"missionName":"Test","missionNameSource":"file","briefingName":"Brief",
				"serverName":"Server","serverProfile":"Profile","onLoadName":"Loading",
				"author":"Author","tag":"TvT","captureDelay":1.0,
				"playableSlots":[10,10,5,0,2],"sideFriendly":[false,false,false]
			}`,
		},
		{
			name:  "missing captureDelay",
			world: validWorld,
			miss:  validMission(map[string]string{"captureDelay": `"not_a_number"`}),
		},
		{
			name:  "bad missionNameSource",
			world: validWorld,
			miss:  validMission(map[string]string{"missionNameSource": `123`}),
		},
		{
			name:  "bad missionName",
			world: validWorld,
			miss:  validMission(map[string]string{"missionName": `123`}),
		},
		{
			name:  "bad briefingName",
			world: validWorld,
			miss:  validMission(map[string]string{"briefingName": `123`}),
		},
		{
			name:  "bad serverName",
			world: validWorld,
			miss:  validMission(map[string]string{"serverName": `123`}),
		},
		{
			name:  "bad serverProfile",
			world: validWorld,
			miss:  validMission(map[string]string{"serverProfile": `123`}),
		},
		{
			name:  "bad onLoadName",
			world: validWorld,
			miss:  validMission(map[string]string{"onLoadName": `123`}),
		},
		{
			name:  "bad author",
			world: validWorld,
			miss:  validMission(map[string]string{"author": `123`}),
		},
		{
			name:  "bad tag",
			world: validWorld,
			miss:  validMission(map[string]string{"tag": `123`}),
		},
		{
			name:  "playableSlots too short",
			world: validWorld,
			miss:  validMission(map[string]string{"playableSlots": `[10,10]`}),
		},
		{
			name:  "sideFriendly too short",
			world: validWorld,
			miss:  validMission(map[string]string{"sideFriendly": `[false]`}),
		},
		{
			name:  "playableSlots missing",
			world: validWorld,
			miss:  validMission(map[string]string{"playableSlots": `"not_array"`}),
		},
		{
			name:  "sideFriendly missing",
			world: validWorld,
			miss:  validMission(map[string]string{"sideFriendly": `"not_array"`}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := p.ParseMission([]string{tt.world, tt.miss})
			assert.Error(t, err, "expected error for: %s", tt.name)
		})
	}
}
