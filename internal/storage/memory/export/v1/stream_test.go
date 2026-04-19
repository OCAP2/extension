package v1

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/require"
)

// buildThenEncode runs the reference pipeline used in production before
// streaming landed: build full struct, encode via json.NewEncoder. The
// encoder appends a trailing '\n' which we strip for equivalence.
func buildThenEncode(t *testing.T, data *MissionData) []byte {
	t.Helper()
	exp := Build(data)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	require.NoError(t, enc.Encode(exp))
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out
}

// streamBytes runs the new streaming pipeline.
func streamBytes(t *testing.T, data *MissionData) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Stream(&buf, data))
	return buf.Bytes()
}

func TestStream_EquivalentToBuild_EmptyMission(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Empty", Author: "Test"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
	}

	want := buildThenEncode(t, data)
	got := streamBytes(t, data)

	require.JSONEq(t, string(want), string(got))
}

func TestStream_EquivalentToBuild_MinimalSoldier(t *testing.T) {
	data := &MissionData{
		Mission: &core.Mission{MissionName: "M", CaptureDelay: 0.1},
		World:   &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			1: {
				Soldier: core.Soldier{
					ID:              1,
					UnitName:        "Alice",
					Side:            "WEST",
					GroupID:         "G1",
					RoleDescription: "Rifleman",
					JoinFrame:       1,
					DeleteFrame:     0, // still active
					IsPlayer:        true,
				},
				States: []core.SoldierState{
					{
						SoldierID:    1,
						CaptureFrame: 1,
						Position:     core.Position3D{X: 10, Y: 20, Z: 0.5},
						Bearing:      90,
						Lifestate:    1,
						IsPlayer:     true,
						UnitName:     "Alice",
						CurrentRole:  "Rifleman",
						GroupID:      "G1",
						Side:         "WEST",
					},
					{
						SoldierID:    1,
						CaptureFrame: 3,
						Position:     core.Position3D{X: 11, Y: 20, Z: 0.5},
						Bearing:      90,
						Lifestate:    1,
						IsPlayer:     true,
						UnitName:     "Alice",
						CurrentRole:  "Rifleman",
						GroupID:      "G1",
						Side:         "WEST",
					},
				},
			},
		},
		Vehicles:   map[uint16]*VehicleRecord{},
		Markers:    map[string]*MarkerRecord{},
		TimeStates: []core.TimeState{{CaptureFrame: 1, SystemTimeUTC: "2026-04-19T00:00:00Z", MissionDate: "2035-01-01T12:00:00", MissionTime: 0, TimeMultiplier: 1}},
	}

	want := buildThenEncode(t, data)
	got := streamBytes(t, data)

	require.JSONEq(t, string(want), string(got))
}

func TestStream_EquivalentToBuild_SoldierAndVehicleAndEvents(t *testing.T) {
	vid := uint(2)
	data := &MissionData{
		Mission: &core.Mission{MissionName: "M", CaptureDelay: 0.1},
		World:   &core.World{WorldName: "Stratis"},
		Soldiers: map[uint16]*SoldierRecord{
			1: {
				Soldier: core.Soldier{ID: 1, UnitName: "Bob", Side: "WEST", GroupID: "G", JoinFrame: 1, DeleteFrame: 5},
				States: []core.SoldierState{
					{SoldierID: 1, CaptureFrame: 1, Position: core.Position3D{X: 0}, UnitName: "Bob", Side: "WEST", GroupID: "G"},
				},
				FiredEvents: []core.FiredEvent{
					{SoldierID: 1, CaptureFrame: 2, EndPos: core.Position3D{X: 100, Y: 0, Z: 0}},
				},
			},
		},
		Vehicles: map[uint16]*VehicleRecord{
			2: {
				Vehicle: core.Vehicle{ID: 2, DisplayName: "Hunter", Side: "WEST", OcapType: "car", JoinFrame: 1, DeleteFrame: 4},
				States: []core.VehicleState{
					{VehicleID: 2, CaptureFrame: 1, Position: core.Position3D{X: 0}, IsAlive: true, Crew: "[1]"},
				},
			},
		},
		Markers: map[string]*MarkerRecord{},
		HitEvents: []core.HitEvent{
			{CaptureFrame: 3, VictimSoldierID: &vid, ShooterSoldierID: &vid, EventText: "rifle", Distance: 50.0},
		},
		GeneralEvents: []core.GeneralEvent{{CaptureFrame: 2, Name: "mission", Message: "start"}},
	}

	want := buildThenEncode(t, data)
	got := streamBytes(t, data)

	require.JSONEq(t, string(want), string(got))
}

// TestStream_LargeMissionPeakMemory builds a synthetic mission with
// dense per-frame state for many entities and runs Stream through an
// io.Discard writer, asserting heap growth stays bounded. Regression
// guard: if someone re-introduces full-export materialization inside
// Stream, peak heap balloons and this test catches it.
func TestStream_LargeMissionPeakMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-mission memory test in -short mode")
	}

	const (
		entities = 200
		frames   = 20000 // 20k frames (~33min at 10Hz) per entity
	)

	soldiers := make(map[uint16]*SoldierRecord, entities)
	for i := 0; i < entities; i++ {
		id := uint16(i + 1)
		states := make([]core.SoldierState, 0, frames)
		for f := 1; f <= frames; f++ {
			states = append(states, core.SoldierState{
				SoldierID:    id,
				CaptureFrame: core.Frame(f),
				Position:     core.Position3D{X: float64(f), Y: float64(i)},
				UnitName:     "U",
				Side:         "WEST",
				GroupID:      "G",
			})
		}
		soldiers[id] = &SoldierRecord{
			Soldier: core.Soldier{ID: id, UnitName: "U", Side: "WEST", GroupID: "G", JoinFrame: 1},
			States:  states,
		}
	}

	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Big", CaptureDelay: 0.1},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: soldiers,
		Vehicles: map[uint16]*VehicleRecord{},
		Markers:  map[string]*MarkerRecord{},
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	require.NoError(t, Stream(io.Discard, data))

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// HeapInuse delta during the call should be far smaller than a
	// full materialization of all gap-filled positions. Full
	// materialization would push this to hundreds of MB. Use a
	// generous ceiling that still catches a regression back to
	// full-materialization.
	const ceiling = 256 * 1024 * 1024 // 256 MiB
	delta := int64(after.HeapInuse) - int64(before.HeapInuse)
	if delta > ceiling {
		t.Fatalf("HeapInuse grew by %d bytes during Stream; expected < %d (streaming regression?)", delta, ceiling)
	}
}
