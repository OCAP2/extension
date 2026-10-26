package main

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
)

func TestHandleNewMission_FailureIsReported(t *testing.T) {
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	t.Cleanup(func() { a3interface.SetCallbackSinkForTest(nil) })
	previousLogger, previousParser := Logger, parserService
	t.Cleanup(func() { Logger, parserService = previousLogger, previousParser })
	Logger = slog.Default()

	parserService = nil
	_, err := handleNewMission(dispatcher.Event{})
	require.Error(t, err)
	cb := rec.wait(t, ":MISSION:FAILED:")
	require.Len(t, cb.data, 1)
	assert.Contains(t, cb.data[0], "not initialized")

	rec.calls = nil
	parserService = parser.NewParser(Logger)
	_, err = handleNewMission(dispatcher.Event{Args: []string{"not json"}})
	require.Error(t, err)
	rec.wait(t, ":MISSION:FAILED:")
}
