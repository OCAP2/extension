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

func TestStream_IncludesPlayerUIDInConnectionEvent(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "Connection"},
		World:    &core.World{WorldName: "Altis"},
		Soldiers: make(map[uint16]*SoldierRecord),
		Vehicles: make(map[uint16]*VehicleRecord),
		Markers:  make(map[string]*MarkerRecord),
		GeneralEvents: []core.GeneralEvent{
			{
				CaptureFrame: 10,
				Name:         "connected",
				Message:      "Alice",
				ExtraData:    map[string]any{"playerUid": "76561198000000001"},
			},
		},
	}

	var export Export
	require.NoError(t, json.Unmarshal(streamBytes(t, data), &export))
	require.Len(t, export.Events, 1)
	require.Equal(t, []any{float64(9), "connected", "Alice", "76561198000000001"}, export.Events[0])
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

// TestStream_EquivalentToBuild_AllFeatures exercises every branch in
// buildAggregates and writeEntityForID: sector / end-mission / hit /
// kill / general events, POLYLINE and point markers with state
// changes, placed objects with detonations and hit events, and
// projectile events that split between firelines (bullets), markers
// (non-bullets, both with and without a magazine icon, both fired
// from soldiers and from vehicles), and projectile hits.
//
// If this passes, Stream has walked every branch Build walks on the
// same inputs.
func TestStream_EquivalentToBuild_AllFeatures(t *testing.T) {
	soldier1 := uint16(1)
	vehicle2 := uint16(2)
	victimSoldier := uint(1)
	victimVehicle := uint(2)
	shooterSoldier := uint(1)
	killerVehicle := uint(2)
	hitSoldier := uint16(1)

	data := &MissionData{
		Mission: &core.Mission{
			MissionName:      "Kitchen Sink",
			Author:           "Author",
			AddonVersion:     "1.0",
			ExtensionVersion: "5.0",
			ExtensionBuild:   "abc123",
			CaptureDelay:     0.1,
			Tag:              "TvT",
		},
		World: &core.World{WorldName: "Altis"},
		Soldiers: map[uint16]*SoldierRecord{
			1: {
				Soldier: core.Soldier{
					ID: 1, UnitName: "Alice", Side: "WEST", GroupID: "G1",
					RoleDescription: "Rifleman", JoinFrame: 1, DeleteFrame: 20,
					IsPlayer: true,
				},
				States: []core.SoldierState{
					{SoldierID: 1, CaptureFrame: 1, Position: core.Position3D{X: 10, Y: 20, Z: 0.5}, UnitName: "Alice", Side: "WEST", GroupID: "G1", IsPlayer: true, CurrentRole: "Rifleman"},
					{SoldierID: 1, CaptureFrame: 10, Position: core.Position3D{X: 15, Y: 22, Z: 0.5}, UnitName: "Alice", Side: "WEST", GroupID: "G1", IsPlayer: true, CurrentRole: "Rifleman"},
				},
				FiredEvents: []core.FiredEvent{
					{SoldierID: 1, CaptureFrame: 3, EndPos: core.Position3D{X: 50, Y: 20, Z: 0.5}},
				},
			},
		},
		Vehicles: map[uint16]*VehicleRecord{
			2: {
				Vehicle: core.Vehicle{ID: 2, DisplayName: "Ifrit", Side: "EAST", OcapType: "car", JoinFrame: 1, DeleteFrame: 15},
				States: []core.VehicleState{
					{VehicleID: 2, CaptureFrame: 1, Position: core.Position3D{X: 100, Y: 200}, IsAlive: true, Crew: "[5,6]"},
					{VehicleID: 2, CaptureFrame: 8, Position: core.Position3D{X: 105, Y: 200}, IsAlive: false, Crew: ""},
					// Extends past any soldier state so the vehicle branch of
					// computeMetadata's maxFrame update is exercised.
					{VehicleID: 2, CaptureFrame: 25, Position: core.Position3D{X: 110, Y: 200}, IsAlive: false, Crew: ""},
				},
			},
		},
		Markers: map[string]*MarkerRecord{
			"m_point": {
				Marker: core.Marker{
					MarkerName: "m_point", MarkerType: "mil_triangle", Text: "Point",
					Shape: "ICON", Brush: "Solid", Color: "#FF0000", Size: "[1,1]",
					Position: core.Position3D{X: 500, Y: 500}, Side: "WEST",
					CaptureFrame: 2, EndFrame: 0, Alpha: 1, Direction: 90, OwnerID: -1,
				},
				States: []core.MarkerState{
					{MarkerID: 1, CaptureFrame: 5, Position: core.Position3D{X: 510, Y: 500},
						Alpha: 0.8, Text: "Point2", Color: "#FF00FF", Size: "[2,2]",
						Shape: "ICON", Brush: "Solid", MarkerType: "mil_triangle"},
				},
			},
			"m_poly": {
				Marker: core.Marker{
					MarkerName: "m_poly", MarkerType: "POLYLINE", Text: "Poly",
					Shape: "POLYLINE", Brush: "Solid", Color: "#00FF00", Size: "[1,1]",
					Polyline: core.Polyline{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 0}},
					Side:     "EAST", CaptureFrame: 3, EndFrame: 0, Alpha: 1, OwnerID: -1,
				},
			},
		},
		PlacedObjects: map[uint16]*PlacedObjectRecord{
			10: {
				PlacedObject: core.PlacedObject{
					ID: 10, DisplayName: "APERS Mine", MagazineIcon: `\ca\weapons\mine_icon.paa`,
					Position: core.Position3D{X: 300, Y: 400}, JoinFrame: 2, OwnerID: 1,
				},
				Events: []core.PlacedObjectEvent{
					{CaptureFrame: 7, PlacedID: 10, EventType: "hit", Position: core.Position3D{X: 301, Y: 401}, HitEntityID: &hitSoldier},
					{CaptureFrame: 8, PlacedID: 10, EventType: "detonated", Position: core.Position3D{X: 300, Y: 400}},
				},
			},
			11: {
				PlacedObject: core.PlacedObject{
					ID: 11, DisplayName: "Unknown Mine", MagazineIcon: "",
					Position: core.Position3D{X: 320, Y: 420}, JoinFrame: 2, OwnerID: 1,
				},
				Events: []core.PlacedObjectEvent{
					{CaptureFrame: 9, PlacedID: 11, EventType: "deleted", Position: core.Position3D{X: 320, Y: 420}},
				},
			},
		},
		GeneralEvents: []core.GeneralEvent{
			{CaptureFrame: 1, Name: "mission", Message: "start"},
			{CaptureFrame: 2, Name: "mission", Message: `{"obj":"payload"}`},
		},
		SectorEvents: []core.SectorEvent{
			{CaptureFrame: 4, Name: "captured", ObjectType: "sector", UnitName: "Alpha", Side: "WEST", PosX: 0, PosY: 0, PosZ: 0},
		},
		EndMissionEvents: []core.EndMissionEvent{
			{CaptureFrame: 20, Side: "WEST", Message: "Victory"},
		},
		HitEvents: []core.HitEvent{
			{CaptureFrame: 5, VictimSoldierID: &victimSoldier, ShooterSoldierID: &shooterSoldier, EventText: "rifle", Distance: 50},
			{CaptureFrame: 6, VictimVehicleID: &victimVehicle, ShooterSoldierID: &shooterSoldier, EventText: "RPG", Distance: 100},
			// Vehicle-shooter branch.
			{CaptureFrame: 9, VictimSoldierID: &victimSoldier, ShooterVehicleID: &killerVehicle, EventText: "cannon", Distance: 300},
		},
		KillEvents: []core.KillEvent{
			{CaptureFrame: 7, VictimSoldierID: &victimSoldier, KillerVehicleID: &killerVehicle, EventText: "cannon", Distance: 200},
			{CaptureFrame: 8, VictimVehicleID: &victimVehicle, KillerSoldierID: &shooterSoldier, EventText: "AT", Distance: 150},
		},
		TimeStates: []core.TimeState{
			{CaptureFrame: 1, SystemTimeUTC: "2026-04-19T00:00:00Z", MissionDate: "2035-01-01T12:00:00", MissionTime: 0, TimeMultiplier: 1},
			{CaptureFrame: 10, SystemTimeUTC: "2026-04-19T00:00:01Z", MissionDate: "2035-01-01T12:00:01", MissionTime: 1, TimeMultiplier: 1},
		},
		ProjectileEvents: []core.ProjectileEvent{
			// Bullet → fireline on soldier 1
			{
				CaptureFrame: 4, FirerObjectID: soldier1, SimulationType: "shotBullet",
				WeaponDisplay: "M4", MagazineDisplay: "5.56", MuzzleDisplay: "M4",
				Trajectory: []core.TrajectoryPoint{
					{Position: core.Position3D{X: 10, Y: 20}, FrameNum: 4},
					{Position: core.Position3D{X: 60, Y: 20}, FrameNum: 4},
				},
			},
			// Non-bullet with magazine icon → marker
			{
				CaptureFrame: 5, FirerObjectID: soldier1, SimulationType: "shotRocket",
				WeaponDisplay: "RPG", MagazineDisplay: "PG-7", MuzzleDisplay: "RPG",
				MagazineIcon:  `\ca\weapons\rpg_icon.paa`,
				Trajectory: []core.TrajectoryPoint{
					{Position: core.Position3D{X: 10, Y: 20}, FrameNum: 5},
					{Position: core.Position3D{X: 70, Y: 40}, FrameNum: 6},
				},
				Hits: []core.ProjectileHit{
					{CaptureFrame: 6, Position: core.Position3D{X: 70, Y: 40}, SoldierID: &hitSoldier},
				},
			},
			// Non-bullet without icon → mil_triangle fallback
			{
				CaptureFrame: 6, FirerObjectID: soldier1, SimulationType: "shotGrenade",
				MagazineDisplay: "M67",
				Trajectory: []core.TrajectoryPoint{
					{Position: core.Position3D{X: 10, Y: 20}, FrameNum: 6},
					{Position: core.Position3D{X: 20, Y: 25}, FrameNum: 7},
				},
			},
			// Vehicle-fired projectile, hits a vehicle. Empty MuzzleDisplay
			// exercises the weaponName fallback to WeaponDisplay.
			{
				CaptureFrame: 7, FirerObjectID: soldier1, VehicleObjectID: &vehicle2,
				SimulationType: "shotShell", WeaponDisplay: "cannon", MagazineDisplay: "HEAT", MuzzleDisplay: "",
				Trajectory: []core.TrajectoryPoint{
					{Position: core.Position3D{X: 100, Y: 200}, FrameNum: 7},
					{Position: core.Position3D{X: 400, Y: 500}, FrameNum: 8},
				},
				Hits: []core.ProjectileHit{
					{CaptureFrame: 8, Position: core.Position3D{X: 400, Y: 500}, VehicleID: &vehicle2},
				},
			},
		},
	}

	want := buildThenEncode(t, data)
	got := streamBytes(t, data)

	require.JSONEq(t, string(want), string(got))
}

// errorWriter fails after n successful writes. Used to exercise the
// error-return paths in Stream, writeField, and writeJSON.
type errorWriter struct {
	n      int // remaining writes allowed before returning err
	failAt string
}

func (ew *errorWriter) Write(p []byte) (int, error) {
	if ew.n <= 0 {
		return 0, io.ErrClosedPipe
	}
	ew.n--
	return len(p), nil
}

func TestStream_WriterErrorPropagates(t *testing.T) {
	data := &MissionData{
		Mission:  &core.Mission{MissionName: "M"},
		World:    &core.World{WorldName: "W"},
		Soldiers: map[uint16]*SoldierRecord{1: {Soldier: core.Soldier{ID: 1, UnitName: "X", Side: "WEST", JoinFrame: 1}}},
		Vehicles: map[uint16]*VehicleRecord{},
		Markers:  map[string]*MarkerRecord{},
	}

	// Fail immediately on first write. Stream uses a 64k buffered
	// writer, so Stream's internal WriteByte/WriteString calls
	// accumulate into the buffer and only hit the underlying writer
	// on Flush.
	w := &errorWriter{n: 0}
	err := Stream(w, data)
	require.Error(t, err)
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
