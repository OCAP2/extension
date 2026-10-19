package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestReconnectReplaysStartBeforeQueuedStates(t *testing.T) {
	type observed struct {
		generation int32
		env        streaming.Envelope
	}
	received := make(chan observed, 256)
	sockets := make(chan *ws.Conn, 4)
	var generation atomic.Int32
	upgrader := ws.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		g := generation.Add(1)
		sockets <- conn
		for {
			var env streaming.Envelope
			if conn.ReadJSON(&env) != nil {
				return
			}
			received <- observed{g, env}
			if env.Type == streaming.TypeStartMission {
				conn.WriteJSON(streaming.AckMessage{Type: "ack", For: env.Type})
			}
		}
	}))
	defer srv.Close()
	b := New(Config{URL: "ws" + strings.TrimPrefix(srv.URL, "http")})
	require.NoError(t, b.Init())
	defer b.Close()
	require.NoError(t, b.StartMission(&core.Mission{MissionName: "Reconnect", CaptureDelay: 1}, &core.World{WorldName: "altis"}))
	first := <-received
	require.Equal(t, int32(1), first.generation)
	old := <-sockets
	require.NoError(t, old.Close())
	require.Eventually(t, func() bool { b.conn.mu.Lock(); defer b.conn.mu.Unlock(); return b.conn.sock == nil }, time.Second, time.Millisecond)
	for i := 1; i <= 100; i++ {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(i)}))
	}
	read := func() observed {
		select {
		case m := <-received:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("reconnect did not deliver queued states")
			return observed{}
		}
	}
	start := read()
	require.Equal(t, int32(2), start.generation)
	require.Equal(t, streaming.TypeStartMission, start.env.Type)
	for i := 1; i <= 100; i++ {
		m := read()
		require.Equal(t, int32(2), m.generation)
		require.Equal(t, streaming.TypeSoldierState, m.env.Type)
		var state core.SoldierState
		require.NoError(t, json.Unmarshal(m.env.Payload, &state))
		require.Equal(t, core.Frame(i), state.CaptureFrame)
	}
}

func TestStartDuringReconnectIsDeliveredOnce(t *testing.T) {
	for _, phase := range []string{"backoff", "dial"} {
		t.Run(phase, func(t *testing.T) {
			received := make(chan streaming.Envelope, 16)
			initial := make(chan *ws.Conn, 1)
			replacementDial := make(chan struct{})
			releaseReplacement := make(chan struct{})
			var generation atomic.Int32
			upgrader := ws.Upgrader{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				g := generation.Add(1)
				if g == 2 {
					close(replacementDial)
					<-releaseReplacement
				}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				if g == 1 {
					initial <- conn
				}
				for {
					var env streaming.Envelope
					if conn.ReadJSON(&env) != nil {
						return
					}
					received <- env
					if env.Type == streaming.TypeStartMission {
						_ = conn.WriteJSON(streaming.AckMessage{Type: "ack", For: env.Type})
					}
				}
			}))
			defer srv.Close()
			b := New(Config{URL: "ws" + strings.TrimPrefix(srv.URL, "http")})
			require.NoError(t, b.Init())
			defer b.Close()
			// Always release a blocked upgrade, including when an assertion fails.
			defer func() {
				select {
				case <-releaseReplacement:
				default:
					close(releaseReplacement)
				}
			}()
			require.NoError(t, (<-initial).Close())
			require.Eventually(t, func() bool {
				b.conn.mu.Lock()
				defer b.conn.mu.Unlock()
				return b.conn.sock == nil
			}, time.Second, time.Millisecond)
			if phase == "dial" {
				select {
				case <-replacementDial:
				case <-time.After(5 * time.Second):
					t.Fatal("replacement dial did not begin")
				}
			}
			started := make(chan error, 1)
			go func() {
				started <- b.StartMission(&core.Mission{MissionName: "Reconnect", CaptureDelay: 1}, &core.World{WorldName: "altis"})
			}()
			require.Eventually(t, func() bool {
				b.conn.mu.Lock()
				defer b.conn.mu.Unlock()
				return b.conn.startData != nil
			}, time.Second, time.Millisecond)
			close(releaseReplacement)
			select {
			case err := <-started:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("StartMission did not receive its acknowledgement")
			}
			// A following state is a barrier: the queued initial start, if any,
			// must have been processed before this state reaches the server.
			require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 1}))
			starts := 0
			for {
				select {
				case env := <-received:
					switch env.Type {
					case streaming.TypeStartMission:
						starts++
						var payload streaming.StartMissionPayload
						require.NoError(t, json.Unmarshal(env.Payload, &payload))
						require.Equal(t, "Reconnect", payload.Mission.MissionName)
					case streaming.TypeSoldierState:
						require.Equal(t, 1, starts, "replacement connection must receive exactly one start before states")
						return
					default:
						t.Fatalf("unexpected message: %s", env.Type)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("replacement connection did not deliver the state")
				}
			}
		})
	}
}

func TestCloseCancelsPendingMissionSave(t *testing.T) {
	b := New(Config{SaveTimeout: time.Minute})
	result := make(chan error, 1)
	go func() { result <- b.EndMission("") }()
	require.Eventually(t, func() bool { n, _, _ := b.conn.log.stats(); return n > 0 }, time.Second, time.Millisecond)
	require.NoError(t, b.Close())
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel pending save")
	}
}
