package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
	"github.com/OCAP2/extension/v5/pkg/core"
)

// isolateMissionStart records callbacks and gives a test its own parser and
// no storage. Logger is left alone: the package init still reads it.
func isolateMissionStart(t *testing.T, p parser.Service) *callbackRecorder {
	t.Helper()
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	previousParser, previousBackend := parserService, storageBackend
	t.Cleanup(func() {
		a3interface.SetCallbackSinkForTest(nil)
		parserService, storageBackend = previousParser, previousBackend
	})
	parserService = p
	storageBackend = nil
	return rec
}

func TestHandleNewMission_FailureIsReported(t *testing.T) {
	rec := isolateMissionStart(t, nil)

	_, err := handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-1"`}})
	require.Error(t, err)
	cb := rec.wait(t, ":MISSION:FAILED:")
	require.Len(t, cb.data, 2)
	assert.Contains(t, cb.data[0], "not initialized")
	assert.Equal(t, `"tok-1"`, cb.data[1], "the addon's registration token is echoed")

	rec.mu.Lock()
	rec.calls = nil
	rec.mu.Unlock()
	parserService = parser.NewParser(Logger)
	_, err = handleNewMission(dispatcher.Event{Args: []string{"not json"}})
	require.Error(t, err)
	cb = rec.wait(t, ":MISSION:FAILED:")
	assert.Equal(t, `""`, cb.data[1], "an addon without a token gets an empty one")
}

// acceptingParser starts every mission.
type acceptingParser struct{ parser.Service }

func (acceptingParser) ParseMission([]string) (core.Mission, core.World, error) {
	return core.Mission{MissionName: "M"}, core.World{WorldName: "W"}, nil
}

func TestHandleNewMission_WaitsForSaveOrDiscard(t *testing.T) {
	rec := isolateMissionStart(t, acceptingParser{})

	// The previous mission's discard was dispatched just before this start.
	saveState.backendBusy.Store(true)
	t.Cleanup(func() { saveState.backendBusy.Store(false) })
	done := make(chan error, 1)
	go func() {
		_, err := handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-3"`}})
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("the start ran before the discard finished")
	case <-time.After(100 * time.Millisecond):
	}
	saveState.backendBusy.Store(false)
	require.NoError(t, <-done)
	rec.wait(t, ":MISSION:OK:")
}

func TestHandleNewMission_RejectsStartOfUnloadedMission(t *testing.T) {
	rec := isolateMissionStart(t, acceptingParser{})
	previous := missionLoadedAt.Load()
	t.Cleanup(func() { missionLoadedAt.Store(previous) })

	// The start was dispatched, then the next mission loaded while it waited.
	dispatched := time.Now()
	loaded := dispatched.Add(time.Second)
	missionLoadedAt.Store(&loaded)
	_, err := handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-4"`}, Timestamp: dispatched})
	require.Error(t, err)
	cb := rec.wait(t, ":MISSION:FAILED:")
	assert.Equal(t, `"tok-4"`, cb.data[1])

	// A start dispatched after the load goes through.
	_, err = handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-5"`}, Timestamp: dispatched.Add(2 * time.Second)})
	require.NoError(t, err)
	rec.wait(t, ":MISSION:OK:")
}

func TestHandleNewMission_OKEchoesToken(t *testing.T) {
	rec := isolateMissionStart(t, acceptingParser{})

	_, err := handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-2"`}})
	require.NoError(t, err)
	cb := rec.wait(t, ":MISSION:OK:")
	assert.Equal(t, []string{`"OK"`, `"tok-2"`}, cb.data)
}

func TestHandleNewMission_DiscardAbandonedUntilKept(t *testing.T) {
	rec := isolateMissionStart(t, acceptingParser{})
	backend := &abandoningBackend{rec: rec}
	storageBackend = backend

	_, err := handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-6"`}})
	require.NoError(t, err)
	assert.Empty(t, backend.discard, "an addon that does not ask keeps an abandoned recording")

	_, err = handleNewMission(dispatcher.Event{Args: []string{"{}", "{}", `"tok-7"`, `"discard"`}})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, backend.discard)

	// The recording met the addon's save limits.
	d, err := dispatcher.New(logging.NewDispatcherLogger(Logger))
	require.NoError(t, err)
	registerLifecycleHandlers(d)
	_, err = d.Dispatch(dispatcher.Event{Command: ":MISSION:KEEP:"})
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, backend.discard)
}

// panickingParser fails every mission start with a panic.
type panickingParser struct{ parser.Service }

func (panickingParser) ParseMission([]string) (core.Mission, core.World, error) {
	panic("boom")
}

func TestHandleNewMission_PanicIsReported(t *testing.T) {
	rec := isolateMissionStart(t, panickingParser{})

	assert.Panics(t, func() { _, _ = handleNewMission(dispatcher.Event{}) }, "the dispatcher still sees the panic")
	cb := rec.wait(t, ":MISSION:FAILED:")
	require.Len(t, cb.data, 2)
	assert.Contains(t, cb.data[0], "panic")
}
