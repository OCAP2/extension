package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSnapshotManager(t *testing.T) (*dispatcher.Dispatcher, *mockBackend) {
	t.Helper()
	d, _ := newTestDispatcher(t)
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parser.NewParser(slog.New(slog.NewTextHandler(io.Discard, nil))),
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)
	return d, backend
}

// serverFps is an additive generic event: the sampler sends one at the first
// capture frame and every 10 seconds after, and the web Stats panel reads the
// payload straight out of the generic message field.
func TestServerFpsSamplesRecordedAsGeneralEvents(t *testing.T) {
	d, backend := newSnapshotManager(t)

	samples := []struct {
		frame string
		fps   float64
	}{
		{"0", 51.2},   // clamped first sample, before the capture loop ran
		{"10", 48.75},
		{"20", 22.5},
	}
	for _, sample := range samples {
		_, err := d.Dispatch(dispatcher.Event{
			Command: ":EVENT:GENERAL:",
			Args:    []string{sample.frame, "serverFps", fmt.Sprintf(`{"fps":%v}`, sample.fps)},
		})
		require.NoError(t, err)
	}

	waitFor(t, func() bool {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		return len(backend.generalEvents) == len(samples)
	}, "timed out waiting for serverFps samples")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	for i, sample := range samples {
		recorded := backend.generalEvents[i]
		assert.Equal(t, "serverFps", recorded.Name)

		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(recorded.Message), &payload))
		assert.Equal(t, sample.fps, payload["fps"])
	}
	assert.Equal(t, uint(0), uint(backend.generalEvents[0].CaptureFrame))
	assert.Equal(t, uint(20), uint(backend.generalEvents[2].CaptureFrame))
}

func TestWeatherSamplesRecordedAsGeneralEvents(t *testing.T) {
	d, backend := newSnapshotManager(t)

	payload := `{"overcast":0.42,"rain":0.1,"fog":0.2,"windSpeed":3.4,"windDir":247,"humidity":0.68,"moonPhase":0.81,"temperature":16.3,"ace":true}`
	samples := []string{"0", "60", "120"}
	for _, frame := range samples {
		_, err := d.Dispatch(dispatcher.Event{
			Command: ":EVENT:GENERAL:",
			Args:    []string{frame, "weather", payload},
		})
		require.NoError(t, err)
	}

	waitFor(t, func() bool {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		return len(backend.generalEvents) == len(samples)
	}, "timed out waiting for weather samples")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	for i, frame := range samples {
		recorded := backend.generalEvents[i]
		assert.Equal(t, "weather", recorded.Name)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(recorded.Message), &decoded))
		assert.Equal(t, 0.42, decoded["overcast"])
		assert.Equal(t, 0.81, decoded["moonPhase"])
		assert.Equal(t, true, decoded["ace"])
		assert.Equal(t, uint(mustAtoi(frame)), uint(recorded.CaptureFrame))
	}
}

func mustAtoi(value string) int {
	n := 0
	fmt.Sscanf(value, "%d", &n)
	return n
}

// Follow-up snapshots are diffs against the previous payload of the same kind,
// so the recorded order inside one command decides whether the chain rebuilds.
// Each player command owns its own buffer, drained by a single goroutine.
func TestPlayerSnapshotBurstKeepsRecordedOrder(t *testing.T) {
	d, backend := newSnapshotManager(t)

	const burst = 50
	for i := 0; i < burst; i++ {
		payload := fmt.Sprintf(`{"unitId":4,"diffOf":%d,"set":{"massUnits":%d}}`, i, 600+i)
		if i == 0 {
			payload = `{"unitId":4,"massUnits":600,"reason":"periodic"}`
		}
		_, err := d.Dispatch(dispatcher.Event{
			Command: ":PLAYER:INVENTORY:",
			Args:    []string{fmt.Sprint(i * 30), "inventorySnapshot", payload},
		})
		require.NoError(t, err, "snapshot %d was dropped", i)
	}

	waitFor(t, func() bool {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		return len(backend.generalEvents) == burst
	}, "timed out waiting for the snapshot burst")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	for i := 0; i < burst; i++ {
		recorded := backend.generalEvents[i]
		assert.Equal(t, "inventorySnapshot", recorded.Name)
		assert.Equal(t, uint(i*30), uint(recorded.CaptureFrame), "snapshot %d out of order", i)
	}

	var keyframe map[string]any
	require.NoError(t, json.Unmarshal([]byte(backend.generalEvents[0].Message), &keyframe))
	assert.Equal(t, "periodic", keyframe["reason"])
	assert.NotContains(t, keyframe, "diffOf")

	var last map[string]any
	require.NoError(t, json.Unmarshal([]byte(backend.generalEvents[burst-1].Message), &last))
	assert.Equal(t, float64(burst-1), last["diffOf"])
}

// The four player commands are registered separately from :EVENT:GENERAL: so a
// medical burst cannot starve the inventory queue. Each one has to be routed to
// the general-event handler, otherwise the payload never reaches storage.
func TestPlayerSnapshotCommandsAreSeparatelyRegistered(t *testing.T) {
	d, backend := newSnapshotManager(t)

	commands := map[string]string{
		":PLAYER:INVENTORY:": "inventorySnapshot",
		":PLAYER:MEDICAL:":   "medicalSnapshot",
		":PLAYER:STAMINA:":   "staminaSnapshot",
		":PLAYER:RADIO:":     "radioSnapshot",
	}
	for command, eventType := range commands {
		require.True(t, d.HasHandler(command), "%s is not registered", command)
		_, err := d.Dispatch(dispatcher.Event{
			Command: command,
			Args:    []string{"60", eventType, `{"unitId":4}`},
		})
		require.NoError(t, err)
	}

	waitFor(t, func() bool {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		return len(backend.generalEvents) == len(commands)
	}, "timed out waiting for the four snapshot kinds")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	seen := map[string]bool{}
	for _, recorded := range backend.generalEvents {
		seen[recorded.Name] = true
	}
	for _, eventType := range commands {
		assert.True(t, seen[eventType], "%s never reached the backend", eventType)
	}
}
