package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

// resumeServer is an in-process server speaking the resumable protocol. It
// keeps missions across connections, like the web server's session registry.
type resumeServer struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	missions map[string]*fakeMission
	conns    map[*ws.Conn]string // live connection → mission id ("" before start/resume)

	discard   atomic.Bool // drop envelopes as if lost in transit; the link stays dead
	noAck     atomic.Bool // never send cumulative acks
	dropEnd   atomic.Bool // save end_mission, then close without acknowledging it
	legacy    atomic.Bool // behave like a web server without resume support
	resumes   atomic.Int32
	starts    atomic.Int32
	conns2ndP atomic.Int32 // connections that began with resume
}

type fakeMission struct {
	producer  string
	lastSeq   uint64
	received  []streaming.Envelope // accepted envelopes in arrival order
	dups      int
	gaps      int
	completed bool
}

func newResumeServer(t *testing.T) *resumeServer {
	t.Helper()
	rs := &resumeServer{t: t, missions: map[string]*fakeMission{}, conns: map[*ws.Conn]string{}}
	upgrader := ws.Upgrader{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		rs.serve(c)
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

func (rs *resumeServer) url() string { return wsURL(rs.srv) }

func (rs *resumeServer) serve(c *ws.Conn) {
	rs.mu.Lock()
	rs.conns[c] = ""
	rs.mu.Unlock()
	defer func() {
		rs.mu.Lock()
		delete(rs.conns, c)
		rs.mu.Unlock()
		_ = c.Close()
	}()

	var m *fakeMission
	dead := false // a link that started losing data never delivers again, as with TCP
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			return
		}
		var env streaming.Envelope
		if json.Unmarshal(raw, &env) != nil {
			continue
		}
		if env.Type != streaming.TypeResume && env.Type != streaming.TypeStartMission {
			if rs.discard.Load() {
				dead = true
			}
			if dead {
				continue
			}
		}

		switch env.Type {
		case streaming.TypeStartMission:
			var p streaming.StartMissionPayload
			require.NoError(rs.t, json.Unmarshal(env.Payload, &p))
			rs.starts.Add(1)
			rs.mu.Lock()
			m = rs.missions[p.MissionID]
			if m == nil {
				m = &fakeMission{producer: p.ProducerID, lastSeq: env.Seq}
				rs.missions[p.MissionID] = m
			} else if env.Seq > m.lastSeq {
				m.lastSeq = env.Seq
			}
			m.received = append(m.received, env)
			rs.conns[c] = p.MissionID
			rs.mu.Unlock()
			ack := streaming.AckMessage{Type: streaming.ServerAck, For: env.Type, Seq: env.Seq, ProtocolVersion: streaming.ProtocolVersionResume}
			if rs.legacy.Load() {
				ack = streaming.AckMessage{Type: streaming.ServerAck, For: env.Type}
			}
			if c.WriteJSON(ack) != nil {
				return
			}

		case streaming.TypeResume:
			var p streaming.ResumePayload
			require.NoError(rs.t, json.Unmarshal(env.Payload, &p))
			rs.resumes.Add(1)
			if rs.legacy.Load() {
				continue // an older server ignores resume
			}
			rs.conns2ndP.Add(1)
			rs.mu.Lock()
			m = rs.missions[p.MissionID]
			reply := streaming.AckMessage{Type: streaming.ServerResumeUnknown}
			if m != nil && m.producer == p.ProducerID {
				reply = streaming.AckMessage{Type: streaming.ServerResumeOK, Seq: m.lastSeq}
				if m.completed {
					reply = streaming.AckMessage{Type: streaming.ServerResumeCompleted}
				}
				rs.conns[c] = p.MissionID
			} else {
				m = nil
			}
			rs.mu.Unlock()
			if c.WriteJSON(reply) != nil {
				return
			}

		default:
			if m == nil {
				continue
			}
			rs.mu.Lock()
			switch {
			case env.Seq <= m.lastSeq:
				m.dups++
				rs.mu.Unlock()
				continue
			case env.Seq > m.lastSeq+1:
				m.gaps++
			}
			m.lastSeq = env.Seq
			m.received = append(m.received, env)
			last := m.lastSeq
			if env.Type == streaming.TypeEndMission {
				m.completed = true
			}
			rs.mu.Unlock()

			if env.Type == streaming.TypeEndMission {
				if rs.dropEnd.Load() {
					return // saved, but the acknowledgement never arrives
				}
				_ = c.WriteJSON(streaming.AckMessage{Type: streaming.ServerAck, For: env.Type, Seq: last})
				return
			}
			if !rs.noAck.Load() {
				if c.WriteJSON(streaming.AckMessage{Type: streaming.ServerAck, Seq: last}) != nil {
					return
				}
			}
		}
	}
}

// kill drops every live connection, as a network failure would.
func (rs *resumeServer) kill() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for c := range rs.conns {
		_ = c.Close()
	}
}

// killMission drops the connections serving one mission.
func (rs *resumeServer) killMission(id string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for c, mid := range rs.conns {
		if mid == id {
			_ = c.Close()
		}
	}
}

// forget drops all server-side mission state, as a web server restart would.
func (rs *resumeServer) forget() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.missions = map[string]*fakeMission{}
}

func (rs *resumeServer) mission(id string) fakeMission {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	m := rs.missions[id]
	if m == nil {
		return fakeMission{}
	}
	cp := *m
	cp.received = append([]streaming.Envelope(nil), m.received...)
	return cp
}

func (rs *resumeServer) lastSeq(id string) uint64 { return rs.mission(id).lastSeq }

// fastConfig keeps recovery quick for tests.
func fastConfig(url string) Config {
	return Config{
		URL: url, Secret: "s", SaveTimeout: 5 * time.Second,
		ReconnectMin: 10 * time.Millisecond, ReconnectMax: 50 * time.Millisecond,
		AckStallTimeout: 300 * time.Millisecond,
	}
}

func startResumable(t *testing.T, b *Backend) string {
	t.Helper()
	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Resume", CaptureDelay: 1}, &core.World{WorldName: "altis"}))
	require.True(t, b.conn.isResumable(), "server acknowledged resume support")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.missionID
}

func recordStates(t *testing.T, b *Backend, from, to int) {
	t.Helper()
	for i := from; i <= to; i++ {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(i)}))
	}
}

// requireContiguous checks the server holds envelopes 1..last exactly once,
// in order, with soldier states carrying increasing frames.
func requireContiguous(t *testing.T, m fakeMission, last uint64) {
	t.Helper()
	require.Equal(t, last, m.lastSeq)
	require.Len(t, m.received, int(last))
	var frame core.Frame
	for i, env := range m.received {
		require.Equal(t, uint64(i+1), env.Seq, "envelope %d", i)
		if env.Type == streaming.TypeSoldierState {
			var s core.SoldierState
			require.NoError(t, json.Unmarshal(env.Payload, &s))
			require.Greater(t, s.CaptureFrame, frame, "frames stay in order")
			frame = s.CaptureFrame
		}
	}
	require.Zero(t, m.gaps)
}

func TestResumableServerDetected(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 10)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 11 }, 2*time.Second, 5*time.Millisecond)
	// Acknowledged envelopes are released.
	require.Eventually(t, func() bool { n, _, _ := b.conn.log.stats(); return n == 0 }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), 12)
}

func TestResumeAfterDropDeliversEverythingOnce(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 50)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 51 }, 2*time.Second, 5*time.Millisecond)

	rs.kill()
	recordStates(t, b, 51, 100) // recorded while the connection is down
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 101 }, 5*time.Second, 5*time.Millisecond)

	require.NoError(t, b.EndMission(""))
	m := rs.mission(id)
	requireContiguous(t, m, 102)
	assert.GreaterOrEqual(t, rs.resumes.Load(), int32(1))
	assert.Equal(t, int32(1), rs.starts.Load(), "a resumed mission is not started again")
}

func TestResumeResendsEnvelopesLostInTransit(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 20)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 21 }, 2*time.Second, 5*time.Millisecond)

	// Envelopes 22..41 are written but never reach the mission.
	rs.discard.Store(true)
	recordStates(t, b, 21, 40)
	require.Eventually(t, func() bool { return b.conn.sent.Load() == 41 }, 2*time.Second, 5*time.Millisecond)
	n, _, _ := b.conn.log.stats()
	require.Equal(t, 20, n, "unacknowledged envelopes stay in the log")

	rs.discard.Store(false)
	rs.kill()
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 41 }, 5*time.Second, 5*time.Millisecond)
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), 42)
}

func TestAckStallTriggersReconnect(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)

	// Half-open link: writes succeed but nothing arrives or is acknowledged.
	rs.discard.Store(true)
	recordStates(t, b, 1, 10)
	time.Sleep(100 * time.Millisecond)
	rs.discard.Store(false)

	// No server-side close: only the stall watchdog can recover.
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 11 }, 5*time.Second, 10*time.Millisecond)
	assert.GreaterOrEqual(t, rs.resumes.Load(), int32(1))
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), 12)
}

func TestIdleSocketIsNotTreatedAsStalled(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	startResumable(t, b)
	time.Sleep(4 * fastConfig("").AckStallTimeout) // nothing written, nothing pending
	assert.Zero(t, rs.resumes.Load())
	recordStates(t, b, 1, 5)
	time.Sleep(4 * fastConfig("").AckStallTimeout) // written and acknowledged
	assert.Zero(t, rs.resumes.Load())
	require.NoError(t, b.EndMission(""))
}

func TestResumeUnknownStartsNewSegment(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 10)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 11 }, 2*time.Second, 5*time.Millisecond)

	// The server restarts and loses the mission; envelopes 12..21 are lost too.
	rs.discard.Store(true)
	recordStates(t, b, 11, 20)
	require.Eventually(t, func() bool { return b.conn.sent.Load() == 21 }, 2*time.Second, 5*time.Millisecond)
	rs.forget()
	rs.discard.Store(false)
	rs.kill()

	recordStates(t, b, 21, 25)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 26 }, 5*time.Second, 5*time.Millisecond)
	require.NoError(t, b.EndMission(""))

	m := rs.mission(id)
	require.NotEmpty(t, m.received)
	assert.Equal(t, streaming.TypeStartMission, m.received[0].Type, "the new segment opens with start_mission")
	assert.Equal(t, uint64(1), m.received[0].Seq)
	// Unacknowledged envelopes follow: nothing the server never confirmed is lost.
	assert.Equal(t, uint64(12), m.received[1].Seq)
	assert.Equal(t, 1, m.gaps, "envelopes 2..11 were acknowledged by the lost session")
	assert.Equal(t, int32(2), rs.starts.Load())
}

func TestLostEndAckResolvedByResume(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	rs.dropEnd.Store(true)

	done := make(chan error, 1)
	go func() { done <- b.EndMission("") }()
	select {
	case err := <-done:
		require.NoError(t, err, "resume_completed stands in for the lost end_mission ack")
	case <-time.After(5 * time.Second):
		t.Fatal("EndMission did not complete")
	}
	m := rs.mission(id)
	assert.True(t, m.completed)
	assert.Equal(t, int32(1), rs.starts.Load(), "the saved mission is not started again")
}

func TestEndMissionDeliveredAcrossReconnect(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	rs.discard.Store(true) // end_mission is written but lost
	done := make(chan error, 1)
	go func() { done <- b.EndMission("Final") }()
	require.Eventually(t, func() bool { return b.conn.sent.Load() == 2 }, 2*time.Second, 5*time.Millisecond)
	rs.discard.Store(false)
	rs.kill()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("EndMission did not complete after reconnect")
	}
	m := rs.mission(id)
	require.True(t, m.completed)
	var p streaming.EndMissionPayload
	require.NoError(t, json.Unmarshal(m.received[len(m.received)-1].Payload, &p))
	assert.Equal(t, "Final", p.Tag)
}

func TestTwoProducersRecoverIndependently(t *testing.T) {
	rs := newResumeServer(t)
	a := New(fastConfig(rs.url()))
	b := New(fastConfig(rs.url()))
	require.NoError(t, a.Init())
	require.NoError(t, b.Init())
	defer a.Close()
	defer b.Close()
	require.NotEqual(t, a.producerID, b.producerID)

	idA := startResumable(t, a)
	idB := startResumable(t, b)
	require.NotEqual(t, idA, idB)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); recordStates(t, a, 1, 200) }()
	go func() { defer wg.Done(); recordStates(t, b, 1, 200) }()
	time.Sleep(20 * time.Millisecond)
	rs.killMission(idA) // only A's network fails
	wg.Wait()

	require.Eventually(t, func() bool { return rs.lastSeq(idA) == 201 && rs.lastSeq(idB) == 201 }, 5*time.Second, 5*time.Millisecond)
	require.NoError(t, a.EndMission(""))
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(idA), 202)
	requireContiguous(t, rs.mission(idB), 202)
	assert.Zero(t, rs.mission(idB).dups, "B was never disturbed")
}

func TestLegacyServerReleasesWrittenEnvelopes(t *testing.T) {
	srv, ml := testServer(t) // acknowledges lifecycle messages only, without protocolVersion
	defer srv.Close()

	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Legacy"}, &core.World{WorldName: "altis"}))
	require.False(t, b.conn.isResumable())
	recordStates(t, b, 1, 100)
	require.Eventually(t, func() bool { n, _, _ := b.conn.log.stats(); return n == 0 }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, b.EndMission(""))

	for _, m := range ml.all() {
		assert.NotEqual(t, streaming.TypeResume, m.Type, "a legacy server never receives resume")
	}
}
