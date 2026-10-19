package websocket

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
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

			var env streaming.Envelope
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			ml.add(env)

			// Ack start_mission and end_mission.
			if env.Type == streaming.TypeStartMission || env.Type == streaming.TypeEndMission {
				ack := streaming.AckMessage{Type: "ack", For: env.Type}
				data, err := json.Marshal(ack)
				require.NoError(t, err)
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
	messages []streaming.Envelope
}

func (m *messageLog) add(env streaming.Envelope) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, env)
}

func (m *messageLog) all() []streaming.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]streaming.Envelope, len(m.messages))
	copy(cp, m.messages)
	return cp
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// --- Backend lifecycle tests ---

func TestStartAndEndMission(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "test"})
	require.NoError(t, b.Init())
	defer b.Close()

	mission := &core.Mission{MissionName: "TestMission", Tag: "TvT"}
	world := &core.World{WorldName: "Altis"}
	require.NoError(t, b.StartMission(mission, world))

	require.NoError(t, b.EndMission(""))

	msgs := ml.all()
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, streaming.TypeStartMission, msgs[0].Type)
	assert.Equal(t, streaming.TypeEndMission, msgs[len(msgs)-1].Type)
}

func TestEndMissionTagPayload(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "test", SaveTimeout: 5 * time.Second})
	require.NoError(t, b.Init())
	defer b.Close()

	mission := &core.Mission{MissionName: "TaggedMission", Tag: "TvT"}
	world := &core.World{WorldName: "Altis"}

	lastEnd := func() *streaming.Envelope {
		var env *streaming.Envelope
		for _, m := range ml.all() {
			if m.Type == streaming.TypeEndMission {
				c := m
				env = &c
			}
		}
		return env
	}

	// Empty tag → null payload (byte-identical to the previous wire format).
	require.NoError(t, b.StartMission(mission, world))
	require.NoError(t, b.EndMission(""))
	env := lastEnd()
	require.NotNil(t, env)
	assert.Equal(t, "null", string(env.Payload))

	// Non-empty tag → {"tag":"..."} payload.
	require.NoError(t, b.StartMission(mission, world))
	require.NoError(t, b.EndMission("SnatchAndGrab"))
	env = lastEnd()
	require.NotNil(t, env)
	var payload streaming.EndMissionPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "SnatchAndGrab", payload.Tag)
}

func TestAllMessageTypes(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "M"}, &core.World{WorldName: "W"}))

	// Entity registration
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, UnitName: "Alpha 1-1"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 100, ClassName: "B_MRAP_01_F"}))
	_, err := b.AddMarker(&core.Marker{MarkerName: "m1"})
	require.NoError(t, err)

	// State updates
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 1}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 100, CaptureFrame: 1}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 1, CaptureFrame: 1}))
	require.NoError(t, b.DeleteMarker(&core.DeleteMarker{Name: "m1", EndFrame: 10}))
	require.NoError(t, b.DeleteSoldier(1, 50))
	require.NoError(t, b.DeleteVehicle(100, 60))

	// Events
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{SoldierID: 1, Weapon: "arifle_MX_F"}))
	require.NoError(t, b.RecordProjectileEvent(&core.ProjectileEvent{FirerObjectID: 1}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"}))
	require.NoError(t, b.RecordSectorEvent(&core.SectorEvent{Name: "captured", ObjectType: "sector", UnitName: "Alpha"}))
	require.NoError(t, b.RecordEndMissionEvent(&core.EndMissionEvent{Side: "WEST", Message: "BLUFOR wins"}))
	require.NoError(t, b.RecordHitEvent(&core.HitEvent{EventText: "hit"}))
	require.NoError(t, b.RecordKillEvent(&core.KillEvent{EventText: "killed"}))
	require.NoError(t, b.RecordChatEvent(&core.ChatEvent{Message: "hello"}))
	require.NoError(t, b.RecordRadioEvent(&core.RadioEvent{Radio: "ACRE"}))
	require.NoError(t, b.RecordTelemetryEvent(&core.TelemetryEvent{CaptureFrame: 1, FpsAverage: 45}))
	require.NoError(t, b.RecordTimeState(&core.TimeState{MissionTime: 120}))
	require.NoError(t, b.RecordAce3DeathEvent(&core.Ace3DeathEvent{SoldierID: 1, Reason: "bleeding"}))
	require.NoError(t, b.RecordAce3UnconsciousEvent(&core.Ace3UnconsciousEvent{SoldierID: 1, IsUnconscious: true}))
	require.NoError(t, b.AddPlacedObject(&core.PlacedObject{ID: 200, ClassName: "APERSMine"}))
	require.NoError(t, b.RecordPlacedObjectEvent(&core.PlacedObjectEvent{PlacedID: 200, EventType: "detonated"}))
	require.NoError(t, b.SetFocusStart(core.Frame(100)))
	require.NoError(t, b.SetFocusEnd(core.Frame(200)))

	require.NoError(t, b.EndMission(""))

	// Give a moment for all messages to arrive at server.
	time.Sleep(50 * time.Millisecond)

	msgs := ml.all()
	types := make(map[string]int)
	for _, m := range msgs {
		types[m.Type]++
	}

	// Every message type should appear exactly once.
	for _, typ := range streaming.AllMessageTypes {
		assert.Equalf(t, 1, types[typ], "expected exactly 1 message of type %q", typ)
	}
}

func TestAddMarkerAssignsID(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	m1 := &core.Marker{MarkerName: "marker1"}
	m2 := &core.Marker{MarkerName: "marker2"}

	id1, err := b.AddMarker(m1)
	require.NoError(t, err)
	id2, err := b.AddMarker(m2)
	require.NoError(t, err)

	assert.Equal(t, uint(1), id1)
	assert.Equal(t, uint(2), id2)
}

func TestEnvelopeSerialization(t *testing.T) {
	payload := core.DeleteMarker{Name: "mrk1", EndFrame: 42}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	env := streaming.Envelope{Type: streaming.TypeDeleteMarker, Payload: raw}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded streaming.Envelope
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, streaming.TypeDeleteMarker, decoded.Type)

	var dp core.DeleteMarker
	require.NoError(t, json.Unmarshal(decoded.Payload, &dp))
	assert.Equal(t, "mrk1", dp.Name)
	assert.Equal(t, core.Frame(42), dp.EndFrame)
}

func TestMarkerIDResetsAfterEndMission(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
	m1 := &core.Marker{MarkerName: "a"}
	id1, err := b.AddMarker(m1)
	require.NoError(t, err)
	assert.Equal(t, uint(1), id1)
	require.NoError(t, b.EndMission(""))

	// After EndMission, marker IDs should reset.
	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
	m2 := &core.Marker{MarkerName: "b"}
	id2, err := b.AddMarker(m2)
	require.NoError(t, err)
	assert.Equal(t, uint(1), id2)
	require.NoError(t, b.EndMission(""))
}

// --- Dial / close edge cases ---

func TestDialFailure(t *testing.T) {
	b := New(Config{URL: "ws://127.0.0.1:1", Secret: "s"})
	err := b.Init()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "websocket dial failed")
}

func TestDialInvalidURL(t *testing.T) {
	b := New(Config{URL: "://bad", Secret: "s"})
	err := b.Init()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid websocket URL")
}

func TestCloseIdempotent(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())

	require.NoError(t, b.Close())
	require.NoError(t, b.Close()) // second close should not error
}

func TestCloseNeverDialed(t *testing.T) {
	c := newConnection(slog.Default(), connOptions{})
	require.NoError(t, c.close()) // conn is nil, should still succeed
}

// --- ack waiting edge cases ---

// silentServer accepts WebSocket connections and never answers.
func silentServer(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
}

func TestAckTimeout(t *testing.T) {
	srv := silentServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	err := b.sendEnvelopeAndWait(streaming.TypeEndMission, nil, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for ack")
}

func TestSendAndWaitClosedConnection(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	require.NoError(t, b.Close())

	err := b.sendEnvelopeAndWait(streaming.TypeEndMission, nil, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

func TestWaitAckClosedWhileWaiting(t *testing.T) {
	c := newConnection(slog.Default(), connOptions{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = c.close()
	}()
	_, err := c.waitAck(streaming.TypeEndMission, time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed while waiting")
}

// --- marshalEnvelope error ---

func TestMarshalEnvelopeError(t *testing.T) {
	// Channels cannot be JSON-marshaled.
	_, err := marshalEnvelope("test", make(chan int), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal test payload")
}

func TestMarshalEnvelopeSeq(t *testing.T) {
	data, err := marshalEnvelope(streaming.TypeAddSoldier, &core.Soldier{ID: 1}, 7)
	require.NoError(t, err)
	var env streaming.Envelope
	require.NoError(t, json.Unmarshal(data, &env))
	assert.Equal(t, uint64(7), env.Seq)

	// Zero omits the field, keeping the legacy wire shape.
	data, err = marshalEnvelope(streaming.TypeResume, streaming.ResumePayload{MissionID: "m"}, 0)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"seq"`)
}

func TestSendEnvelopeError(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	err := b.sendEnvelope("test", make(chan int))
	require.Error(t, err)
}

func TestSendEnvelopeAndWaitError(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	err := b.sendEnvelopeAndWait("test", make(chan int), time.Second)
	require.Error(t, err)
}

func TestSendAfterEndMissionFails(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "X"}, &core.World{WorldName: "Y"}))
	require.NoError(t, b.EndMission(""))
	err := b.AddSoldier(&core.Soldier{ID: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mission already ended")
}

func TestSendBeforeStartIsDiscarded(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1}))
	require.NoError(t, b.StartMission(&core.Mission{MissionName: "X"}, &core.World{WorldName: "Y"}))
	require.NoError(t, b.EndMission(""))

	for _, m := range ml.all() {
		assert.NotEqual(t, streaming.TypeAddSoldier, m.Type, "pre-mission envelopes are not sent")
	}
}

func TestStartMissionRegistersStart(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "X"}, &core.World{WorldName: "Y"}))

	b.conn.mu.Lock()
	start, resume := b.conn.startData, b.conn.resumeData
	b.conn.mu.Unlock()
	require.NotNil(t, start, "StartMission should keep the serialized start for reconnects")
	require.NotNil(t, resume)

	msgs := ml.all()
	require.NotEmpty(t, msgs)
	assert.Equal(t, uint64(1), msgs[0].Seq, "start_mission is envelope 1")
	var payload streaming.StartMissionPayload
	require.NoError(t, json.Unmarshal(msgs[0].Payload, &payload))
	assert.NotEmpty(t, payload.MissionID)
	assert.Equal(t, b.producerID, payload.ProducerID)
	assert.Equal(t, uint8(streaming.ProtocolVersionResume), payload.ProtocolVersion)

	require.NoError(t, b.EndMission(""))

	b.conn.mu.Lock()
	start = b.conn.startData
	b.conn.mu.Unlock()
	assert.Nil(t, start, "EndMission should forget the mission")
}

func TestMissionIDsAreUniquePerMission(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	for range 2 {
		require.NoError(t, b.StartMission(&core.Mission{MissionName: "X"}, &core.World{WorldName: "Y"}))
		require.NoError(t, b.EndMission(""))
	}
	var ids []string
	producers := map[string]bool{}
	for _, m := range ml.all() {
		if m.Type == streaming.TypeStartMission {
			var p streaming.StartMissionPayload
			require.NoError(t, json.Unmarshal(m.Payload, &p))
			ids = append(ids, p.MissionID)
			producers[p.ProducerID] = true
		}
	}
	require.Len(t, ids, 2)
	assert.NotEqual(t, ids[0], ids[1])
	assert.Len(t, producers, 1, "one extension process keeps one producer id")
}

func TestNewIDIsUUIDv4(t *testing.T) {
	id := newID()
	require.Len(t, id, 36)
	assert.Equal(t, byte('4'), id[14])
	assert.Contains(t, "89ab", string(id[19]))
	assert.NotEqual(t, id, newID())
}

// --- buffer limit ---

func TestBufferFullRejectsEnvelope(t *testing.T) {
	srv := silentServer(t)
	defer srv.Close()

	// A tiny buffer holds the start and little else.
	b := New(Config{URL: wsURL(srv), Secret: "s", BufferBytes: 4000})
	require.NoError(t, b.Init())
	defer b.Close()

	// Start without an ack: mark the mission active by hand.
	require.NoError(t, b.conn.beginMission(func(seq uint64) ([]byte, error) {
		return marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{Mission: &core.Mission{}, World: &core.World{}}, seq)
	}, nil, nil))
	b.conn.setResumable(true) // retain everything until acknowledged
	b.mu.Lock()
	b.active = true
	b.mu.Unlock()

	var full error
	for i := 0; i < 100 && full == nil; i++ {
		full = b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(i)})
	}
	require.ErrorIs(t, full, errBufferFull)
	_, _, rejected := b.conn.log.stats()
	assert.Equal(t, uint64(1), rejected)
}

func TestSendDoesNotBlockWhenFull(t *testing.T) {
	c := newConnection(slog.Default(), connOptions{bufferBytes: 10})
	done := make(chan error, 1)
	go func() {
		done <- c.send(func(uint64) ([]byte, error) { return []byte(`{"type":"test","payload":{}}`), nil })
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, errBufferFull)
	case <-time.After(time.Second):
		t.Fatal("send blocked when the buffer was full")
	}
}

func TestClosedConnectionRejectsSend(t *testing.T) {
	c := newConnection(slog.Default(), connOptions{})
	require.NoError(t, c.close())
	for i := 0; i < 20; i++ {
		require.Error(t, c.send(func(uint64) ([]byte, error) { return []byte("data"), nil }))
	}
}

// --- readLoop edge cases ---

func TestReadLoopNonJsonMessage(t *testing.T) {
	// Server that sends a non-JSON message before a valid ack.
	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			// Send garbage first (covers json.Unmarshal error in readLoop).
			_ = c.WriteMessage(ws.TextMessage, []byte("not json"))

			var env streaming.Envelope
			if json.Unmarshal(msg, &env) == nil && env.Type == streaming.TypeStartMission {
				ack, _ := json.Marshal(streaming.AckMessage{Type: "ack", For: env.Type})
				_ = c.WriteMessage(ws.TextMessage, ack)
			}
		}
	}))
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	// StartMission will get garbage first, then a valid ack.
	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
}

func TestReadLoopNonAckMessage(t *testing.T) {
	// Server that sends a valid JSON message that is not an ack.
	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			// Send a non-ack JSON message (valid JSON, but type != "ack").
			_ = c.WriteMessage(ws.TextMessage, []byte(`{"type":"info","data":"hello"}`))

			var env streaming.Envelope
			if json.Unmarshal(msg, &env) == nil && env.Type == streaming.TypeStartMission {
				ack, _ := json.Marshal(streaming.AckMessage{Type: "ack", For: env.Type})
				_ = c.WriteMessage(ws.TextMessage, ack)
			}
		}
	}))
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
}

// --- reconnect policy ---

func TestReconnectStopsOnRejectedCredentials(t *testing.T) {
	var dials atomic.Int32
	upgrader := ws.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dials.Add(1) > 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = c.Close() // drop the first connection at once
	}))
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s", ReconnectMin: 10 * time.Millisecond, ReconnectMax: 20 * time.Millisecond})
	require.NoError(t, b.Init())
	defer b.Close()

	require.Eventually(t, func() bool {
		b.conn.mu.Lock()
		defer b.conn.mu.Unlock()
		return b.conn.fatal != nil
	}, 2*time.Second, 5*time.Millisecond)
	n := dials.Load()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, n, dials.Load(), "no dials after the server rejected the credentials")
}

func TestReconnectKeepsTryingPastOldLimit(t *testing.T) {
	var dials atomic.Int32
	upgrader := ws.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := dials.Add(1)
		if n > 1 && n < 15 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		if n == 1 {
			return // drop the first connection
		}
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s", ReconnectMin: time.Millisecond, ReconnectMax: 2 * time.Millisecond})
	require.NoError(t, b.Init())
	defer b.Close()

	// The previous implementation gave up after 10 attempts.
	require.Eventually(t, func() bool {
		b.conn.mu.Lock()
		defer b.conn.mu.Unlock()
		return b.conn.sock != nil && dials.Load() >= 15
	}, 5*time.Second, 5*time.Millisecond)
}

func TestJitterStaysWithinBounds(t *testing.T) {
	for range 1000 {
		d := jitter(time.Second)
		require.GreaterOrEqual(t, d, 800*time.Millisecond)
		require.LessOrEqual(t, d, 1200*time.Millisecond)
	}
}

func TestConnOptionsDefaults(t *testing.T) {
	o := connOptions{}.withDefaults()
	assert.Equal(t, defaultBufferBytes, o.bufferBytes)
	assert.Equal(t, defaultAckStall, o.ackStall)
	assert.Equal(t, defaultReconnectMin, o.reconnectMin)
	assert.Equal(t, defaultReconnectMax, o.reconnectMax)

	o = connOptions{reconnectMin: time.Minute}.withDefaults()
	assert.Equal(t, time.Minute, o.reconnectMax, "ceiling never falls below the first delay")
}

// --- restartable server for integration reconnect test ---

type restartableServer struct {
	mu       sync.Mutex
	srv      *http.Server
	ml       *messageLog
	upgrader ws.Upgrader
	t        *testing.T
}

func newRestartableServer(t *testing.T) *restartableServer {
	t.Helper()
	return &restartableServer{
		ml:       &messageLog{},
		upgrader: ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		t:        t,
	}
}

func (rs *restartableServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := rs.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			var env streaming.Envelope
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			rs.ml.add(env)

			if env.Type == streaming.TypeStartMission || env.Type == streaming.TypeEndMission {
				ack := streaming.AckMessage{Type: "ack", For: env.Type}
				data, _ := json.Marshal(ack)
				if err := c.WriteMessage(ws.TextMessage, data); err != nil {
					return
				}
			}
		}
	})
}

func (rs *restartableServer) start() string {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(rs.t, err)

	rs.srv = &http.Server{Handler: rs.handler()}
	go rs.srv.Serve(ln)
	return "ws://" + ln.Addr().String()
}

func (rs *restartableServer) startOnAddr(addr string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	require.NoError(rs.t, err)

	rs.srv = &http.Server{Handler: rs.handler()}
	go rs.srv.Serve(ln)
}

func (rs *restartableServer) stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.srv != nil {
		rs.srv.Close()
		rs.srv = nil
	}
}

func TestReconnectAfterServerRestart(t *testing.T) {
	rs := newRestartableServer(t)
	url := rs.start()
	addr := strings.TrimPrefix(url, "ws://")

	b := New(Config{URL: url, Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "R"}, &core.World{WorldName: "W"}))

	rs.stop()
	time.Sleep(100 * time.Millisecond)

	rs.startOnAddr(addr)
	defer rs.stop()

	// Wait for reconnect (backoff starts at 1s).
	time.Sleep(2 * time.Second)

	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 5, UnitName: "Bravo"}))
	time.Sleep(200 * time.Millisecond)

	msgs := rs.ml.all()
	types := make(map[string]int)
	for _, m := range msgs {
		types[m.Type]++
	}

	assert.GreaterOrEqual(t, types[streaming.TypeStartMission], 1, "start_mission should be replayed on reconnect")
	assert.GreaterOrEqual(t, types[streaming.TypeAddSoldier], 1, "message after reconnect should arrive")
}
