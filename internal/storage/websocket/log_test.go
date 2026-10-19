package websocket

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendN(t *testing.T, l *outboundLog, n int) {
	t.Helper()
	for range n {
		_, err := l.append(func(seq uint64) ([]byte, error) { return []byte(fmt.Sprintf("m%d", seq)), nil })
		require.NoError(t, err)
	}
}

func seqs(entries []logEntry) []uint64 {
	out := make([]uint64, len(entries))
	for i, e := range entries {
		out[i] = e.seq
	}
	return out
}

func TestOutboundLogNumbersFromOne(t *testing.T) {
	l := newOutboundLog(0)
	appendN(t, l, 3)
	entries, _, _ := l.after(0)
	assert.Equal(t, []uint64{1, 2, 3}, seqs(entries))
	assert.Equal(t, uint64(3), l.lastSeq())
}

func TestOutboundLogAfterAndAck(t *testing.T) {
	l := newOutboundLog(0)
	appendN(t, l, 5)

	entries, _, _ := l.after(2)
	assert.Equal(t, []uint64{3, 4, 5}, seqs(entries))

	l.ack(3)
	entries, _, _ = l.after(0)
	assert.Equal(t, []uint64{4, 5}, seqs(entries))
	n, bytes, _ := l.stats()
	assert.Equal(t, 2, n)
	assert.Equal(t, len("m4")+len("m5"), bytes)

	l.ack(1) // stale ack is a no-op
	entries, _, _ = l.after(0)
	assert.Equal(t, []uint64{4, 5}, seqs(entries))

	l.ack(100)
	entries, _, _ = l.after(0)
	assert.Empty(t, entries)
	_, bytes, _ = l.stats()
	assert.Zero(t, bytes)
}

func TestOutboundLogRejectsOverLimitButConsumesSeq(t *testing.T) {
	l := newOutboundLog(4) // room for two "mN" entries
	appendN(t, l, 2)
	seq, err := l.append(func(seq uint64) ([]byte, error) { return []byte("m3"), nil })
	require.ErrorIs(t, err, errBufferFull)
	assert.Equal(t, uint64(3), seq)

	// Space freed by an ack is reused; the refused number stays a hole.
	l.ack(1)
	appendN(t, l, 1)
	entries, _, _ := l.after(0)
	assert.Equal(t, []uint64{2, 4}, seqs(entries))
	_, _, rejected := l.stats()
	assert.Equal(t, uint64(1), rejected)
}

func TestOutboundLogBuildErrorConsumesNothing(t *testing.T) {
	l := newOutboundLog(0)
	_, err := l.append(func(uint64) ([]byte, error) { return nil, fmt.Errorf("boom") })
	require.Error(t, err)
	appendN(t, l, 1)
	entries, _, _ := l.after(0)
	assert.Equal(t, []uint64{1}, seqs(entries))
}

func TestOutboundLogResetStartsNewEpoch(t *testing.T) {
	l := newOutboundLog(0)
	appendN(t, l, 3)
	_, epoch, _ := l.after(0)
	l.reset(nil)
	appendN(t, l, 1)
	entries, epoch2, _ := l.after(0)
	assert.NotEqual(t, epoch, epoch2)
	assert.Equal(t, []uint64{1}, seqs(entries))
}

func TestOutboundLogChangedSignalsAppend(t *testing.T) {
	l := newOutboundLog(0)
	_, _, changed := l.after(0)
	select {
	case <-changed:
		t.Fatal("changed closed before any append")
	default:
	}
	appendN(t, l, 1)
	select {
	case <-changed:
	default:
		t.Fatal("append did not signal waiting writers")
	}
}

func TestOutboundLogAfterIsStableUnderAck(t *testing.T) {
	l := newOutboundLog(0)
	appendN(t, l, 10)
	entries, _, _ := l.after(0)
	l.ack(8) // compacts the backing array
	appendN(t, l, 5)
	// The batch a writer already holds is unaffected.
	assert.Equal(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, seqs(entries))
	assert.Equal(t, "m10", string(entries[9].data))
}
