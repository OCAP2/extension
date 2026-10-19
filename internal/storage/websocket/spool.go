package websocket

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	spoolReadBatch   = 4 << 20 // bytes returned by one read
	spoolHeaderBytes = 12      // seq (8) + length (4)
)

// spoolFlushInterval is how often buffered records reach the disk and the
// live file is refreshed. It must stay well below spoolStaleAfter; tests
// lower both.
var spoolFlushInterval = time.Second

// spoolSegmentBytes is the size past which a segment file is rotated; tests
// lower it.
var spoolSegmentBytes int64 = 64 << 20

var errSpoolFull = errors.New("websocket spool full")

// spoolSegment is one append-only file of consecutive records.
type spoolSegment struct {
	firstSeq uint64
	path     string
}

// spool keeps every envelope of a mission on disk until the server has saved
// the mission. The outbound log holds envelopes in memory only until they are
// acknowledged; the spool lets a producer resend the whole mission when the
// server lost it, and holds what does not fit in memory during long outages.
//
// A record is the envelope's sequence number (8 bytes, big endian), its
// length (4 bytes, big endian) and the envelope.
type spool struct {
	mu       sync.Mutex
	dir      string
	segments []spoolSegment
	file     *os.File
	w        *bufio.Writer
	segBytes int64
	total    int64
	maxBytes int64
	lastSeq  uint64 // the last record appended
	// failed is set once an append is refused; nothing more is spooled, but
	// the records already appended stay readable.
	failed error
	// ioErr is set once writing to disk fails; buffered records may be lost
	// and are no longer flushed.
	ioErr  error
	lock   *spoolLock
	done   chan struct{}
	closed bool
	logger *slog.Logger
}

// spoolLiveFile marks a spool its process is still writing. Its modification
// time is refreshed every flush; recovery leaves the spool alone while it is
// recent, so processes sharing a spool directory never recover each other's
// live missions. Close removes it.
const spoolLiveFile = "live"

// spoolCreatingInfix marks a spool directory still being created; recovery
// skips it.
const spoolCreatingInfix = ".creating-"

// openSpool creates the spool directory for one mission. maxBytes <= 0 means
// no size limit.
func openSpool(dir string, maxBytes int64, logger *slog.Logger) (*spool, error) {
	// The directory appears with its live file already inside: recovery must
	// never see a live mission's directory without it.
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create spool dir: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(dir)+spoolCreatingInfix)
	if err != nil {
		return nil, fmt.Errorf("create spool dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, spoolLiveFile), nil, 0o644); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("create spool dir: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, fmt.Errorf("create spool dir: %w", err)
	}
	// Hold the spool for as long as this process writes it. Until the lock
	// is taken, the fresh live file keeps recovery away.
	lock, err := tryLockSpool(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("lock spool dir: %w", err)
	}
	s := &spool{dir: dir, maxBytes: maxBytes, lock: lock, done: make(chan struct{}), logger: logger}
	go s.flushLoop(spoolFlushInterval)
	return s, nil
}

func (s *spool) flushLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	live := filepath.Join(s.dir, spoolLiveFile)
	for {
		select {
		case now := <-ticker.C:
			s.mu.Lock()
			s.flushLocked()
			s.mu.Unlock()
			_ = os.Chtimes(live, now, now)
		case <-s.done:
			return
		}
	}
}

// healthy reports whether every record so far is spooled and new ones will be.
func (s *spool) healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed == nil && !s.closed
}

// append stores one record. After the first failure it keeps returning that
// failure: later records are not spooled, so a full replay has a gap. The
// records appended before stay readable.
func (s *spool) append(seq uint64, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errConnectionClosed
	}
	if s.failed != nil {
		return s.failed
	}
	size := int64(spoolHeaderBytes + len(data))
	if s.maxBytes > 0 && s.total+size > s.maxBytes {
		return s.failLocked(errSpoolFull, false)
	}
	if s.file == nil || s.segBytes+size > spoolSegmentBytes {
		if err := s.rotateLocked(seq); err != nil {
			return s.failLocked(err, true)
		}
	}
	var header [spoolHeaderBytes]byte
	binary.BigEndian.PutUint64(header[:8], seq)
	binary.BigEndian.PutUint32(header[8:], uint32(len(data)))
	if _, err := s.w.Write(header[:]); err != nil {
		return s.failLocked(err, true)
	}
	if _, err := s.w.Write(data); err != nil {
		return s.failLocked(err, true)
	}
	s.segBytes += size
	s.total += size
	s.lastSeq = seq
	return nil
}

// failLocked stops spooling. ioFailure means the disk write itself failed,
// so buffered records are no longer flushed either.
func (s *spool) failLocked(err error, ioFailure bool) error {
	s.failed = err
	if ioFailure {
		s.ioErr = err
	}
	s.logger.Error("WebSocket spool stopped; later messages of this mission are not spooled", "dir", s.dir, "error", err)
	return err
}

func (s *spool) rotateLocked(firstSeq uint64) error {
	if s.file != nil {
		if err := s.w.Flush(); err != nil {
			return err
		}
		if err := s.file.Close(); err != nil {
			return err
		}
	}
	path := filepath.Join(s.dir, fmt.Sprintf("%020d.seg", firstSeq))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	s.file = f
	s.w = bufio.NewWriterSize(f, 256<<10)
	s.segBytes = 0
	s.segments = append(s.segments, spoolSegment{firstSeq: firstSeq, path: path})
	return nil
}

func (s *spool) flushLocked() {
	// A refused append leaves the buffered records intact: keep flushing
	// them. Only a failed disk write stops it.
	if s.w == nil || s.ioErr != nil || s.closed {
		return
	}
	if err := s.w.Flush(); err != nil {
		s.failLocked(err, true)
	}
}

// read returns the records numbered from..to, up to about maxBytes of
// envelopes, as far as the spool holds them. Records appended before
// spooling stopped stay readable; after a failed disk write the tail may
// be missing.
func (s *spool) read(from, to uint64, maxBytes int) ([]logEntry, error) {
	s.mu.Lock()
	s.flushLocked()
	if to > s.lastSeq {
		to = s.lastSeq
	}
	var paths []string
	for i, seg := range s.segments {
		next := uint64(0)
		if i+1 < len(s.segments) {
			next = s.segments[i+1].firstSeq
		}
		if next != 0 && next <= from {
			continue // the whole segment precedes the range
		}
		if seg.firstSeq > to {
			break
		}
		paths = append(paths, seg.path)
	}
	s.mu.Unlock()

	var out []logEntry
	bytes := 0
	for _, path := range paths {
		done, err := readSegment(path, from, to, maxBytes, &out, &bytes)
		if err != nil {
			return nil, fmt.Errorf("read spool %s: %w", filepath.Base(path), err)
		}
		if done {
			break
		}
	}
	return out, nil
}

// readSegment appends the segment's records numbered from..to to out. It
// reports true once to is reached or maxBytes is exceeded.
func readSegment(path string, from, to uint64, maxBytes int, out *[]logEntry, bytes *int) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256<<10)
	var header [spoolHeaderBytes]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return false, nil // end of what has been flushed
			}
			return false, err
		}
		seq := binary.BigEndian.Uint64(header[:8])
		n := binary.BigEndian.Uint32(header[8:])
		if seq > to {
			return true, nil
		}
		if seq < from {
			if _, err := r.Discard(int(n)); err != nil {
				return false, nil
			}
			continue
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return false, nil // a record still being flushed
		}
		*out = append(*out, logEntry{seq: seq, data: data})
		*bytes += int(n)
		if seq == to || *bytes >= maxBytes {
			return true, nil
		}
	}
}

// close flushes and closes the spool, keeping its files.
func (s *spool) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.done)
	var err error
	if s.file != nil {
		if s.ioErr == nil {
			err = s.w.Flush()
		}
		if cerr := s.file.Close(); err == nil {
			err = cerr
		}
	}
	// The mission is complete on disk; recovery may take it from here.
	if rerr := os.Remove(filepath.Join(s.dir, spoolLiveFile)); err == nil && rerr != nil && !os.IsNotExist(rerr) {
		err = rerr
	}
	s.lock.release()
	return err
}

// remove closes the spool and deletes its files.
func (s *spool) remove() error {
	_ = s.close()
	return os.RemoveAll(s.dir)
}
