package websocket

import (
	"encoding/json"
	"log/slog"
	"net"
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

// newIdleConn creates a connection wired to the given server URL but does NOT
// start writeLoop/readLoop. Useful for testing connection internals directly.
func newIdleConn(t *testing.T, serverURL string) *connection {
	t.Helper()
	c := newConnection(slog.Default())
	c.wsURL = serverURL
	c.secret = "s"

	conn, err := c.dialOnce()
	require.NoError(t, err)

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	return c
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

	require.NoError(t, b.EndMission())

	msgs := ml.all()
	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, streaming.TypeStartMission, msgs[0].Type)
	assert.Equal(t, streaming.TypeEndMission, msgs[len(msgs)-1].Type)
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

	// Events
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{SoldierID: 1, Weapon: "arifle_MX_F"}))
	require.NoError(t, b.RecordProjectileEvent(&core.ProjectileEvent{FirerObjectID: 1}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"}))
	require.NoError(t, b.RecordHitEvent(&core.HitEvent{EventText: "hit"}))
	require.NoError(t, b.RecordKillEvent(&core.KillEvent{EventText: "killed"}))
	require.NoError(t, b.RecordChatEvent(&core.ChatEvent{Message: "hello"}))
	require.NoError(t, b.RecordRadioEvent(&core.RadioEvent{Radio: "ACRE"}))
	require.NoError(t, b.RecordTelemetryEvent(&core.TelemetryEvent{CaptureFrame: 1, FpsAverage: 45}))
	require.NoError(t, b.RecordTimeState(&core.TimeState{MissionTime: 120}))
	require.NoError(t, b.RecordAce3DeathEvent(&core.Ace3DeathEvent{SoldierID: 1, Reason: "bleeding"}))
	require.NoError(t, b.RecordAce3UnconsciousEvent(&core.Ace3UnconsciousEvent{SoldierID: 1, IsUnconscious: true}))

	require.NoError(t, b.EndMission())

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
	assert.Equal(t, uint(42), dp.EndFrame)
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
	require.NoError(t, b.EndMission())

	// After EndMission, marker IDs should reset.
	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
	m2 := &core.Marker{MarkerName: "b"}
	id2, err := b.AddMarker(m2)
	require.NoError(t, err)
	assert.Equal(t, uint(1), id2)
	require.NoError(t, b.EndMission())
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
	c := newConnection(slog.Default())
	require.NoError(t, c.close()) // conn is nil, should still succeed
}

// --- sendAndWait edge cases ---

func TestAckTimeout(t *testing.T) {
	// Server that never sends acks.
	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	data, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{})
	require.NoError(t, err)

	err = b.conn.sendAndWait(data, streaming.TypeStartMission, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for ack")
}

func TestSendAndWaitClosedConnection(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())

	require.NoError(t, b.Close())

	data, err := marshalEnvelope(streaming.TypeEndMission, nil)
	require.NoError(t, err)

	err = b.conn.sendAndWait(data, streaming.TypeEndMission, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

// --- marshalEnvelope error ---

func TestMarshalEnvelopeError(t *testing.T) {
	// Channels cannot be JSON-marshaled.
	_, err := marshalEnvelope("test", make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal test payload")
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

	err := b.sendEnvelopeAndWait("test", make(chan int))
	require.Error(t, err)
}

func TestStartMissionMarshalError(t *testing.T) {
	// StartMission has its own marshal path; use a backend with a nil
	// connection to avoid network issues — the error should happen
	// before any send. Actually StartMissionPayload uses concrete types
	// that always marshal, so we test the full happy path elsewhere.
	// This test verifies StartMission caches the serialized message.
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "X"}, &core.World{WorldName: "Y"}))

	b.conn.mu.Lock()
	cached := b.conn.cachedStartMsg
	b.conn.mu.Unlock()
	assert.NotNil(t, cached, "StartMission should cache the serialized message")

	require.NoError(t, b.EndMission())

	b.conn.mu.Lock()
	cached = b.conn.cachedStartMsg
	b.conn.mu.Unlock()
	assert.Nil(t, cached, "EndMission should clear the cached message")
}

// --- send channel full ---

func TestSendChannelFullDropsMessage(t *testing.T) {
	// Create a connection without starting writeLoop so the channel
	// stays full and isn't drained.
	c := newConnection(slog.Default())

	for i := range sendChSize {
		c.sendCh <- []byte("{}")
		_ = i
	}

	done := make(chan struct{})
	go func() {
		c.send([]byte(`{"type":"test"}`))
		close(done)
	}()

	select {
	case <-done:
		// Good — send returned without blocking.
	case <-time.After(time.Second):
		t.Fatal("send blocked when channel was full")
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
	data, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: &core.Mission{}, World: &core.World{},
	})
	require.NoError(t, err)
	require.NoError(t, b.conn.sendAndWait(data, streaming.TypeStartMission, 2*time.Second))
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

	data, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: &core.Mission{}, World: &core.World{},
	})
	require.NoError(t, err)
	require.NoError(t, b.conn.sendAndWait(data, streaming.TypeStartMission, 2*time.Second))
}

// --- reconnect tests (called directly for reliable coverage) ---

func TestReconnectDirectSuccess(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	c := newIdleConn(t, wsURL(srv))
	defer c.close()

	// Close the underlying WebSocket so reconnect has work to do.
	c.mu.Lock()
	_ = c.conn.Close()
	c.conn = nil
	c.mu.Unlock()

	// Call reconnect synchronously — server is up so it should succeed
	// on the first attempt (after 1s backoff).
	c.reconnect()

	// Verify the connection is restored.
	c.mu.Lock()
	assert.NotNil(t, c.conn, "connection should be restored after reconnect")
	c.mu.Unlock()

	// Send a message to verify the connection works.
	data, err := marshalEnvelope(streaming.TypeAddSoldier, &core.Soldier{ID: 1})
	require.NoError(t, err)
	c.send(data)
	time.Sleep(200 * time.Millisecond)

	msgs := ml.all()
	assert.GreaterOrEqual(t, len(msgs), 1)
}

func TestReconnectDirectWithCachedMessage(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()

	c := newIdleConn(t, wsURL(srv))
	defer c.close()

	// Set a cached start_mission message for replay.
	startData, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: &core.Mission{MissionName: "Reconnect"},
		World:   &core.World{WorldName: "Altis"},
	})
	require.NoError(t, err)

	c.mu.Lock()
	c.cachedStartMsg = startData
	_ = c.conn.Close()
	c.conn = nil
	c.mu.Unlock()

	c.reconnect()

	// Wait for the replayed message to arrive.
	time.Sleep(200 * time.Millisecond)

	msgs := ml.all()
	require.GreaterOrEqual(t, len(msgs), 1)
	assert.Equal(t, streaming.TypeStartMission, msgs[0].Type, "start_mission should be replayed")
}

func TestReconnectWhenClosed(t *testing.T) {
	c := newConnection(slog.Default())
	c.closed = true

	// Should return immediately without panicking.
	c.reconnect()

	assert.False(t, c.reconnecting.Load(), "reconnecting flag should be cleared")
}

func TestReconnectCancelledByDone(t *testing.T) {
	c := newConnection(slog.Default())
	c.wsURL = "ws://127.0.0.1:1" // unreachable
	c.secret = "s"
	close(c.done) // signal shutdown

	// Should return on the first loop iteration via <-c.done.
	c.reconnect()

	assert.False(t, c.reconnecting.Load())
}

func TestReconnectDialFailure(t *testing.T) {
	c := newConnection(slog.Default())
	c.wsURL = "ws://127.0.0.1:1" // unreachable
	c.secret = "s"

	// Close done after a short delay so reconnect doesn't loop forever.
	go func() {
		time.Sleep(1500 * time.Millisecond)
		close(c.done)
	}()

	c.reconnect()

	// Connection should still be nil since all dials failed.
	c.mu.Lock()
	assert.Nil(t, c.conn)
	c.mu.Unlock()
}

func TestReconnectGuardPreventsDouble(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	b.conn.reconnecting.Store(true)
	b.conn.reconnect() // should be a no-op
	b.conn.reconnecting.Store(false)
}

// --- writeLoop error paths ---

func TestWriteLoopReconnectsOnError(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	c := newIdleConn(t, wsURL(srv))
	defer c.close()

	// Close the underlying WebSocket connection so the next write fails.
	c.mu.Lock()
	oldConn := c.conn
	c.mu.Unlock()
	_ = oldConn.Close()

	// Start writeLoop and push a message — it should fail and trigger reconnect.
	go c.writeLoop()
	c.sendCh <- []byte(`{"type":"test"}`)

	// Wait for reconnect to complete (1s backoff + margin).
	time.Sleep(1800 * time.Millisecond)

	c.mu.Lock()
	restored := c.conn != nil
	c.mu.Unlock()

	assert.True(t, restored, "writeLoop error should trigger reconnect")
}

func TestWriteLoopExitsOnDone(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	c := newIdleConn(t, wsURL(srv))

	done := make(chan struct{})
	go func() {
		c.writeLoop()
		close(done)
	}()

	// Signal shutdown.
	c.close()

	select {
	case <-done:
		// writeLoop exited.
	case <-time.After(time.Second):
		t.Fatal("writeLoop did not exit on done")
	}
}

// --- readLoop error paths ---

func TestReadLoopReconnectsOnError(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	c := newIdleConn(t, wsURL(srv))
	defer c.close()

	// Close the underlying connection so readLoop gets an error.
	c.mu.Lock()
	oldConn := c.conn
	c.mu.Unlock()
	_ = oldConn.Close()

	readDone := make(chan struct{})
	go func() {
		c.readLoop()
		close(readDone)
	}()

	// readLoop should exit quickly after the error.
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("readLoop did not exit on connection error")
	}

	// Wait for reconnect to complete (1s backoff + margin).
	time.Sleep(1800 * time.Millisecond)

	c.mu.Lock()
	restored := c.conn != nil
	c.mu.Unlock()

	assert.True(t, restored, "readLoop error should trigger reconnect")
}

func TestReadLoopExitsWhenConnNil(t *testing.T) {
	c := newConnection(slog.Default())
	// conn is nil → readLoop should return immediately.

	done := make(chan struct{})
	go func() {
		c.readLoop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readLoop did not exit when conn is nil")
	}
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
