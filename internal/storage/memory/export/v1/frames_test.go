package v1

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A snapshot's diffOf names the frame of the payload it patches. Internally
// frames are 1-based; the v1 export is 0-based, so the pointer has to be
// rebased with the wrapper or the UI folds a diff onto the wrong keyframe.
func TestRewriteSnapshotDiffOfRebasesEverySnapshotKind(t *testing.T) {
	for _, eventName := range []string{
		"inventorySnapshot", "medicalSnapshot", "staminaSnapshot", "radioSnapshot",
	} {
		message := map[string]any{"unitId": 4.0, "diffOf": 60.0, "set": map[string]any{"load": 0.6}}
		out := rewriteSnapshotDiffOf(eventName, message).(map[string]any)

		assert.Equal(t, 59, out["diffOf"], eventName)
		// The rest of the payload is carried through untouched.
		assert.Equal(t, 4.0, out["unitId"], eventName)
		assert.Equal(t, map[string]any{"load": 0.6}, out["set"], eventName)
		// The input is not mutated: the caller still holds the internal frame.
		assert.Equal(t, 60.0, message["diffOf"], eventName)
	}
}

// The very first frame of a recording is internal 1, v1 0. A diff against it
// must not be pushed to the "forever" sentinel or off the timeline.
func TestRewriteSnapshotDiffOfHandlesFirstFrame(t *testing.T) {
	out := rewriteSnapshotDiffOf("radioSnapshot", map[string]any{"diffOf": 1.0}).(map[string]any)
	assert.Equal(t, 0, out["diffOf"])
}

// Only the four per-player kinds carry diffs. The global samples stamped by the
// recorder have no diffOf semantics, so a field of that name is left alone.
func TestRewriteSnapshotDiffOfIgnoresGlobalEvents(t *testing.T) {
	for _, eventName := range []string{"serverFps", "weather", "tfarSettings", "acreSettings", "generalEvent", "connected"} {
		message := map[string]any{"diffOf": 60.0}
		out := rewriteSnapshotDiffOf(eventName, message)
		assert.Equal(t, map[string]any{"diffOf": 60.0}, out, eventName)
	}
}

func TestRewriteSnapshotDiffOfLeavesUnrewritablePayloads(t *testing.T) {
	// A full keyframe has no diffOf at all.
	keyframe := map[string]any{"unitId": 4.0, "reason": "death"}
	assert.Equal(t, keyframe, rewriteSnapshotDiffOf("inventorySnapshot", keyframe))

	// A payload that never decoded into an object stays a string.
	assert.Equal(t, "not json", rewriteSnapshotDiffOf("inventorySnapshot", "not json"))

	// An array payload is not a snapshot object.
	assert.Equal(t, []any{1.0}, rewriteSnapshotDiffOf("medicalSnapshot", []any{1.0}))

	// A diffOf that is not a number cannot be rebased, so it is passed through.
	broken := map[string]any{"diffOf": "60"}
	assert.Equal(t, broken, rewriteSnapshotDiffOf("staminaSnapshot", broken))
}

// The entry-level array diff the recorder writes for gear and radios carries
// per-entry keys the exporter knows nothing about. It has to survive verbatim.
func TestBuildPreservesEntryLevelArrayDiff(t *testing.T) {
	data := snapshotMissionData(
		generalEvent(7, "inventorySnapshot", `{"unitId":4,"magazines":[{"class":"30Rnd","count":6}]}`),
		generalEvent(67, "inventorySnapshot", `{"unitId":4,"diffOf":7,"set":{"magazines":[{"$key":"30Rnd","put":{"count":4,"totalRounds":110},"ord":0},{"$key":"HandGrenade","del":true}]}}`),
	)

	export := Build(data)
	require.Len(t, export.Events, 2)

	payload := export.Events[1][2].(map[string]any)
	assert.Equal(t, 6, payload["diffOf"])

	entries := payload["set"].(map[string]any)["magazines"].([]any)
	require.Len(t, entries, 2)
	put := entries[0].(map[string]any)
	assert.Equal(t, "30Rnd", put["$key"])
	assert.Equal(t, 4.0, put["put"].(map[string]any)["count"])
	assert.Equal(t, 0.0, put["ord"])
	del := entries[1].(map[string]any)
	assert.Equal(t, "HandGrenade", del["$key"])
	assert.Equal(t, true, del["del"])
}

// The global samples land in the event stream as decoded objects, not as the
// raw JSON string, so an unmodified reader sees the same shape as any other
// generic event and a newer one can read the fields.
func TestBuildKeepsGlobalSamplePayloadsAsObjects(t *testing.T) {
	data := snapshotMissionData(
		generalEvent(1, "serverFps", `{"fps":47.25}`),
		generalEvent(1, "tfarSettings", `{"terrainInterceptionCoefficient":7,"globalRadioRangeCoef":1}`),
		generalEvent(1, "acreSettings", `{"terrainLoss":1,"signalModel":2}`),
	)

	export := Build(data)
	require.Len(t, export.Events, 3)

	fps := export.Events[0][2].(map[string]any)
	assert.Equal(t, 47.25, fps["fps"])
	assert.Equal(t, 0, export.Events[0][0], "the clamped first sample stays on the timeline")

	tfar := export.Events[1][2].(map[string]any)
	assert.Equal(t, 7.0, tfar["terrainInterceptionCoefficient"])
	assert.Equal(t, 1.0, tfar["globalRadioRangeCoef"])

	acre := export.Events[2][2].(map[string]any)
	assert.Equal(t, 1.0, acre["terrainLoss"])
	assert.Equal(t, 2.0, acre["signalModel"])
}

func generalEvent(frame core.Frame, name, message string) core.GeneralEvent {
	return core.GeneralEvent{CaptureFrame: frame, Name: name, Message: message}
}

func snapshotMissionData(events ...core.GeneralEvent) *MissionData {
	return &MissionData{
		Mission:       &core.Mission{MissionName: "Snapshot test"},
		World:         &core.World{WorldName: "Altis"},
		Soldiers:      make(map[uint16]*SoldierRecord),
		Vehicles:      make(map[uint16]*VehicleRecord),
		Markers:       make(map[string]*MarkerRecord),
		GeneralEvents: events,
	}
}
