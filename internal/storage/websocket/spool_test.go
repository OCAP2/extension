package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

func newTestSpool(t *testing.T, maxBytes int64) *spool {
	t.Helper()
	sp, err := openSpool(filepath.Join(t.TempDir(), "mission"), maxBytes, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sp.close() })
	return sp
}

func spoolN(t *testing.T, sp *spool, from, to uint64) {
	t.Helper()
	for seq := from; seq <= to; seq++ {
		require.NoError(t, sp.append(seq, fmt.Appendf(nil, "envelope-%d", seq)))
	}
}

func TestSpoolReadsRangeIncludingUnflushed(t *testing.T) {
	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 100) // nothing flushed yet: read must flush first

	entries, err := sp.read(10, 20, spoolReadBatch)
	require.NoError(t, err)
	require.Equal(t, uint64(10), entries[0].seq)
	assert.Equal(t, []uint64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, seqs(entries))
	assert.Equal(t, "envelope-15", string(entries[5].data))
}

func TestSpoolReadsAcrossSegments(t *testing.T) {
	old := spoolSegmentBytes
	spoolSegmentBytes = 100 // a few records per segment
	t.Cleanup(func() { spoolSegmentBytes = old })

	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 50)
	require.Greater(t, len(sp.segments), 5)

	entries, err := sp.read(1, 50, spoolReadBatch)
	require.NoError(t, err)
	require.Len(t, entries, 50)
	for i, e := range entries {
		require.Equal(t, uint64(i+1), e.seq)
	}

	entries, err = sp.read(23, 27, spoolReadBatch)
	require.NoError(t, err)
	assert.Equal(t, []uint64{23, 24, 25, 26, 27}, seqs(entries))
}

func TestSpoolReadBatchLimit(t *testing.T) {
	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 100)
	entries, err := sp.read(1, 100, 30) // "envelope-N" is 10-12 bytes
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 2, 3}, seqs(entries))
}

func TestSpoolReadClampsToLastSeq(t *testing.T) {
	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 5)
	entries, err := sp.read(4, 99, spoolReadBatch)
	require.NoError(t, err)
	assert.Equal(t, []uint64{4, 5}, seqs(entries))
}

func TestSpoolLimitStopsSpooling(t *testing.T) {
	sp := newTestSpool(t, 100)
	var err error
	for seq := uint64(1); err == nil && seq < 100; seq++ {
		err = sp.append(seq, []byte("0123456789"))
	}
	require.ErrorIs(t, err, errSpoolFull)
	assert.False(t, sp.healthy())
	require.ErrorIs(t, sp.append(1000, []byte("x")), errSpoolFull, "a spool with a hole stays failed")
	// The records spooled before the limit stay readable.
	entries, err := sp.read(1, 10, spoolReadBatch)
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 2, 3, 4}, seqs(entries))
}

func TestSpoolCloseKeepsAndRemoveDeletes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mission")
	sp, err := openSpool(dir, 0, slog.Default())
	require.NoError(t, err)
	spoolN(t, sp, 1, 3)
	require.NoError(t, sp.close())
	require.NoError(t, sp.close(), "close is idempotent")
	segs, _, err := spoolSegments(dir)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	info, err := os.Stat(segs[0])
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "close flushes")
	assert.NoFileExists(t, filepath.Join(dir, spoolLiveFile))
	lock, err := tryLockSpool(dir)
	require.NoError(t, err, "close releases the spool lock")
	lock.release()
	require.ErrorIs(t, sp.append(4, []byte("x")), errConnectionClosed)

	require.NoError(t, sp.remove())
	assert.NoDirExists(t, dir)
}

func TestSpoolFlushesPeriodically(t *testing.T) {
	sp := newTestSpool(t, 0)
	spoolN(t, sp, 1, 3)
	require.Eventually(t, func() bool {
		info, err := os.Stat(sp.segments[0].path)
		return err == nil && info.Size() > 0
	}, 3*spoolFlushInterval, 50*time.Millisecond)
}

func TestOpenSpoolFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	_, err := openSpool(filepath.Join(file, "mission"), 0, slog.Default())
	require.Error(t, err)
}

// --- outbound log with a spool ---

func TestOutboundLogEvictsToSpoolInsteadOfRejecting(t *testing.T) {
	sp := newTestSpool(t, 0)
	l := newOutboundLog(20) // memory for about two entries
	l.reset(sp)
	appendN(t, l, 10)
	n, bytes, rejected := l.stats()
	assert.LessOrEqual(t, bytes, 20)
	assert.Less(t, n, 10)
	assert.Zero(t, rejected)
	assert.Equal(t, uint64(10-n), l.evictedCount())

	// A writer behind memory reads the rest from disk, in order.
	var got []uint64
	cursor := uint64(0)
	for cursor < 10 {
		entries, _, _ := l.after(cursor)
		require.NotEmpty(t, entries)
		for _, e := range entries {
			got = append(got, e.seq)
			cursor = e.seq
		}
	}
	assert.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, got)
}

func TestOutboundLogReplaysAcknowledgedFromSpool(t *testing.T) {
	sp := newTestSpool(t, 0)
	l := newOutboundLog(0)
	l.reset(sp)
	appendN(t, l, 5)
	l.ack(5) // the old session acknowledged everything

	entries, _, _ := l.after(1) // a full resend after start_mission
	assert.Equal(t, []uint64{2, 3, 4, 5}, seqs(entries))
}

func TestOutboundLogUnreadableSpoolFallsBackToMemory(t *testing.T) {
	sp := newTestSpool(t, 0)
	l := newOutboundLog(0)
	l.reset(sp)
	appendN(t, l, 5)
	l.ack(2)
	require.NoError(t, sp.close())
	require.NoError(t, os.RemoveAll(sp.dir)) // the spool's files are gone

	entries, _, _ := l.after(0)
	assert.Equal(t, []uint64{3, 4, 5}, seqs(entries), "memory still serves what it holds")
}

// appendSized appends n envelopes of 10 bytes each.
func appendSized(t *testing.T, l *outboundLog, n int) []error {
	t.Helper()
	var errs []error
	for range n {
		_, err := l.append(func(seq uint64) ([]byte, error) { return fmt.Appendf(nil, "env-%06d", seq), nil })
		errs = append(errs, err)
	}
	return errs
}

// readAll follows the log from cursor like a writer does.
func readAll(l *outboundLog, cursor uint64) []uint64 {
	var got []uint64
	for range 100 {
		entries, _, _ := l.after(cursor)
		if len(entries) == 0 {
			return got
		}
		for _, e := range entries {
			got = append(got, e.seq)
			cursor = e.seq
		}
	}
	return got
}

func TestOutboundLogKeepsSpooledPrefixWhenSpoolFills(t *testing.T) {
	// Memory for two 10-byte envelopes; a spool for exactly four records.
	sp := newTestSpool(t, 4*(spoolHeaderBytes+10))
	l := newOutboundLog(20)
	l.reset(sp)
	for i, err := range appendSized(t, l, 4) {
		require.NoError(t, err, "envelope %d", i+1)
	}
	require.Equal(t, uint64(2), l.evictedCount(), "1 and 2 live only in the spool")
	assert.Equal(t, []uint64{1, 2, 3, 4}, readAll(l, 0))

	// The fifth envelope fills the spool: it is refused, nothing else is lost.
	errs := appendSized(t, l, 1)
	require.ErrorIs(t, errs[0], errBufferFull)
	require.False(t, sp.healthy())
	assert.Equal(t, []uint64{1, 2, 3, 4}, readAll(l, 0), "accepted envelopes stay replayable")
}

func TestSpoolFillFlushesBufferedRecords(t *testing.T) {
	sp := newTestSpool(t, 2*(spoolHeaderBytes+10))
	require.NoError(t, sp.append(1, []byte("0123456789")))
	require.NoError(t, sp.append(2, []byte("0123456789")))
	require.ErrorIs(t, sp.append(3, []byte("0123456789")), errSpoolFull)
	require.NoError(t, sp.close())

	// What reached the spool before the limit is on disk for recovery.
	var got []uint64
	require.NoError(t, scanSpool([]string{sp.segments[0].path}, func(seq uint64, _ []byte) error {
		got = append(got, seq)
		return nil
	}))
	assert.Equal(t, []uint64{1, 2}, got)
}

// --- end to end with the resumable test server ---

func spoolConfig(t *testing.T, url string) Config {
	cfg := fastConfig(url)
	cfg.SpoolDir = t.TempDir()
	return cfg
}

func TestResumeUnknownResendsFullMissionFromSpool(t *testing.T) {
	rs := newResumeServer(t)
	b := New(spoolConfig(t, rs.url()))
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 10)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 11 }, 2*time.Second, 5*time.Millisecond)

	// The web server restarts and loses the mission and everything after it.
	rs.discard.Store(true)
	recordStates(t, b, 11, 20)
	require.Eventually(t, func() bool { return b.conn.sent.Load() == 21 }, 2*time.Second, 5*time.Millisecond)
	rs.forget()
	rs.discard.Store(false)
	rs.kill()

	recordStates(t, b, 21, 25)
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 26 }, 5*time.Second, 5*time.Millisecond)
	require.NoError(t, b.EndMission(""))

	// The new session holds the whole mission: no gap, nothing partial.
	requireContiguous(t, rs.mission(id), 27)
}

func TestLongOutageSpillsToDisk(t *testing.T) {
	rs := newResumeServer(t)
	cfg := spoolConfig(t, rs.url())
	cfg.BufferBytes = 4 << 10 // a few dozen envelopes in memory
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	rs.discard.Store(true) // nothing arrives during the outage
	recordStates(t, b, 1, 2000)
	assert.Positive(t, b.conn.log.evictedCount(), "memory overflowed into the spool")
	_, _, rejected := b.conn.log.stats()
	assert.Zero(t, rejected, "nothing was refused")

	rs.discard.Store(false)
	rs.kill()
	require.Eventually(t, func() bool { return rs.lastSeq(id) == 2001 }, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), 2002)
}

func TestSpoolRemovedAfterSavedMission(t *testing.T) {
	rs := newResumeServer(t)
	cfg := spoolConfig(t, rs.url())
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	require.DirExists(t, filepath.Join(cfg.SpoolDir, id))
	require.NoError(t, b.EndMission(""))
	assert.NoDirExists(t, filepath.Join(cfg.SpoolDir, id))
}

func TestSpoolKeptWhenSaveUnconfirmed(t *testing.T) {
	rs := newResumeServer(t)
	cfg := spoolConfig(t, rs.url())
	cfg.SaveTimeout = 200 * time.Millisecond
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	recordStates(t, b, 1, 5)
	rs.discard.Store(true) // end_mission never arrives
	require.Error(t, b.EndMission(""))

	dir := filepath.Join(cfg.SpoolDir, id)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, files, "the unsaved mission stays on disk")

	// The kept spool holds the whole mission, end_mission included.
	var out []logEntry
	var n int
	_, err = readSegment(filepath.Join(dir, files[0].Name()), 1, ^uint64(0), 1<<30, &out, &n)
	require.NoError(t, err)
	require.Len(t, out, 7)
	var last streaming.Envelope
	require.NoError(t, json.Unmarshal(out[6].data, &last))
	assert.Equal(t, streaming.TypeEndMission, last.Type)
}

func TestSpoolUnavailableRecordsWithoutIt(t *testing.T) {
	rs := newResumeServer(t)
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	cfg := fastConfig(rs.url())
	cfg.SpoolDir = file // cannot hold directories
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	id := startResumable(t, b)
	assert.Nil(t, b.conn.log.currentSpool())
	require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: 1}))
	require.NoError(t, b.EndMission(""))
	requireContiguous(t, rs.mission(id), 3)
}
