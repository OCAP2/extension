package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBackend is a minimal storage.Backend + storage.Uploadable implementation
// that only implements what the save path actually touches. Every other
// Backend method is a panic — if the test triggers one of those, the save
// path is doing something it shouldn't.
type fakeBackend struct {
	endErr      error
	endDuration time.Duration
	endTag      string
	path        string
	meta        core.UploadMetadata
}

func (f *fakeBackend) Init() error  { return nil }
func (f *fakeBackend) Close() error { return nil }
func (f *fakeBackend) StartMission(*core.Mission, *core.World) error {
	panic("unexpected StartMission")
}
func (f *fakeBackend) EndMission(tag string) error {
	f.endTag = tag
	if f.endDuration > 0 {
		time.Sleep(f.endDuration)
	}
	return f.endErr
}
func (f *fakeBackend) AddSoldier(*core.Soldier) error              { panic("no") }
func (f *fakeBackend) AddVehicle(*core.Vehicle) error              { panic("no") }
func (f *fakeBackend) AddMarker(*core.Marker) (uint, error)        { panic("no") }
func (f *fakeBackend) DeleteSoldier(uint16, core.Frame) error      { panic("no") }
func (f *fakeBackend) DeleteVehicle(uint16, core.Frame) error      { panic("no") }
func (f *fakeBackend) RecordSoldierState(*core.SoldierState) error { panic("no") }
func (f *fakeBackend) RecordVehicleState(*core.VehicleState) error { panic("no") }
func (f *fakeBackend) RecordMarkerState(*core.MarkerState) error   { panic("no") }
func (f *fakeBackend) DeleteMarker(*core.DeleteMarker) error       { panic("no") }
func (f *fakeBackend) RecordFiredEvent(*core.FiredEvent) error     { panic("no") }
func (f *fakeBackend) RecordProjectileEvent(*core.ProjectileEvent) error {
	panic("no")
}
func (f *fakeBackend) RecordGeneralEvent(*core.GeneralEvent) error { panic("no") }
func (f *fakeBackend) RecordSectorEvent(*core.SectorEvent) error   { panic("no") }
func (f *fakeBackend) RecordEndMissionEvent(*core.EndMissionEvent) error {
	panic("no")
}
func (f *fakeBackend) RecordHitEvent(*core.HitEvent) error     { panic("no") }
func (f *fakeBackend) RecordKillEvent(*core.KillEvent) error   { panic("no") }
func (f *fakeBackend) RecordChatEvent(*core.ChatEvent) error   { panic("no") }
func (f *fakeBackend) RecordRadioEvent(*core.RadioEvent) error { panic("no") }
func (f *fakeBackend) RecordTelemetryEvent(*core.TelemetryEvent) error {
	panic("no")
}
func (f *fakeBackend) RecordTimeState(*core.TimeState) error { panic("no") }
func (f *fakeBackend) RecordAce3DeathEvent(*core.Ace3DeathEvent) error {
	panic("no")
}
func (f *fakeBackend) RecordAce3UnconsciousEvent(*core.Ace3UnconsciousEvent) error {
	panic("no")
}
func (f *fakeBackend) AddPlacedObject(*core.PlacedObject) error { panic("no") }
func (f *fakeBackend) RecordPlacedObjectEvent(*core.PlacedObjectEvent) error {
	panic("no")
}
func (f *fakeBackend) SetFocusStart(core.Frame) error { panic("no") }
func (f *fakeBackend) SetFocusEnd(core.Frame) error   { panic("no") }

// Uploadable
func (f *fakeBackend) GetExportedFilePath() string            { return f.path }
func (f *fakeBackend) GetExportMetadata() core.UploadMetadata { return f.meta }

var (
	_ storage.Backend    = (*fakeBackend)(nil)
	_ storage.Uploadable = (*fakeBackend)(nil)
)

type recordedCallback struct {
	function string
	data     []string
}

type callbackRecorder struct {
	mu    sync.Mutex
	calls []recordedCallback
}

func (r *callbackRecorder) sink(name, function string, data ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCallback{function: function, data: append([]string(nil), data...)})
}

func (r *callbackRecorder) wait(t *testing.T, fn string) recordedCallback {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, c := range r.calls {
			if c.function == fn {
				r.mu.Unlock()
				return c
			}
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for callback %s", fn)
	return recordedCallback{}
}

// newTestSaveState returns a fresh missionSaveState suitable for tests.
func newTestSaveState() *missionSaveState {
	return &missionSaveState{}
}

func TestRunMissionSave_SuccessWithoutUpload(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &fakeBackend{path: "recordings/test.json.gz"}

	// apiClient = nil → no upload
	result, err := triggerMissionSave(state, backend, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	cb := rec.wait(t, ":MISSION:SAVED:")
	require.Len(t, cb.data, 2)
	assert.Equal(t, `"ok"`, cb.data[0])
	assert.Equal(t, `"recordings/test.json.gz"`, cb.data[1])
	assert.False(t, state.inFlight.Load())
}

func TestRunMissionSave_ForwardsTagOverride(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &fakeBackend{path: "recordings/tagged.json.gz"}

	result, err := triggerMissionSave(state, backend, nil, "SnatchAndGrab")
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	rec.wait(t, ":MISSION:SAVED:")
	assert.Equal(t, "SnatchAndGrab", backend.endTag)
}

func TestRunMissionSave_EmptyTagKeepsMissionStart(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &fakeBackend{path: "recordings/untagged.json.gz"}

	result, err := triggerMissionSave(state, backend, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	rec.wait(t, ":MISSION:SAVED:")
	assert.Equal(t, "", backend.endTag)
}

func TestRunMissionSave_EndMissionError(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &fakeBackend{endErr: errors.New("disk full")}

	result, err := triggerMissionSave(state, backend, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	cb := rec.wait(t, ":MISSION:SAVED:")
	require.Len(t, cb.data, 2)
	assert.Equal(t, `"error"`, cb.data[0])
	assert.Contains(t, cb.data[1], "disk full")
}

func TestRunMissionSave_RejectsConcurrent(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	// Make the first save slow so the second call lands while it's still running.
	backend := &fakeBackend{endDuration: 200 * time.Millisecond, path: "r.json.gz"}

	result1, err1 := triggerMissionSave(state, backend, nil, "")
	require.NoError(t, err1)
	assert.Equal(t, "queued", result1)

	_, err2 := triggerMissionSave(state, backend, nil, "")
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "save already in progress")

	rec.wait(t, ":MISSION:SAVED:")
}

func TestRunMissionSave_PanicIsRecovered(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	// A backend whose EndMission panics.
	panicBackend := &panickingBackend{fakeBackend: fakeBackend{path: "r.json.gz"}}

	result, err := triggerMissionSave(state, panicBackend, nil, "")
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	cb := rec.wait(t, ":MISSION:SAVED:")
	require.Len(t, cb.data, 2)
	assert.Equal(t, `"error"`, cb.data[0])
	assert.Contains(t, cb.data[1], "panic")
	assert.False(t, state.inFlight.Load(), "inFlight must be cleared even after panic")
}

type panickingBackend struct{ fakeBackend }

func (p *panickingBackend) EndMission(tag string) error {
	panic(fmt.Errorf("boom"))
}

// discardingBackend is a fakeBackend that also implements storage.Discardable.
type discardingBackend struct {
	fakeBackend
	discardErr   error
	discardPanic bool
	discards     int
}

func (d *discardingBackend) DiscardMission() error {
	d.discards++
	if d.discardPanic {
		panic("boom")
	}
	return d.discardErr
}

var _ storage.Discardable = (*discardingBackend)(nil)

func TestRunMissionDiscard_Success(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &discardingBackend{}

	result, err := triggerMissionDiscard(state, backend)
	require.NoError(t, err)
	assert.Equal(t, "queued", result)

	cb := rec.wait(t, ":MISSION:DISCARDED:")
	assert.Equal(t, []string{`"ok"`}, cb.data)
	assert.Equal(t, 1, backend.discards)
	assert.Empty(t, backend.endTag, "a discarded mission is not saved")
	assert.False(t, state.inFlight.Load())
}

func TestRunMissionDiscard_BackendKeepsData(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	_, err := triggerMissionDiscard(state, &fakeBackend{})
	require.NoError(t, err)

	cb := rec.wait(t, ":MISSION:DISCARDED:")
	assert.Equal(t, []string{`"kept"`}, cb.data)
	assert.False(t, state.inFlight.Load())
}

func TestRunMissionDiscard_StorageKeepsMission(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	// For example a websocket server too old to drop it.
	state := newTestSaveState()
	backend := &discardingBackend{discardErr: fmt.Errorf("old server: %w", storage.ErrMissionKept)}
	_, err := triggerMissionDiscard(state, backend)
	require.NoError(t, err)

	cb := rec.wait(t, ":MISSION:DISCARDED:")
	assert.Equal(t, []string{`"kept"`}, cb.data)
	assert.Equal(t, 1, backend.discards)
}

func TestRunMissionDiscard_Error(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	_, err := triggerMissionDiscard(state, &discardingBackend{discardErr: errors.New("server gone")})
	require.NoError(t, err)

	cb := rec.wait(t, ":MISSION:DISCARDED:")
	require.Len(t, cb.data, 2)
	assert.Equal(t, `"error"`, cb.data[0])
	assert.Contains(t, cb.data[1], "server gone")
}

func TestRunMissionDiscard_PanicIsRecovered(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	_, err := triggerMissionDiscard(state, &discardingBackend{discardPanic: true})
	require.NoError(t, err)

	cb := rec.wait(t, ":MISSION:DISCARDED:")
	require.Len(t, cb.data, 2)
	assert.Equal(t, `"error"`, cb.data[0])
	assert.Contains(t, cb.data[1], "panic")
	assert.False(t, state.inFlight.Load(), "inFlight must be cleared even after panic")
}

func TestRunMissionDiscard_RejectedWhileSaving(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })

	state := newTestSaveState()
	backend := &discardingBackend{fakeBackend: fakeBackend{endDuration: 200 * time.Millisecond, path: "r.json.gz"}}

	_, err := triggerMissionSave(state, backend, nil, "")
	require.NoError(t, err)
	_, err = triggerMissionDiscard(state, backend)
	require.Error(t, err, "a mission is either saved or discarded")
	assert.Contains(t, err.Error(), "already in progress")

	rec.wait(t, ":MISSION:SAVED:")
	assert.Zero(t, backend.discards)
}

func TestTriggerMissionDiscard_NoBackend(t *testing.T) {
	_, err := triggerMissionDiscard(newTestSaveState(), nil)
	require.Error(t, err)
}
