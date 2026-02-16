package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/core"
)

// Compile-time interface check.
var _ storage.Backend = (*Backend)(nil)

// testServer creates an httptest server that upgrades to WebSocket,
// records received messages, and sends acks for start_mission/end_mission.
func testServer(t *testing.T) (*httptest.Server, *messageLog) {
	t.Helper()
	ml := &messageLog{}

	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer c.Close()

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}

			var env Envelope
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			ml.add(env)

			// Ack start_mission and end_mission.
			if env.Type == TypeStartMission || env.Type == TypeEndMission {
				ack := AckMessage{Type: "ack", For: env.Type}
				data, _ := json.Marshal(ack)
				if err := c.WriteMessage(ws.TextMessage, data); err != nil {
					return
				}
			}
		}
	}))

	return srv, ml
}

type messageLog struct {
	mu       sync.Mutex
	messages []Envelope
}

func (m *messageLog) add(env Envelope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, env)
}

func (m *messageLog) all() []Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Envelope, len(m.messages))
	copy(cp, m.messages)
	return cp
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestStartAndEndMission(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "test"})
	require.NoError(t, b.Init())
	defer b.Close()

	mission := &core.Mission{MissionName: "TestMission", Tag: "TvT"}
	world := &core.World{WorldName: "Altis"}
	require.NoError(t, b.StartMission(mission, world))

	require.NoError(t, b.EndMission())

	msgs := ml.all()
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, TypeStartMission, msgs[0].Type)
	assert.Equal(t, TypeEndMission, msgs[len(msgs)-1].Type)
}

func TestFireAndForgetMessages(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	mission := &core.Mission{MissionName: "M"}
	world := &core.World{WorldName: "W"}
	require.NoError(t, b.StartMission(mission, world))

	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Alpha 1-1"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 100, ClassName: "B_MRAP_01_F"}))
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 1}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 100, CaptureFrame: 1}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"}))
	require.NoError(t, b.RecordServerFpsEvent(&core.ServerFpsEvent{FpsAverage: 50}))

	require.NoError(t, b.EndMission())

	// Give a moment for all messages to arrive at server.
	time.Sleep(50 * time.Millisecond)

	msgs := ml.all()

	types := make(map[string]int)
	for _, m := range msgs {
		types[m.Type]++
	}

	assert.Equal(t, 1, types[TypeStartMission])
	assert.Equal(t, 1, types[TypeEndMission])
	assert.Equal(t, 1, types[TypeAddSoldier])
	assert.Equal(t, 1, types[TypeAddVehicle])
	assert.Equal(t, 1, types[TypeSoldierState])
	assert.Equal(t, 1, types[TypeVehicleState])
	assert.Equal(t, 1, types[TypeGeneralEvent])
	assert.Equal(t, 1, types[TypeServerFps])
}

func TestAddMarkerAssignsID(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	m1 := &core.Marker{MarkerName: "marker1"}
	m2 := &core.Marker{MarkerName: "marker2"}

	require.NoError(t, b.AddMarker(m1))
	require.NoError(t, b.AddMarker(m2))

	assert.Equal(t, uint(1), m1.ID)
	assert.Equal(t, uint(2), m2.ID)
}

func TestEnvelopeSerialization(t *testing.T) {
	payload := DeleteMarkerPayload{Name: "mrk1", EndFrame: 42}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	env := Envelope{Type: TypeDeleteMarker, Payload: raw}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, TypeDeleteMarker, decoded.Type)

	var dp DeleteMarkerPayload
	require.NoError(t, json.Unmarshal(decoded.Payload, &dp))
	assert.Equal(t, "mrk1", dp.Name)
	assert.Equal(t, uint(42), dp.EndFrame)
}
