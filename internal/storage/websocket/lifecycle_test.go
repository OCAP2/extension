package websocket

import (
	"encoding/json"
	"log/slog"
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

func TestClosedProxyReturnsError(t *testing.T) {
	c := newConnection(slog.Default())
	require.NoError(t, c.close())
	for i := 0; i < 20; i++ {
		require.Error(t, c.send([]byte("data")))
	}
}

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
	require.Eventually(t, func() bool { b.conn.mu.Lock(); defer b.conn.mu.Unlock(); return b.conn.conn == nil }, time.Second, time.Millisecond)
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

func TestQueueSaturationReturnsError(t *testing.T) {
	c := newConnection(slog.Default())
	for i := 0; i < cap(c.sendCh); i++ {
		require.NoError(t, c.send([]byte("data")))
	}
	require.Error(t, c.send([]byte("overflow")))
}

func TestCloseCancelsPendingMissionSave(t *testing.T) {
	b := New(Config{SaveTimeout: time.Minute})
	result := make(chan error, 1)
	go func() { result <- b.EndMission() }()
	require.Eventually(t, func() bool { return len(b.conn.sendCh) > 0 }, time.Second, time.Millisecond)
	require.NoError(t, b.Close())
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel pending save")
	}
}
