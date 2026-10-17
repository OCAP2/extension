package parser

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Arma doubles every quote inside a string before it reaches the extension, so
// a CBA_fnc_encodeJSON snapshot payload arrives wrapped in quotes with every
// inner quote doubled. The parser has to hand the web side JSON it can decode,
// otherwise a profile card gets a payload it cannot rebuild.
func TestParseGeneralEvent_UnescapesSnapshotPayloads(t *testing.T) {
	p := newTestParser()

	cases := []struct {
		name      string
		eventType string
		raw       string
		check     func(t *testing.T, payload map[string]any)
	}{
		{
			name:      "inventory snapshot",
			eventType: "inventorySnapshot",
			raw:       `"{""unitId"":4,""playerUid"":""76561198000000000"",""reason"":""periodic"",""magazines"":[{""class"":""ACE_fieldDressing"",""count"":3,""cat"":""medical""}]}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, float64(4), payload["unitId"])
				assert.Equal(t, "76561198000000000", payload["playerUid"])
				assert.Equal(t, "periodic", payload["reason"])
				magazines := payload["magazines"].([]any)
				item := magazines[0].(map[string]any)
				assert.Equal(t, "ACE_fieldDressing", item["class"])
				assert.Equal(t, "medical", item["cat"])
			},
		},
		{
			name:      "medical snapshot with generic body part items",
			eventType: "medicalSnapshot",
			raw:       `"{""unitId"":4,""bodyParts"":[{""part"":""leftarm"",""damage"":0.35,""items"":[{""kind"":""tourniquet"",""name"":""Tourniquet (CAT)""}]}]}"`,
			check: func(t *testing.T, payload map[string]any) {
				parts := payload["bodyParts"].([]any)
				part := parts[0].(map[string]any)
				assert.Equal(t, "leftarm", part["part"])
				items := part["items"].([]any)
				assert.Equal(t, "tourniquet", items[0].(map[string]any)["kind"])
				assert.Equal(t, "Tourniquet (CAT)", items[0].(map[string]any)["name"])
			},
		},
		{
			name:      "stamina snapshot carrying the stance enum",
			eventType: "staminaSnapshot",
			raw:       `"{""unitId"":4,""vanilla"":{""stamina"":12.5,""staminaMax"":30,""load"":0.61,""massUnits"":695,""stance"":3}}"`,
			check: func(t *testing.T, payload map[string]any) {
				vanilla := payload["vanilla"].(map[string]any)
				assert.Equal(t, 12.5, vanilla["stamina"])
				assert.Equal(t, float64(3), vanilla["stance"])
			},
		},
		{
			name:      "radio snapshot recorded as a diff",
			eventType: "radioSnapshot",
			raw:       `"{""unitId"":4,""diffOf"":60,""set"":{""radios"":[{""class"":""TFAR_anprc152"",""mod"":""TFAR"",""type"":""SW"",""frequency"":472.8,""rangeMeters"":5000}]},""unset"":[""reason""]}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, float64(60), payload["diffOf"])
				radios := payload["set"].(map[string]any)["radios"].([]any)
				radio := radios[0].(map[string]any)
				assert.Equal(t, "TFAR", radio["mod"])
				assert.Equal(t, float64(5000), radio["rangeMeters"])
				assert.Equal(t, []any{"reason"}, payload["unset"])
			},
		},
		{
			name:      "server fps sample",
			eventType: "serverFps",
			raw:       `"{""fps"":47.25}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, 47.25, payload["fps"])
			},
		},
		{
			name:      "weather sample",
			eventType: "weather",
			raw:       `"{""overcast"":0.42,""rain"":0.1,""fog"":0.2,""windSpeed"":3.4,""windDir"":247,""humidity"":0.68,""moonPhase"":0.81,""temperature"":16.3,""ace"":true}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, 0.42, payload["overcast"])
				assert.Equal(t, 0.81, payload["moonPhase"])
				assert.Equal(t, 16.3, payload["temperature"])
				assert.Equal(t, true, payload["ace"])
			},
		},
		{
			name:      "tfar settings stamped on the first capture frame",
			eventType: "tfarSettings",
			raw:       `"{""terrainInterceptionCoefficient"":7,""globalRadioRangeCoef"":1,""tfarLoaded"":true,""source"":""cba""}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, float64(7), payload["terrainInterceptionCoefficient"])
				assert.Equal(t, float64(1), payload["globalRadioRangeCoef"])
				assert.Equal(t, true, payload["tfarLoaded"])
			},
		},
		{
			name:      "acre settings stamped on the first capture frame",
			eventType: "acreSettings",
			raw:       `"{""terrainLoss"":1,""signalModel"":2,""acreLoaded"":true,""source"":""cba""}"`,
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, float64(1), payload["terrainLoss"])
				assert.Equal(t, float64(2), payload["signalModel"])
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			event, err := p.ParseGeneralEvent([]string{"120", `"` + testCase.eventType + `"`, testCase.raw})
			require.NoError(t, err)
			assert.Equal(t, testCase.eventType, event.Name)

			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(event.Message), &payload),
				"snapshot message is not decodable JSON: %s", event.Message)
			testCase.check(t, payload)
		})
	}
}

// The server FPS PFH fires once before the capture loop has produced a frame,
// so the addon clamps that first sample to frame 0. Frame 0 must survive the
// parser as 0 and not be rejected.
func TestParseGeneralEvent_ServerFpsFirstSampleClampedToFrameZero(t *testing.T) {
	p := newTestParser()

	event, err := p.ParseGeneralEvent([]string{"0", "serverFps", `{"fps":51}`})
	require.NoError(t, err)
	assert.Equal(t, uint(0), uint(event.CaptureFrame))
	assert.Equal(t, "serverFps", event.Name)
	assert.Equal(t, `{"fps":51}`, event.Message)
}
