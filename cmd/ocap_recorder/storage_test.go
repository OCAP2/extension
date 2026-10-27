package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/storage/websocket"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCreateWebSocketBackend(t *testing.T) {
	viper.Set("api.serverUrl", "https://example.test/ocap/")
	viper.Set("api.apiKey", "test-key")
	t.Cleanup(viper.Reset)
	b, err := createStorageBackend(config.StorageConfig{Type: "websocket"})
	require.NoError(t, err)
	require.IsType(t, (*websocket.Backend)(nil), b)
	_, uploads := b.(storage.Uploadable)
	require.False(t, uploads, "streaming proxy must not export or upload local recordings")
}

func TestStorageDefaultsRemainCompatible(t *testing.T) {
	previous := Logger
	Logger = slog.Default()
	t.Cleanup(func() { Logger = previous })
	b, err := createStorageBackend(config.StorageConfig{Type: "memory"})
	require.NoError(t, err)
	_, uploads := b.(storage.Uploadable)
	require.True(t, uploads)
	_, err = createStorageBackend(config.StorageConfig{Type: "not-real"})
	require.Error(t, err)
}

func TestWebSocketBackendRequiresSecret(t *testing.T) {
	viper.Set("api.apiKey", "")
	t.Cleanup(viper.Reset)
	_, err := createStorageBackend(config.StorageConfig{Type: "websocket"})
	require.Error(t, err)
}

// isolateStorage gives a test its own storage globals.
func isolateStorage(t *testing.T) *callbackRecorder {
	t.Helper()
	rec := &callbackRecorder{}
	a3interface.SetCallbackSinkForTest(rec.sink)
	// Logger is left alone: the package init still reads it.
	previousBackend, previousDispatcher, previousWorkers := storageBackend, eventDispatcher, workerManager
	t.Cleanup(func() {
		a3interface.SetCallbackSinkForTest(nil)
		storageBackend, eventDispatcher, workerManager = previousBackend, previousDispatcher, previousWorkers
		viper.Reset()
	})
	storageBackend = nil
	d, err := dispatcher.New(logging.NewDispatcherLogger(Logger))
	require.NoError(t, err)
	eventDispatcher = d
	return rec
}

func TestInitStorageReusesBackend(t *testing.T) {
	rec := isolateStorage(t)
	viper.Set("storage.type", "memory")
	viper.Set("storage.memory.outputDir", t.TempDir())

	require.NoError(t, initStorage())
	first := storageBackend
	require.NotNil(t, first)
	// The addon initializes storage again on the next mission load.
	require.NoError(t, initStorage())
	require.Same(t, first, storageBackend, "the next mission reuses the backend that ends the one left open")

	rec.mu.Lock()
	defer rec.mu.Unlock()
	oks := 0
	for _, c := range rec.calls {
		if c.function == ":STORAGE:OK:" {
			oks++
		}
	}
	require.Equal(t, 2, oks, "every :STORAGE:INIT: is answered")
}

// abandoningBackend is a fakeBackend that records AbandonMission calls.
type abandoningBackend struct {
	fakeBackend
	abandoned chan struct{}
}

func (a *abandoningBackend) AbandonMission() error {
	close(a.abandoned)
	return nil
}

func TestInitStorageAbandonsOpenMission(t *testing.T) {
	isolateStorage(t)
	viper.Set("storage.type", "websocket")
	backend := &abandoningBackend{abandoned: make(chan struct{})}
	storageBackend = backend

	// The next mission loads.
	require.NoError(t, initStorage())
	select {
	case <-backend.abandoned:
	case <-time.After(2 * time.Second):
		t.Fatal("the mission left open was not abandoned")
	}
	require.Same(t, storage.Backend(backend), storageBackend)
}

func TestStartStorageReportsFailure(t *testing.T) {
	rec := isolateStorage(t)
	viper.Set("storage.type", "not-real")

	startStorage()
	cb := rec.wait(t, ":STORAGE:ERROR:")
	require.Len(t, cb.data, 1)
	require.Nil(t, storageBackend)
}

func TestWebSocketURLPreservesPrefix(t *testing.T) {
	require.Equal(t, "wss://example.test/ocap", httpToWS("https://example.test/ocap/"))
}
