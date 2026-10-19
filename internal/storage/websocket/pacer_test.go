package websocket

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
)

func TestPacerSpacesWritesToRate(t *testing.T) {
	p := newPacer(400_000) // burst: 100 kB
	start := time.Now()
	require.True(t, p.wait(100_000, nil, nil), "the burst passes at once")
	assert.Less(t, time.Since(start), 50*time.Millisecond)

	require.True(t, p.wait(200_000, nil, nil)) // 200 kB more at 400 kB/s
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond)
	assert.Less(t, elapsed, 1500*time.Millisecond)
}

func TestPacerDisabled(t *testing.T) {
	for _, p := range []*pacer{nil, newPacer(-1)} {
		start := time.Now()
		for range 100 {
			require.True(t, p.wait(1<<20, nil, nil))
		}
		assert.Less(t, time.Since(start), 50*time.Millisecond)
	}
}

func TestPacerStopsWhenDone(t *testing.T) {
	p := newPacer(1000)
	done := make(chan struct{})
	go func() { time.Sleep(20 * time.Millisecond); close(done) }()
	start := time.Now()
	assert.False(t, p.wait(1_000_000, done, nil))
	assert.Less(t, time.Since(start), time.Second)
}

func TestDefaultResendRate(t *testing.T) {
	assert.Equal(t, defaultResendBytesPerSec, connOptions{}.withDefaults().resendRate)
	assert.Equal(t, -1, connOptions{resendRate: -1}.withDefaults().resendRate, "negative disables pacing")
}

// stateBytes is the size of one recorded soldier state envelope.
func stateBytes(t *testing.T) int {
	data, err := marshalEnvelope("soldier_state", &core.SoldierState{SoldierID: 1, CaptureFrame: 1000}, 1000)
	require.NoError(t, err)
	return len(data)
}

func TestBacklogAfterReconnectIsPaced(t *testing.T) {
	rs := newResumeServer(t)
	cfg := fastConfig(rs.url())
	cfg.ResendBytesPerSec = 100_000
	cfg.AckStallTimeout = 10 * time.Second // the dead link is cut by hand
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	rs.discard.Store(true) // the outage: nothing arrives
	const n = 300
	recordStates(t, b, 1, n)
	require.Eventually(t, func() bool { return b.conn.sent.Load() == n+1 }, 5*time.Second, 5*time.Millisecond)
	rs.discard.Store(false)

	start := time.Now()
	rs.kill()
	require.Eventually(t, func() bool { return rs.lastSeq(id) == n+1 }, 20*time.Second, 10*time.Millisecond)
	elapsed := time.Since(start)

	// The backlog minus the burst, at the paced rate.
	expected := time.Duration(float64(n*stateBytes(t)-25_000) / 100_000 * float64(time.Second))
	assert.GreaterOrEqual(t, elapsed, expected*8/10, "backlog resent faster than the rate allows")
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), n+2)
}

func TestLiveTrafficIsNotPaced(t *testing.T) {
	rs := newResumeServer(t)
	cfg := fastConfig(rs.url())
	cfg.ResendBytesPerSec = 1000 // would take minutes if live data were paced
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	start := time.Now()
	recordStates(t, b, 1, 300)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 301 }, 2*time.Second, 5*time.Millisecond)
	assert.Less(t, time.Since(start), 2*time.Second)
	require.NoError(t, b.EndMission(""))
}

func TestRecoveryIsPaced(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	const n = 300
	writeTestSpool(t, filepath.Join(spoolDir, "m-paced"), "m-paced", "p-old", n)

	cfg := recoveryConfig(rs.url(), spoolDir)
	cfg.ResendBytesPerSec = 100_000
	b := New(cfg)
	start := time.Now()
	b.recoverSpools(b.done)
	elapsed := time.Since(start)

	require.True(t, rs.mission("m-paced").completed)
	expected := time.Duration(float64(n*stateBytes(t)-25_000) / 100_000 * float64(time.Second))
	assert.GreaterOrEqual(t, elapsed, expected*8/10, "recovery resent faster than the rate allows")
}
