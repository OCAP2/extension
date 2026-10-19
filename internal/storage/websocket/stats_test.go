package websocket

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs routes the default logger into a buffer for the test.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestStatsReportsStream(t *testing.T) {
	rs := newResumeServer(t)
	b := New(spoolConfig(t, rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	assert.Empty(t, b.Stats().MissionID, "no mission yet")
	id := startResumable(t, b)
	recordStates(t, b, 1, 10)
	require.Eventually(t, func() bool { s := b.Stats(); return s.Acked == 11 && s.BufferedEnvelopes == 0 }, 2*time.Second, 5*time.Millisecond)

	s := b.Stats()
	assert.Equal(t, id, s.MissionID)
	assert.True(t, s.Connected)
	assert.True(t, s.Resumable)
	assert.Equal(t, uint64(11), s.Sent)
	assert.True(t, s.Spooling)
	assert.Zero(t, s.Rejected)
	assert.Zero(t, s.Reconnects)
}

func TestStatsCountsRecovery(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 6 }, 2*time.Second, 5*time.Millisecond)
	rs.kill()
	require.Eventually(t, func() bool { s := b.Stats(); return s.Resumes == 1 && s.Connected }, 5*time.Second, 5*time.Millisecond)

	rs.forget()
	rs.kill()
	// A restart is counted during the handshake, the reconnect once the
	// socket is attached: wait for both.
	require.Eventually(t, func() bool { s := b.Stats(); return s.Restarts == 1 && s.Reconnects == 2 }, 5*time.Second, 5*time.Millisecond)

	// Totals survive the connection being replaced for the next mission.
	require.NoError(t, b.EndMission(""))
	startResumable(t, b)
	s := b.Stats()
	assert.Equal(t, uint64(1), s.Resumes)
	assert.Equal(t, uint64(1), s.Restarts)
	assert.Equal(t, uint64(2), s.Reconnects)
	require.NoError(t, b.EndMission(""))
}

func TestStatsLogged(t *testing.T) {
	logs := captureLogs(t)
	rs := newResumeServer(t)
	cfg := fastConfig(rs.url())
	cfg.StatsInterval = 20 * time.Millisecond
	cfg.ReconnectMin, cfg.ReconnectMax = time.Hour, time.Hour // stay disconnected once dropped
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	time.Sleep(60 * time.Millisecond)
	assert.NotContains(t, logs.String(), "WebSocket stream", "nothing to report without a mission")

	startResumable(t, b)
	require.Eventually(t, func() bool { return strings.Contains(logs.String(), "level=INFO msg=\"WebSocket stream\"") }, 2*time.Second, 10*time.Millisecond)

	rs.kill()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "level=WARN msg=\"WebSocket stream disconnected, buffering\"")
	}, 2*time.Second, 10*time.Millisecond)
}

func TestStatsCountsRecoveredMissions(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	writeTestSpool(t, filepath.Join(spoolDir, "m-1"), "m-1", "p-old", 3)
	writeTestSpool(t, filepath.Join(spoolDir, "m-2"), "m-2", "p-old", 3)
	require.NoError(t, os.MkdirAll(filepath.Join(spoolDir, "empty"), 0o755))

	b := New(recoveryConfig(rs.url(), spoolDir))
	b.recoverSpools(b.done)
	assert.Equal(t, uint64(2), b.Stats().Recovered, "discarded spools are not counted")
}
