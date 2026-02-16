package websocket

import (
	"encoding/json"
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
	require.NoError(t, b.AddMarker(&core.Marker{MarkerName: "m1"}))

	// State updates
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 1}))
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 100, CaptureFrame: 1}))
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: 1, CaptureFrame: 1}))
	require.NoError(t, b.DeleteMarker("m1", 10))

	// Events
	require.NoError(t, b.RecordFiredEvent(&core.FiredEvent{SoldierID: 1, Weapon: "arifle_MX_F"}))
	require.NoError(t, b.RecordProjectileEvent(&core.ProjectileEvent{FirerObjectID: 1}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{Name: "test"}))
	require.NoError(t, b.RecordHitEvent(&core.HitEvent{EventText: "hit"}))
	require.NoError(t, b.RecordKillEvent(&core.KillEvent{EventText: "killed"}))
	require.NoError(t, b.RecordChatEvent(&core.ChatEvent{Message: "hello"}))
	require.NoError(t, b.RecordRadioEvent(&core.RadioEvent{Radio: "ACRE"}))
	require.NoError(t, b.RecordServerFpsEvent(&core.ServerFpsEvent{FpsAverage: 50}))
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
	for _, typ := range []string{
		TypeStartMission, TypeEndMission,
		TypeAddSoldier, TypeAddVehicle, TypeAddMarker,
		TypeSoldierState, TypeVehicleState, TypeMarkerState, TypeDeleteMarker,
		TypeFiredEvent, TypeProjectileEvent, TypeGeneralEvent,
		TypeHitEvent, TypeKillEvent, TypeChatEvent, TypeRadioEvent,
		TypeServerFps, TypeTimeState, TypeAce3Death, TypeAce3Unconscious,
	} {
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

func TestMarkerIDResetsAfterEndMission(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
	m1 := &core.Marker{MarkerName: "a"}
	require.NoError(t, b.AddMarker(m1))
	assert.Equal(t, uint(1), m1.ID)
	require.NoError(t, b.EndMission())

	// After EndMission, marker IDs should reset.
	require.NoError(t, b.StartMission(&core.Mission{}, &core.World{}))
	m2 := &core.Marker{MarkerName: "b"}
	require.NoError(t, b.AddMarker(m2))
	assert.Equal(t, uint(1), m2.ID)
	require.NoError(t, b.EndMission())
}

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

func TestAckTimeout(t *testing.T) {
	// Server that never sends acks.
	upgrader := ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Read messages but never reply.
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

	// Test sendAndWait directly with a short timeout instead of going
	// through StartMission (which uses the 10s ackTimeout constant).
	data, err := marshalEnvelope(TypeStartMission, StartMissionPayload{})
	require.NoError(t, err)

	err = b.conn.sendAndWait(data, TypeStartMission, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for ack")
}

func TestSendAndWaitClosedConnection(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())

	// Close the connection, then sendAndWait should return an error.
	require.NoError(t, b.Close())

	data, err := marshalEnvelope(TypeEndMission, nil)
	require.NoError(t, err)

	err = b.conn.sendAndWait(data, TypeEndMission, 100*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection closed")
}

// restartableServer is a test WebSocket server that can be stopped and
// restarted on the same listener to test reconnection.
type restartableServer struct {
	mu       sync.Mutex
	listener *restartableListener
	srv      *http.Server
	ml       *messageLog
	upgrader ws.Upgrader
	t        *testing.T
}

type restartableListener struct {
	net    string
	addr   string
	inner  net.Listener
	closed bool
	mu     sync.Mutex
}

func (l *restartableListener) Accept() (net.Conn, error) { return l.inner.Accept() }
func (l *restartableListener) Addr() net.Addr             { return l.inner.Addr() }
func (l *restartableListener) Close() error {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return l.inner.Close()
}

func newRestartableServer(t *testing.T) *restartableServer {
	t.Helper()
	ml := &messageLog{}
	rs := &restartableServer{
		ml:       ml,
		upgrader: ws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		t:        t,
	}
	return rs
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
			var env Envelope
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			rs.ml.add(env)

			if env.Type == TypeStartMission || env.Type == TypeEndMission {
				ack := AckMessage{Type: "ack", For: env.Type}
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

	rs.listener = &restartableListener{inner: ln}
	rs.srv = &http.Server{Handler: rs.handler()}
	go rs.srv.Serve(rs.listener)
	return "ws://" + ln.Addr().String()
}

func (rs *restartableServer) startOnAddr(addr string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	require.NoError(rs.t, err)

	rs.listener = &restartableListener{inner: ln}
	rs.srv = &http.Server{Handler: rs.handler()}
	go rs.srv.Serve(rs.listener)
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

	// Extract the host:port so we can restart on the same address.
	addr := strings.TrimPrefix(url, "ws://")

	b := New(Config{URL: url, Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "R"}, &core.World{WorldName: "W"}))

	// Stop the server — this will cause the read/write loops to fail.
	rs.stop()
	time.Sleep(100 * time.Millisecond)

	// Restart on the same address so the reconnect dials succeed.
	rs.startOnAddr(addr)
	defer rs.stop()

	// Wait for reconnect (backoff starts at 1s).
	time.Sleep(2 * time.Second)

	// Send a message — if reconnect worked, this should arrive.
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 5, UnitName: "Bravo"}))
	time.Sleep(200 * time.Millisecond)

	msgs := rs.ml.all()
	types := make(map[string]int)
	for _, m := range msgs {
		types[m.Type]++
	}

	// start_mission should have been replayed on reconnect.
	assert.GreaterOrEqual(t, types[TypeStartMission], 1, "start_mission should be replayed on reconnect")
	assert.GreaterOrEqual(t, types[TypeAddSoldier], 1, "message after reconnect should arrive")
}

func TestReconnectGuardPreventsDouble(t *testing.T) {
	// Verify the atomic guard prevents concurrent reconnect calls.
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	// Simulate: set reconnecting flag, then call reconnect — should return immediately.
	b.conn.reconnecting.Store(true)
	b.conn.reconnect() // should be a no-op
	b.conn.reconnecting.Store(false)
}

func TestSendChannelFullDropsMessage(t *testing.T) {
	srv, _ := testServer(t)
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	// Fill the send channel to capacity.
	for i := 0; i < sendChSize; i++ {
		b.conn.sendCh <- []byte("{}")
	}

	// Next send should drop without blocking.
	done := make(chan struct{})
	go func() {
		b.conn.send([]byte(`{"type":"test"}`))
		close(done)
	}()

	select {
	case <-done:
		// Good — send returned without blocking.
	case <-time.After(time.Second):
		t.Fatal("send blocked when channel was full")
	}
}
