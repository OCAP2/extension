package websocket

import (
	"errors"
	"sort"
	"sync"
)

// errBufferFull is returned when the outbound log has reached its size limit.
// The rejected envelope still consumes a sequence number, so a resumable
// server sees the gap and marks the recording partial.
var errBufferFull = errors.New("websocket send buffer full")

// logEntry is one serialized envelope and its sequence number.
type logEntry struct {
	seq  uint64
	data []byte
}

// outboundLog holds a mission's envelopes from the moment they are recorded
// until they are no longer needed: until the server acknowledges them on a
// resumable connection, or until they are written on a legacy one.
//
// Each socket's writer keeps its own cursor into the log, so a replacement
// connection can resend whatever the server did not acknowledge.
//
// With a spool, every envelope is also kept on disk for the whole mission.
// Memory then holds the newest envelopes only: when it is full the oldest are
// dropped from memory instead of refusing new ones, and a writer whose cursor
// is behind memory reads from the spool.
type outboundLog struct {
	mu       sync.Mutex
	entries  []logEntry // ascending seq; holes where an append was rejected
	bytes    int
	maxBytes int
	nextSeq  uint64
	epoch    uint64        // bumped by reset so writers drop stale cursors
	changed  chan struct{} // closed and replaced by every append and reset
	rejected uint64        // envelopes refused because the log was full
	evicted  uint64        // envelopes dropped from memory, kept in the spool
	spool    *spool        // nil without spooling
}

func newOutboundLog(maxBytes int) *outboundLog {
	return &outboundLog{maxBytes: maxBytes, nextSeq: 1, changed: make(chan struct{})}
}

// reset discards everything held and numbers the next envelope 1. sp, when
// not nil, spools the new mission.
func (l *outboundLog) reset(sp *spool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
	l.bytes = 0
	l.nextSeq = 1
	l.epoch++
	l.evicted = 0
	l.spool = sp
	l.notifyLocked()
}

// currentSpool returns the mission's spool, or nil.
func (l *outboundLog) currentSpool() *spool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.spool
}

// append numbers one envelope and stores it. build receives the sequence
// number and returns the serialized envelope.
func (l *outboundLog) append(build func(seq uint64) ([]byte, error)) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	seq := l.nextSeq
	data, err := build(seq)
	if err != nil {
		return 0, err
	}
	l.nextSeq++
	spooled := l.spool != nil && l.spool.append(seq, data) == nil
	if l.maxBytes > 0 && l.bytes+len(data) > l.maxBytes {
		if !spooled {
			l.rejected++
			return seq, errBufferFull
		}
		l.evictLocked(len(data))
	}
	l.entries = append(l.entries, logEntry{seq: seq, data: data})
	l.bytes += len(data)
	l.notifyLocked()
	return seq, nil
}

// evictLocked drops the oldest entries from memory until need more bytes fit.
// They stay in the spool.
func (l *outboundLog) evictLocked(need int) {
	i := 0
	for i < len(l.entries) && l.bytes+need > l.maxBytes {
		l.bytes -= len(l.entries[i].data)
		i++
	}
	l.evicted += uint64(i)
	l.entries = append([]logEntry(nil), l.entries[i:]...)
}

// ack discards every entry numbered seq or lower.
func (l *outboundLog) ack(seq uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := sort.Search(len(l.entries), func(i int) bool { return l.entries[i].seq > seq })
	if i == 0 {
		return
	}
	for _, e := range l.entries[:i] {
		l.bytes -= len(e.data)
	}
	// Copy once the acknowledged head dominates so its memory is released.
	if i > len(l.entries)/2 {
		l.entries = append([]logEntry(nil), l.entries[i:]...)
	} else {
		l.entries = l.entries[i:]
	}
}

// after returns the entries numbered above seq, the current epoch, and a
// channel that is closed by the next append or reset. The returned entries
// are never modified, so the caller may read them without the lock.
//
// Entries no longer in memory come from the spool, a batch at a time. If the
// spool cannot provide them, the entries still in memory follow and the
// server sees the gap.
func (l *outboundLog) after(seq uint64) ([]logEntry, uint64, <-chan struct{}) {
	l.mu.Lock()
	i := sort.Search(len(l.entries), func(i int) bool { return l.entries[i].seq > seq })
	first := l.nextSeq // the first entry held in memory above seq
	if i < len(l.entries) {
		first = l.entries[i].seq
	}
	if sp := l.spool; sp != nil && seq+1 < first {
		epoch, changed := l.epoch, l.changed
		l.mu.Unlock()
		disk, err := sp.read(seq+1, first-1, spoolReadBatch)
		if err == nil && len(disk) > 0 {
			return disk, epoch, changed
		}
		l.mu.Lock()
		i = sort.Search(len(l.entries), func(i int) bool { return l.entries[i].seq > seq })
	}
	defer l.mu.Unlock()
	return l.entries[i:len(l.entries):len(l.entries)], l.epoch, l.changed
}

// currentEpoch returns the epoch a new cursor belongs to.
func (l *outboundLog) currentEpoch() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.epoch
}

// lastSeq returns the number given to the most recent envelope.
func (l *outboundLog) lastSeq() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.nextSeq - 1
}

// stats reports how much the log holds and how many envelopes it refused.
func (l *outboundLog) stats() (entries, bytes int, rejected uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries), l.bytes, l.rejected
}

// evictedCount reports how many envelopes were dropped from memory into the spool.
func (l *outboundLog) evictedCount() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.evicted
}

func (l *outboundLog) notifyLocked() {
	close(l.changed)
	l.changed = make(chan struct{})
}
