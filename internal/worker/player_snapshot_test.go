package worker

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The addon sends [captureFrame, eventType, payloadJSON] for all four player
// detail commands, with the payload produced by CBA_fnc_encodeJSON. These run
// through the real parser so the JSON survives quote unescaping intact, which is
// what the web UI needs to rebuild snapshots and diffs.
func TestPlayerSnapshotCommands_PreservePayload(t *testing.T) {
	cases := []struct {
		command   string
		eventType string
		payload   string
	}{
		{
			command:   ":PLAYER:INVENTORY:",
			eventType: "inventorySnapshot",
			payload:   `{"unitId":4,"playerUid":"76561198000000000","reason":"death","massUnits":695,"magazines":[{"class":"30Rnd_65x39_caseless_mag","count":6,"totalRounds":174,"loadedCount":1}]}`,
		},
		{
			command:   ":PLAYER:MEDICAL:",
			eventType: "medicalSnapshot",
			payload:   `{"unitId":4,"bodyParts":[{"part":"body","damage":0.4,"items":[{"kind":"bandage","name":"Bandaged wound","count":2}]}],"ace":{"heartRate":133}}`,
		},
		{
			command:   ":PLAYER:STAMINA:",
			eventType: "staminaSnapshot",
			payload:   `{"unitId":4,"diffOf":60,"set":{"vanilla":{"load":0.61}}}`,
		},
		{
			command:   ":PLAYER:RADIO:",
			eventType: "radioSnapshot",
			payload:   `{"unitId":4,"diffOf":60,"set":{"radios":[{"class":"TFAR_anprc152_1","frequency":472.8,"rangeMeters":5000}]},"unset":["reason"]}`,
		},
		{
			command:   ":EVENT:GENERAL:",
			eventType: "tfarSettings",
			payload:   `{"terrainInterceptionCoefficient":12,"globalRadioRangeCoef":0.5,"tfarLoaded":true,"source":"cba"}`,
		},
		{
			command:   ":EVENT:GENERAL:",
			eventType: "acreSettings",
			payload:   `{"terrainLoss":0.4,"signalModel":1,"ignoreAntennaDirection":true,"acreLoaded":true,"source":"cba"}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.eventType, func(t *testing.T) {
			d, _ := newTestDispatcher(t)
			backend := &mockBackend{}
			manager := NewManager(Dependencies{
				ParserService: parser.NewParser(slog.New(slog.NewTextHandler(io.Discard, nil))),
				EntityCache:   cache.NewEntityCache(),
			}, backend)
			manager.RegisterHandlers(d)

			require.True(t, d.HasHandler(testCase.command))

			_, err := d.Dispatch(dispatcher.Event{
				Command: testCase.command,
				Args:    []string{"120", testCase.eventType, testCase.payload},
			})
			require.NoError(t, err)

			waitFor(t, func() bool {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				return len(backend.generalEvents) > 0
			}, "timed out waiting for "+testCase.eventType)

			backend.mu.Lock()
			defer backend.mu.Unlock()
			recorded := backend.generalEvents[0]
			assert.Equal(t, testCase.eventType, recorded.Name)
			assert.Equal(t, uint(120), uint(recorded.CaptureFrame))

			var decoded, expected map[string]any
			require.NoError(t, json.Unmarshal([]byte(recorded.Message), &decoded))
			require.NoError(t, json.Unmarshal([]byte(testCase.payload), &expected))
			assert.Equal(t, expected, decoded)
		})
	}
}
