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
