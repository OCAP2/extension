package websocket

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

// fastRecovery shortens the staleness window for the duration of a test,
// keeping the heartbeat well inside it as in production.
func fastRecovery(t *testing.T) {
	t.Helper()
	oldStale, oldFlush := spoolStaleAfter, spoolFlushInterval
	spoolStaleAfter, spoolFlushInterval = 300*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { spoolStaleAfter, spoolFlushInterval = oldStale, oldFlush })
}

// crash stops a backend the way a game server crash does: the connection
// drops and nothing more is written, but the spool is not closed, so its
// live file stays behind and goes stale.
func crash(t *testing.T, b *Backend) {
	t.Helper()
	sp := b.conn.log.currentSpool()
	require.NotNil(t, sp)
	sp.mu.Lock()
	require.NoError(t, sp.w.Flush()) // what the last flush tick wrote
	require.NoError(t, sp.file.Close())
	close(sp.done)
	sp.closed = true
	sp.lock.release() // what the operating system does for a dead process
	sp.mu.Unlock()
	b.closeOnce.Do(func() { close(b.done) })
	b.loops.Wait()
	_ = b.conn.close()
}

func recoveryConfig(url, spoolDir string) Config {
	cfg := fastConfig(url)
	cfg.SpoolDir = spoolDir
	cfg.RecoveryInterval = 50 * time.Millisecond
	return cfg
}

func endPayload(t *testing.T, m fakeMission) streaming.EndMissionPayload {
	t.Helper()
	require.NotEmpty(t, m.received)
	last := m.received[len(m.received)-1]
	require.Equal(t, streaming.TypeEndMission, last.Type)
	var p streaming.EndMissionPayload
	if string(last.Payload) != "null" {
		require.NoError(t, json.Unmarshal(last.Payload, &p))
	}
	return p
}

func TestRecoveryAfterCrashResumesWaitingSession(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()

	a := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, a.Init())
	id := startResumable(t, a)
	recordStates(t, a, 1, 20)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 21 }, 2*time.Second, 5*time.Millisecond)
	rs.discard.Store(true) // the last states never reach the server
	recordStates(t, a, 21, 30)
	require.Eventually(t, func() bool { return a.conn.sent.Load() == 31 }, 2*time.Second, 5*time.Millisecond)
	crash(t, a)
	rs.discard.Store(false)

	// The game server restarts: a new process with a new producer id.
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()
	require.NotEqual(t, a.producerID, b.producerID)

	require.Eventually(t, func() bool { return rs.mission(id).completed }, 5*time.Second, 10*time.Millisecond)
	m := rs.mission(id)
	requireContiguous(t, m, 32)
	assert.True(t, endPayload(t, m).Interrupted, "a crashed mission is ended as interrupted")
	assert.Equal(t, int32(1), rs.starts.Load(), "the waiting session is resumed, not restarted")
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(spoolDir, id)); return os.IsNotExist(err) }, 2*time.Second, 10*time.Millisecond)
}

func TestRecoveryResendsMissionTheServerLost(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()

	a := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, a.Init())
	id := startResumable(t, a)
	recordStates(t, a, 1, 15)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 16 }, 2*time.Second, 5*time.Millisecond)
	crash(t, a)
	rs.forget() // the web server restarted as well

	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()

	require.Eventually(t, func() bool { return rs.mission(id).completed }, 5*time.Second, 10*time.Millisecond)
	m := rs.mission(id)
	requireContiguous(t, m, 17) // start, 15 states, interrupted end
	assert.Equal(t, streaming.TypeStartMission, m.received[0].Type)
	assert.True(t, endPayload(t, m).Interrupted)
}

func TestRecoveryOfUnconfirmedSave(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	cfg := recoveryConfig(rs.url(), spoolDir)
	cfg.SaveTimeout = 200 * time.Millisecond
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	rs.discard.Store(true) // end_mission is lost and the save times out
	require.Error(t, b.EndMission("Final"))
	rs.discard.Store(false)
	rs.kill()

	// The closed spool is picked up by the same process' recovery loop.
	require.Eventually(t, func() bool { return rs.mission(id).completed }, 5*time.Second, 10*time.Millisecond)
	m := rs.mission(id)
	requireContiguous(t, m, 7)
	p := endPayload(t, m)
	assert.Equal(t, "Final", p.Tag, "the recorded end_mission is the one sent")
	assert.False(t, p.Interrupted)
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(spoolDir, id)); return os.IsNotExist(err) }, 2*time.Second, 10*time.Millisecond)
}

func TestRecoveryDropsMissionAlreadySaved(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()

	a := New(fastConfig(rs.url()))
	require.NoError(t, a.Init())
	id := startResumable(t, a)
	recordStates(t, a, 1, 3)
	require.NoError(t, a.EndMission(""))
	require.NoError(t, a.Close())
	require.True(t, rs.mission(id).completed)

	// The process died before it could delete its spool.
	writeTestSpool(t, filepath.Join(spoolDir, id), id, a.producerID, 3)
	received := len(rs.mission(id).received)
	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(spoolDir, id)); return os.IsNotExist(err) }, 5*time.Second, 10*time.Millisecond)
	assert.Len(t, rs.mission(id).received, received, "nothing is resent for a saved mission")
}

func TestRecoveryLeavesLiveSpoolAlone(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()

	a := New(fastConfig(rs.url()))
	a.cfg.SpoolDir = spoolDir
	require.NoError(t, a.Init())
	defer a.Close()
	id := startResumable(t, a)

	// Another game server sharing the spool directory scans it repeatedly,
	// even after the live file went stale (a stalled disk, a paused VM): the
	// writer's lock alone keeps it out.
	b := New(recoveryConfig(rs.url(), spoolDir))
	stale := time.Now().Add(-2 * spoolStaleAfter)
	for range 5 {
		require.NoError(t, os.Chtimes(filepath.Join(spoolDir, id, spoolLiveFile), stale, stale))
		b.recoverSpools(b.done)
		time.Sleep(spoolStaleAfter / 4)
	}
	_, err := tryLockSpool(filepath.Join(spoolDir, id))
	assert.ErrorIs(t, err, errSpoolLocked, "the writer holds its spool")
	assert.Equal(t, int32(0), rs.resumes.Load(), "a live mission is never recovered")
	recordStates(t, a, 1, 3)
	require.NoError(t, a.EndMission(""))
	requireContiguous(t, rs.mission(id), 5)
}

func TestRecoveryUnsupportedServerKeepsSpool(t *testing.T) {
	fastRecovery(t)
	old := handshakeTimeout
	handshakeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { handshakeTimeout = old })
	srv := silentServer(t) // never answers resume, like an older web server
	defer srv.Close()

	spoolDir := t.TempDir()
	dir := filepath.Join(spoolDir, "m-legacy")
	writeTestSpool(t, dir, "m-legacy", "p-old", 3)

	b := New(recoveryConfig(wsURL(srv), spoolDir))
	b.recoverSpools(b.done)
	assert.DirExists(t, dir)
	lock, err := tryLockSpool(dir)
	require.NoError(t, err, "the lock is released for a later attempt")
	lock.release()
}

func TestRecoveryDiscardsUnusableSpools(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()

	empty := filepath.Join(spoolDir, "empty")
	require.NoError(t, os.MkdirAll(empty, 0o755))
	old := filepath.Join(spoolDir, "old")
	writeTestSpool(t, old, "m-old", "p-old", 2)
	ancient := time.Now().Add(-spoolMaxAge - time.Hour)
	segs, _, err := spoolSegments(old)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(segs[0], ancient, ancient))
	garbage := filepath.Join(spoolDir, "garbage")
	require.NoError(t, os.MkdirAll(garbage, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(garbage, "00000000000000000001.seg"), []byte("not a record at all"), 0o644))
	corrupt := filepath.Join(spoolDir, "corrupt") // a whole first record that is not JSON
	sp, err := openSpool(corrupt, 0, slog.Default())
	require.NoError(t, err)
	require.NoError(t, sp.append(1, []byte("{not json")))
	require.NoError(t, sp.close())

	b := New(recoveryConfig(rs.url(), spoolDir))
	b.recoverSpools(b.done)
	assert.NoDirExists(t, empty)
	assert.NoDirExists(t, old)
	assert.NoDirExists(t, garbage)
	assert.NoDirExists(t, corrupt, "a corrupt spool is not retried forever")
	assert.Zero(t, rs.resumes.Load())
}

func TestSpoolLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	lock, err := tryLockSpool(dir)
	require.NoError(t, err)
	_, err = tryLockSpool(dir)
	require.ErrorIs(t, err, errSpoolLocked, "a held lock is respected")
	lock.release()
	lock.release() // idempotent

	lock, err = tryLockSpool(dir)
	require.NoError(t, err, "a released lock can be taken again")
	lock.release()
}

func TestSpoolLockConcurrentContenders(t *testing.T) {
	// Many recoverers race for the same abandoned spool: exactly one wins,
	// every round.
	for round := range 20 {
		dir := t.TempDir()
		const contenders = 64
		var wg sync.WaitGroup
		var winners atomic.Int32
		locks := make(chan *spoolLock, contenders)
		start := make(chan struct{})
		for range contenders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if l, err := tryLockSpool(dir); err == nil {
					winners.Add(1)
					locks <- l
				}
			}()
		}
		close(start)
		wg.Wait()
		close(locks)
		require.Equal(t, int32(1), winners.Load(), "round %d", round)
		for l := range locks {
			l.release()
		}
	}
}

// TestSpoolLockHolderHelper holds a spool lock in a separate process until
// it is killed, for TestSpoolLockReleasedWhenHolderDies.
func TestSpoolLockHolderHelper(t *testing.T) {
	dir := os.Getenv("OCAP_TEST_LOCK_DIR")
	if dir == "" {
		t.Skip("run by TestSpoolLockReleasedWhenHolderDies")
	}
	_, err := tryLockSpool(dir)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "held"), nil, 0o644))
	time.Sleep(time.Minute) // killed long before
}

func TestSpoolLockReleasedWhenHolderDies(t *testing.T) {
	dir := t.TempDir()
	holder := exec.Command(os.Args[0], "-test.run=^TestSpoolLockHolderHelper$", "-test.count=1")
	holder.Env = append(os.Environ(), "OCAP_TEST_LOCK_DIR="+dir)
	require.NoError(t, holder.Start())
	t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, "held")); return err == nil }, 30*time.Second, 20*time.Millisecond)

	_, err := tryLockSpool(dir)
	require.ErrorIs(t, err, errSpoolLocked, "the other process holds it")

	require.NoError(t, holder.Process.Kill()) // the holder crashes
	_ = holder.Wait()
	lock, err := tryLockSpool(dir)
	require.NoError(t, err, "the operating system releases a dead holder's lock")
	lock.release()
}

func TestRecoverySkipsSpoolHeldByAnotherRecoverer(t *testing.T) {
	fastRecovery(t)
	rs := newResumeServer(t)
	spoolDir := t.TempDir()
	dir := filepath.Join(spoolDir, "m-held")
	writeTestSpool(t, dir, "m-held", "p-old", 3)

	other, err := tryLockSpool(dir) // another process is recovering it
	require.NoError(t, err)
	b := New(recoveryConfig(rs.url(), spoolDir))
	b.recoverSpools(b.done)
	assert.Zero(t, rs.resumes.Load())
	assert.DirExists(t, dir)

	other.release() // the other recoverer gave up
	b.recoverSpools(b.done)
	assert.True(t, rs.mission("m-held").completed)
	assert.NoDirExists(t, dir)
}

func TestRecoverableHonorsLiveFile(t *testing.T) {
	fastRecovery(t)
	dir := t.TempDir()
	assert.True(t, recoverable(dir), "no live file: closed or never opened")
	live := filepath.Join(dir, spoolLiveFile)
	require.NoError(t, os.WriteFile(live, nil, 0o644))
	assert.False(t, recoverable(dir))
	stale := time.Now().Add(-2 * spoolStaleAfter)
	require.NoError(t, os.Chtimes(live, stale, stale))
	assert.True(t, recoverable(dir))
}

func TestScanSpoolStopsAtTruncatedRecord(t *testing.T) {
	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 5)
	require.NoError(t, sp.close())
	path := sp.segments[0].path
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(path, info.Size()-3)) // a crash mid-write

	var got []uint64
	require.NoError(t, scanSpool([]string{path}, func(seq uint64, _ []byte) error {
		got = append(got, seq)
		return nil
	}))
	assert.Equal(t, []uint64{1, 2, 3, 4}, got)
}

func TestSpoolLiveFileLifecycle(t *testing.T) {
	sp := newTestSpool(t, 0)
	live := filepath.Join(sp.dir, spoolLiveFile)
	assert.FileExists(t, live)
	require.NoError(t, sp.close())
	assert.NoFileExists(t, live, "a closed spool is ready for recovery")
}

// writeTestSpool writes a closed spool holding a start_mission and n soldier
// states, as a crashed producer leaves it.
func writeTestSpool(t *testing.T, dir, missionID, producerID string, n int) {
	t.Helper()
	sp, err := openSpool(dir, 0, slog.Default())
	require.NoError(t, err)
	start, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: &core.Mission{MissionName: "Recovered", CaptureDelay: 1}, World: &core.World{WorldName: "altis"},
		MissionID: missionID, ProducerID: producerID, ProtocolVersion: streaming.ProtocolVersionResume,
	}, 1)
	require.NoError(t, err)
	require.NoError(t, sp.append(1, start))
	for i := 1; i <= n; i++ {
		data, err := marshalEnvelope(streaming.TypeSoldierState, &core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(i)}, uint64(i+1))
		require.NoError(t, err)
		require.NoError(t, sp.append(uint64(i+1), data))
	}
	require.NoError(t, sp.close())
}

func TestRecoveryPausesForServerWithoutResume(t *testing.T) {
	fastRecovery(t)
	oldHandshake, oldPause := handshakeTimeout, recoveryUnsupportedPause
	handshakeTimeout, recoveryUnsupportedPause = 100*time.Millisecond, time.Hour
	t.Cleanup(func() { handshakeTimeout, recoveryUnsupportedPause = oldHandshake, oldPause })

	rs := newResumeServer(t)
	rs.legacy.Store(true)
	spoolDir := t.TempDir()
	writeTestSpool(t, filepath.Join(spoolDir, "m-left"), "m-left", "p-old", 3)

	b := New(recoveryConfig(rs.url(), spoolDir)) // checks every 50ms
	require.NoError(t, b.Init())
	defer b.Close()

	require.Eventually(t, func() bool { return rs.resumes.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, int32(1), rs.resumes.Load(), "an older server is not asked again every interval")
	assert.DirExists(t, filepath.Join(spoolDir, "m-left"))

	// The web server is upgraded; the next mission start shows it.
	rs.legacy.Store(false)
	startResumable(t, b)
	require.Eventually(t, func() bool { return rs.mission("m-left").completed }, 5*time.Second, 10*time.Millisecond,
		"recovery resumes as soon as the server supports it")
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(spoolDir, "m-left")); return os.IsNotExist(err) }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, b.EndMission(""))
}

func TestRecoveryPauseExpires(t *testing.T) {
	fastRecovery(t)
	oldHandshake, oldPause := handshakeTimeout, recoveryUnsupportedPause
	handshakeTimeout, recoveryUnsupportedPause = 100*time.Millisecond, 400*time.Millisecond
	t.Cleanup(func() { handshakeTimeout, recoveryUnsupportedPause = oldHandshake, oldPause })

	rs := newResumeServer(t)
	rs.legacy.Store(true)
	spoolDir := t.TempDir()
	writeTestSpool(t, filepath.Join(spoolDir, "m-left"), "m-left", "p-old", 3)

	b := New(recoveryConfig(rs.url(), spoolDir))
	require.NoError(t, b.Init())
	defer b.Close()
	require.Eventually(t, func() bool { return rs.resumes.Load() >= 2 }, 3*time.Second, 10*time.Millisecond, "asked again once the pause is over")
}
