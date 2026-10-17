package v1

import (
	"github.com/OCAP2/extension/v5/pkg/core"
)

// missionMaxFrame is the latest internal frame that should appear in a v1
// export. Entity states are the usual source, but times and events can outlive
// the last position tick (a corpse that stopped updating, a delayed death
// snapshot, disconnect after pause). Those still have to sit on the timeline.
func missionMaxFrame(data *MissionData) core.Frame {
	var max core.Frame
	for _, record := range data.Soldiers {
		for _, state := range record.States {
			max = raiseFrame(max, state.CaptureFrame)
		}
	}
	for _, record := range data.Vehicles {
		for _, state := range record.States {
			max = raiseFrame(max, state.CaptureFrame)
		}
	}
	for _, ts := range data.TimeStates {
		max = raiseFrame(max, ts.CaptureFrame)
	}
	for _, evt := range data.GeneralEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	for _, evt := range data.RadioEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	for _, evt := range data.SectorEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	for _, evt := range data.EndMissionEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	for _, evt := range data.HitEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	for _, evt := range data.KillEvents {
		max = raiseFrame(max, evt.CaptureFrame)
	}
	return max
}

func raiseFrame(max, frame core.Frame) core.Frame {
	if frame > max {
		return frame
	}
	return max
}

// rewriteSnapshotDiffOf maps the internal 1-based `diffOf` stored inside a
// snapshot payload onto the 0-based v1 event frame the wrapper uses.
func rewriteSnapshotDiffOf(eventName string, message any) any {
	switch eventName {
	case "inventorySnapshot", "medicalSnapshot", "staminaSnapshot", "radioSnapshot":
	default:
		return message
	}
	obj, ok := message.(map[string]any)
	if !ok {
		return message
	}
	raw, exists := obj["diffOf"]
	if !exists {
		return message
	}
	internal, ok := jsonToFrame(raw)
	if !ok {
		return message
	}
	out := make(map[string]any, len(obj))
	for key, value := range obj {
		out[key] = value
	}
	out["diffOf"] = frameToV1(internal)
	return out
}

func jsonToFrame(raw any) (core.Frame, bool) {
	switch value := raw.(type) {
	case float64:
		return core.Frame(value), true
	case float32:
		return core.Frame(value), true
	case int:
		return core.Frame(value), true
	case int64:
		return core.Frame(value), true
	case uint:
		return core.Frame(value), true
	case uint64:
		return core.Frame(value), true
	default:
		return 0, false
	}
}
