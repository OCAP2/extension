package websocket

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

var _ storage.Discardable = (*Backend)(nil)

// requireGone waits until path no longer exists.
func requireGone(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool { _, err := os.Stat(path); return os.IsNotExist(err) }, 5*time.Second, 10*time.Millisecond)
}

// requireNoDiscard checks that no discard_mission reached the server.
func requireNoDiscard(t *testing.T, msgs []streaming.Envelope) {
	t.Helper()
	for _, env := range msgs {
		require.NotEqual(t, streaming.TypeDiscardMission, env.Type)
	}
}

func TestDiscardMissionDropsRecording(t *testing.T) {
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	require.NoError(t, b.DiscardMission())

	m := rs.mission(id)
	require.True(t, m.discarded)
	requireContiguous(t, m, 7) // start, 5 states, discard
	assert.Equal(t, streaming.TypeDiscardMission, m.received[6].Type)
	var p streaming.DiscardMissionPayload
	require.NoError(t, json.Unmarshal(m.received[6].Payload, &p))
	assert.Equal(t, streaming.DiscardMissionPayload{MissionID: id, ProducerID: b.producerID}, p, "the discard names the mission")
	requireGone(t, filepath.Join(spoolDir, id)) // a confirmed discard leaves nothing to recover

	// The next mission records as usual.
	next := startResumable(t, b)
	assert.NotEqual(t, id, next)
	recordStates(t, b, 1, 2)
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(next).completed)
	assert.False(t, rs.mission(next).discarded)
}

func TestDiscardMissionWithoutMissionIsNoop(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.DiscardMission(), "nothing recorded yet")
	id := startResumable(t, b)
	require.NoError(t, b.EndMission(""))
	require.NoError(t, b.DiscardMission(), "the mission was already saved")
	m := rs.mission(id)
	assert.True(t, m.completed)
	assert.False(t, m.discarded)
	requireNoDiscard(t, m.received)
}

func TestDiscardAckLostResolvedByResume(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 3)
	rs.dropEnd.Store(true) // the discard is applied but its ack never arrives

	done := make(chan error, 1)
	go func() { done <- b.DiscardMission() }()
	select {
	case err := <-done:
		require.NoError(t, err, "resume_completed stands in for the lost discard_mission ack")
	case <-time.After(5 * time.Second):
		t.Fatal("DiscardMission did not complete")
	}
	assert.True(t, rs.mission(id).discarded)
	assert.Equal(t, int32(1), rs.starts.Load(), "the discarded mission is not started again")
}

func TestDiscardMissionOnServerWithoutDiscard(t *testing.T) {
	rs := newResumeServer(t)
	rs.noDrop.Store(true)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 3)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 4 }, 2*time.Second, 5*time.Millisecond)

	require.ErrorIs(t, b.DiscardMission(), storage.ErrMissionKept)
	m := rs.mission(id)
	assert.False(t, m.completed, "the server keeps the mission until its grace expires")
	requireNoDiscard(t, m.received)
	requireGone(t, filepath.Join(spoolDir, id)) // recovery must not resend a discarded mission
	require.Eventually(t, func() bool {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		return len(rs.conns) == 0
	}, 2*time.Second, 10*time.Millisecond, "the connection is closed so the server stops waiting")

	// The next mission dials a fresh connection.
	next := startResumable(t, b)
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(next).completed)
}

func TestDiscardMissionOnLegacyServer(t *testing.T) {
	srv, ml := testServer(t)
	defer srv.Close()
	b := New(Config{URL: wsURL(srv), Secret: "s"})
	require.NoError(t, b.Init())
	defer b.Close()

	require.NoError(t, b.StartMission(&core.Mission{MissionName: "M"}, &core.World{WorldName: "W"}))
	require.ErrorIs(t, b.DiscardMission(), errDiscardUnsupported)
	requireNoDiscard(t, ml.all())
}

// discardUnconfirmed discards the mission while discard_mission is lost in
// transit, so the discard times out and its spool stays behind.
func discardUnconfirmed(t *testing.T, rs *resumeServer, spoolDir string) (*Backend, string) {
	t.Helper()
	cfg := recoveryConfig(rs.url(), spoolDir)
	cfg.SaveTimeout = 200 * time.Millisecond // below the ack stall, so no reconnect resends it
	cfg.RecoveryInterval = time.Hour         // recovery runs only when the test triggers it
	b := New(cfg)
	require.NoError(t, b.Init())
	t.Cleanup(func() { _ = b.Close() })

	id := startResumable(t, b)
	recordStates(t, b, 1, 3)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 4 }, 2*time.Second, 5*time.Millisecond)
	rs.discard.Store(true)
	require.Error(t, b.DiscardMission())
	rs.discard.Store(false)
	rs.kill()
	require.DirExists(t, filepath.Join(spoolDir, id), "the spool is kept to finish the discard")
	return b, id
}

func TestRecoveryFinishesUnconfirmedDiscard(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b, id := discardUnconfirmed(t, rs, spoolDir)

	b.recoverSpools(b.done)

	m := rs.mission(id)
	require.True(t, m.discarded)
	requireContiguous(t, m, 5) // nothing is resent: start, 3 states, discard
	assert.Zero(t, m.dups)
	assert.NoDirExists(t, filepath.Join(spoolDir, id))
}

func TestRecoveryDropsPartialOfUnconfirmedDiscard(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b, id := discardUnconfirmed(t, rs, spoolDir)
	rs.expire(id) // the producer stayed away past the grace: saved as partial
	require.True(t, rs.partial(id))

	b.recoverSpools(b.done)

	assert.False(t, rs.partial(id), "the partial recording is dropped")
	assert.Equal(t, []string{id}, rs.droppedMissions())
	assert.Empty(t, rs.mission(id).received, "the mission is not resent to be discarded")
	assert.Equal(t, int32(1), rs.starts.Load())
	assert.NoDirExists(t, filepath.Join(spoolDir, id))
}

func TestRecoveryOfDiscardTheServerLost(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b, id := discardUnconfirmed(t, rs, spoolDir)
	rs.forget() // the web server restarted

	b.recoverSpools(b.done)

	assert.Equal(t, []string{id}, rs.droppedMissions(), "the discard names the mission")
	assert.Equal(t, int32(1), rs.starts.Load(), "the mission is not resent to be discarded")
	assert.NoDirExists(t, filepath.Join(spoolDir, id))
}

func TestRecoveryOfDiscardOnServerWithoutDiscard(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	rs.noDrop.Store(true) // the web server was downgraded since
	spoolDir := t.TempDir()
	writeDiscardedSpool(t, filepath.Join(spoolDir, "m-old"), "m-old", "p-old", 3)

	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	requireGone(t, filepath.Join(spoolDir, "m-old"))
	assert.Empty(t, rs.droppedMissions(), "an older server is not sent what it cannot take")
	assert.Zero(t, rs.starts.Load())
}

// writeDiscardedSpool writes a closed spool holding a start_mission, n
// soldier states and a discard_mission.
func writeDiscardedSpool(t *testing.T, dir, missionID, producerID string, n int) {
	t.Helper()
	sp, err := openSpool(dir, 0, slog.Default())
	require.NoError(t, err)
	start, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: &core.Mission{MissionName: "Discarded", CaptureDelay: 1}, World: &core.World{WorldName: "altis"},
		MissionID: missionID, ProducerID: producerID, ProtocolVersion: streaming.ProtocolVersionDiscard,
	}, 1)
	require.NoError(t, err)
	require.NoError(t, sp.append(1, start))
	for i := 1; i <= n; i++ {
		data, err := marshalEnvelope(streaming.TypeSoldierState, &core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(i)}, uint64(i+1))
		require.NoError(t, err)
		require.NoError(t, sp.append(uint64(i+1), data))
	}
	discard, err := marshalEnvelope(streaming.TypeDiscardMission, streaming.DiscardMissionPayload{MissionID: missionID, ProducerID: producerID}, uint64(n+2))
	require.NoError(t, err)
	require.NoError(t, sp.append(uint64(n+2), discard))
	require.NoError(t, sp.close())
}

func TestRecoveryKeepsDiscardWhenResumeTimesOut(t *testing.T) {
	fastRecovery(t)
	oldHandshake := handshakeTimeout
	handshakeTimeout = 100 * time.Millisecond
	t.Cleanup(func() { handshakeTimeout = oldHandshake })

	rs := newResumeServer(t)
	rs.legacy.Store(true) // does not answer resume, like a slow server
	spoolDir := t.TempDir()
	writeDiscardedSpool(t, filepath.Join(spoolDir, "m-kept"), "m-kept", "p-old", 3)

	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	require.Eventually(t, func() bool { return b.recoveryPausedUntil.Load() != 0 }, 5*time.Second, 10*time.Millisecond)
	assert.DirExists(t, filepath.Join(spoolDir, "m-kept"), "the discard is retried once the server answers")
	assert.Empty(t, rs.mission("m-kept").received)
}

func TestSpooledDiscard(t *testing.T) {
	oldSegment := spoolSegmentBytes
	spoolSegmentBytes = 256 // a few records per segment
	t.Cleanup(func() { spoolSegmentBytes = oldSegment })

	dir := filepath.Join(t.TempDir(), "m")
	writeTestSpool(t, dir, "m", "p", 10)
	paths, _, err := spoolSegments(dir)
	require.NoError(t, err)
	require.Greater(t, len(paths), 1, "the spool spans several segments")
	seq, err := spooledDiscard(paths)
	require.NoError(t, err)
	assert.Zero(t, seq, "a spool ending with mission data was not discarded")

	discarded := filepath.Join(t.TempDir(), "d")
	writeDiscardedSpool(t, discarded, "d", "p", 10)
	paths, _, err = spoolSegments(discarded)
	require.NoError(t, err)
	seq, err = spooledDiscard(paths)
	require.NoError(t, err)
	assert.Equal(t, uint64(12), seq)
}

func TestAbandonedMissionEndsAsInterrupted(t *testing.T) {
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	first := startResumable(t, b)
	recordStates(t, b, 1, 3)

	// The game moved on to the next mission without saving or discarding.
	second := startResumable(t, b)
	m := rs.mission(first)
	require.True(t, m.completed, "the abandoned mission is ended before the next one starts")
	assert.True(t, endPayload(t, m).Interrupted)
	requireContiguous(t, m, 5) // start, 3 states, end
	assert.False(t, rs.partial(first))
	requireGone(t, filepath.Join(spoolDir, first)) // nothing is left to recover

	recordStates(t, b, 1, 2)
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(second).completed)
	assert.False(t, rs.partial(second))
	assert.Equal(t, int32(2), rs.starts.Load())
}

func TestRecoveredMissionDoesNotSupersedeLiveOne(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	live := startResumable(t, b)
	recordStates(t, b, 1, 3)
	// A spool of this producer the server no longer holds, such as a save it
	// never confirmed before a web restart; another process sharing the
	// spool directory may recover it the same way.
	writeTestSpool(t, filepath.Join(spoolDir, "m-left"), "m-left", b.producerID, 4)

	require.Eventually(t, func() bool { return rs.mission("m-left").completed }, 5*time.Second, 10*time.Millisecond)
	start := rs.mission("m-left").received[0]
	var p streaming.StartMissionPayload
	require.NoError(t, json.Unmarshal(start.Payload, &p))
	assert.True(t, p.Recovered, "the resent start is marked recovered")
	assert.Equal(t, "m-left", p.MissionID)
	assert.False(t, rs.partial(live), "the live mission is left alone")

	recordStates(t, b, 4, 5)
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(live), 7) // start, 5 states, end
}

func TestFailedStartLeavesNoMission(t *testing.T) {
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	oldAck := ackTimeout
	ackTimeout = 200 * time.Millisecond
	t.Cleanup(func() { ackTimeout = oldAck })
	rs.muteStart.Store(true)
	require.Error(t, b.StartMission(&core.Mission{MissionName: "M"}, &core.World{WorldName: "W"}))
	assert.False(t, b.recording(), "a mission that did not start is not abandoned later")
	require.Eventually(t, func() bool {
		entries, err := os.ReadDir(spoolDir)
		return err == nil && len(entries) == 0
	}, 5*time.Second, 10*time.Millisecond, "its spool is removed")

	rs.muteStart.Store(false)
	id := startResumable(t, b)
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(id).completed)
	assert.Equal(t, int32(2), rs.starts.Load(), "the next start does not end the failed one first")
}

func TestAbandonedEmptyMissionIsDiscarded(t *testing.T) {
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	// Registered, but the game moved on before anything was recorded.
	first := startResumable(t, b)
	second := startResumable(t, b)
	m := rs.mission(first)
	assert.True(t, m.discarded, "an empty recording is not kept")
	requireGone(t, filepath.Join(spoolDir, first))
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(second).completed)
}

func TestAbandonedEmptyMissionOnServerWithoutDiscard(t *testing.T) {
	rs := newResumeServer(t)
	rs.noDrop.Store(true)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	first := startResumable(t, b)
	startResumable(t, b)
	m := rs.mission(first)
	assert.True(t, m.completed, "ended, since this server cannot drop it")
	assert.False(t, m.discarded)
	requireNoDiscard(t, m.received)
	assert.True(t, endPayload(t, m).Interrupted)
}

func TestPendingDiscardRefusedAfterServerDowngrade(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url())) // SaveTimeout 5 s
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 3)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 4 }, 2*time.Second, 5*time.Millisecond)

	// The link dies with the discard in flight, and the web server comes back
	// without discard support.
	rs.discard.Store(true)
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- b.DiscardMission() }()
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 4 }, time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // the discard is logged and written into the dead link
	rs.noDrop.Store(true)
	rs.discard.Store(false)
	rs.kill()

	select {
	case err := <-done:
		require.ErrorIs(t, err, storage.ErrMissionKept, "the older server refuses the waiting discard")
		assert.Less(t, time.Since(start), 4*time.Second, "without waiting for the save timeout")
	case <-time.After(10 * time.Second):
		t.Fatal("DiscardMission did not complete")
	}
	assert.False(t, rs.mission(id).discarded)
}

func TestRecoveryDeletesDroppedSpool(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	dir := filepath.Join(spoolDir, "m-dropped")
	writeTestSpool(t, dir, "m-dropped", "p-old", 3)
	// Its writer was removing it when recovery took it.
	require.NoError(t, markSpoolDropped(dir))

	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	requireGone(t, dir)
	assert.Zero(t, rs.starts.Load(), "a dropped spool is never resent")
}

func TestAbandonedMissionUnconfirmedKeepsSpool(t *testing.T) {
	oldAbandon := abandonTimeout
	abandonTimeout = 200 * time.Millisecond // below the ack stall, so no reconnect resends it
	t.Cleanup(func() { abandonTimeout = oldAbandon })
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	cfg := recoveryConfig(rs.url(), spoolDir)
	cfg.RecoveryInterval = time.Hour
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	first := startResumable(t, b)
	recordStates(t, b, 1, 3)
	require.Eventually(t, func() bool { return rs.lastSeq(first) == 4 }, 2*time.Second, 5*time.Millisecond)
	rs.loseEnd.Store(true) // the interrupted end never arrives
	second := startResumable(t, b)
	assert.NotEqual(t, first, second)
	assert.False(t, rs.mission(first).completed)
	assert.DirExists(t, filepath.Join(spoolDir, first), "kept for recovery")
	rs.loseEnd.Store(false)
	require.NoError(t, b.EndMission(""))
	assert.True(t, rs.mission(second).completed)
}

func TestDiscardAfterServerDowngradeIsNotSent(t *testing.T) {
	rs := newResumeServer(t)
	b := New(fastConfig(rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	require.True(t, b.discardable())
	recordStates(t, b, 1, 3)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 4 }, 2*time.Second, 5*time.Millisecond)

	// The web server is replaced by one without discard; the mission resumes.
	rs.noDrop.Store(true)
	resumes := rs.resumes.Load()
	rs.kill()
	require.Eventually(t, func() bool { return rs.resumes.Load() > resumes }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return !b.discardable() }, 5*time.Second, 10*time.Millisecond)

	require.ErrorIs(t, b.DiscardMission(), storage.ErrMissionKept)
	requireNoDiscard(t, rs.mission(id).received)
}

func TestRecoveryOfDiscardAfterServerDowngrade(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	b, id := discardUnconfirmed(t, rs, spoolDir)
	rs.noDrop.Store(true) // the server holds the session but cannot discard

	b.recoverSpools(b.done)

	assert.False(t, rs.mission(id).discarded)
	requireNoDiscard(t, rs.mission(id).received)
	assert.NoDirExists(t, filepath.Join(spoolDir, id), "an older server cannot do more")
}
