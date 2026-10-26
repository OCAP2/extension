package websocket

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/pkg/streaming"
)

const (
	defaultRecoveryInterval = time.Minute
	spoolMaxAge             = 7 * 24 * time.Hour
)

var (
	// spoolStaleAfter is how long a spool's live file may go untouched before
	// its process is presumed gone; tests lower it. Exclusion rests on the
	// spool lock; the live file only covers the moment between a new spool
	// directory appearing and its writer taking the lock.
	spoolStaleAfter = 15 * time.Second
	// recoveryUnsupportedPause is how long recovery waits after a server that
	// does not answer resume; a resumable mission start ends it sooner.
	recoveryUnsupportedPause = 30 * time.Minute
	// errRecoveryUnsupported means the server does not answer resume.
	errRecoveryUnsupported = errors.New("server does not support mission recovery")
	// errSpoolDiscarded means the spool holds nothing recoverable and is deleted.
	errSpoolDiscarded = errors.New("spool discarded")
	// errAlreadySaved means the server saved the mission before; the spool
	// is a leftover and is deleted.
	errAlreadySaved = errors.New("mission already saved")
	// errDiscardFinished means the spool held a mission the addon discarded
	// and the server no longer keeps it; the spool is deleted.
	errDiscardFinished = errors.New("mission discard finished")
	// errRecoveryDeferred means the spooled mission is this process' own and
	// waits until it records no other one; the spool is kept.
	errRecoveryDeferred = errors.New("mission recovery deferred")
)

// recoveryLoop resends missions left in the spool directory: by a game
// server crash, by a save the server never confirmed, or by an earlier
// process. It runs until the backend is closed.
func (b *Backend) recoveryLoop(done <-chan struct{}) {
	interval := b.cfg.RecoveryInterval
	if interval <= 0 {
		interval = defaultRecoveryInterval
	}
	for {
		if paused := b.recoveryPausedUntil.Load(); paused == 0 || time.Now().UnixNano() >= paused {
			b.recoverSpools(done)
		}
		select {
		case <-done:
			return
		case <-b.recoveryWake:
		case <-time.After(interval):
		}
	}
}

// resumeRecovery lifts a pause set because the server could not take
// recovered missions, once a mission start shows it now can.
func (b *Backend) resumeRecovery() {
	if b.recoveryPausedUntil.Swap(0) == 0 {
		return
	}
	b.logger().Info("WebSocket server supports mission recovery; recovering spooled missions")
	select {
	case b.recoveryWake <- struct{}{}:
	default:
	}
}

// recoverSpools makes one pass over the spool directory.
func (b *Backend) recoverSpools(done <-chan struct{}) {
	entries, err := os.ReadDir(b.cfg.SpoolDir)
	if err != nil {
		if !os.IsNotExist(err) {
			b.logger().Warn("WebSocket spool directory unreadable", "dir", b.cfg.SpoolDir, "error", err)
		}
		return
	}
	for _, e := range entries {
		select {
		case <-done:
			return
		default:
		}
		if !e.IsDir() || strings.Contains(e.Name(), spoolCreatingInfix) {
			continue // not a spool, or one still being created
		}
		dir := filepath.Join(b.cfg.SpoolDir, e.Name())
		if !recoverable(dir) {
			continue
		}
		// Its writer, or another recovering process, holds the lock.
		lock, err := tryLockSpool(dir)
		if err != nil {
			continue
		}
		err = b.recoverMission(dir, done)
		finished := err == nil || errors.Is(err, errAlreadySaved) || errors.Is(err, errDiscardFinished) || errors.Is(err, errSpoolDiscarded)
		if finished {
			// Marked while locked: a process taking the spool before it is
			// gone deletes it instead of resending it.
			_ = markSpoolDropped(dir)
		}
		lock.release() // before removal: Windows cannot delete an open file
		switch {
		case err == nil:
			b.recovered.Add(1)
			b.logger().Info("WebSocket recovered mission from spool", "dir", dir)
			_ = os.RemoveAll(dir)
		case errors.Is(err, errAlreadySaved):
			b.logger().Info("WebSocket spooled mission was already saved by the server; spool removed", "dir", dir)
			_ = os.RemoveAll(dir)
		case errors.Is(err, errDiscardFinished):
			b.logger().Info("WebSocket discarded mission finished from spool; spool removed", "dir", dir)
			_ = os.RemoveAll(dir)
		case errors.Is(err, errSpoolDiscarded):
			_ = os.RemoveAll(dir)
		case errors.Is(err, errRecoveryDeferred):
			b.logger().Debug("WebSocket mission recovery waits until the current mission ends", "dir", dir)
		case errors.Is(err, errRecoveryUnsupported):
			// Asking an older server every minute only fills both logs.
			b.recoveryPausedUntil.Store(time.Now().Add(recoveryUnsupportedPause).UnixNano())
			b.logger().Warn("WebSocket server cannot recover missions; spools kept until it is upgraded",
				"dir", dir, "retryIn", recoveryUnsupportedPause)
			return // the same server cannot take the others either
		default:
			b.logger().Warn("WebSocket mission recovery failed; will retry", "dir", dir, "error", err)
		}
	}
}

// recoverable reports whether the spool is not one just being opened: a
// fresh live file means its writer may not hold the lock yet.
func recoverable(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, spoolLiveFile))
	if err == nil && time.Since(info.ModTime()) < spoolStaleAfter {
		return false
	}
	return err == nil || os.IsNotExist(err)
}

// spoolSegments lists a spool's segment files in sequence order.
func spoolSegments(dir string) ([]string, time.Time, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, time.Time{}, err
	}
	var paths []string
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".seg") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	sort.Strings(paths) // names are zero-padded first sequence numbers
	return paths, newest, nil
}

// scanSpool calls fn for every record of the spool, in order. A truncated
// final record, as a crash leaves it, ends the scan.
func scanSpool(paths []string, fn func(seq uint64, data []byte) error) error {
	for _, path := range paths {
		if err := scanSegment(path, fn); err != nil {
			return err
		}
	}
	return nil
}

func scanSegment(path string, fn func(seq uint64, data []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256<<10)
	var header [spoolHeaderBytes]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		data := make([]byte, binary.BigEndian.Uint32(header[8:]))
		if _, err := io.ReadFull(r, data); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil // a record cut short by a crash
			}
			return err
		}
		if err := fn(binary.BigEndian.Uint64(header[:8]), data); err != nil {
			return err
		}
	}
}

// recoverMission resends one spooled mission to the server and ends it. The
// mission is resumed as the producer that recorded it, so the server
// continues a session that is still waiting, or takes the whole mission when
// it no longer has one.
func (b *Backend) recoverMission(dir string, done <-chan struct{}) error {
	if spoolDropped(dir) {
		return errSpoolDiscarded // its writer or a recovery was removing it
	}
	paths, newest, err := spoolSegments(dir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errSpoolDiscarded // nothing was recorded
	}
	if time.Since(newest) > spoolMaxAge {
		b.logger().Warn("WebSocket spool too old to recover; discarding", "dir", dir, "age", time.Since(newest).Round(time.Hour))
		return errSpoolDiscarded
	}

	var start streaming.Envelope
	var payload streaming.StartMissionPayload
	if err := scanSegment(paths[0], func(seq uint64, data []byte) error {
		if json.Unmarshal(data, &start) != nil || json.Unmarshal(start.Payload, &payload) != nil {
			start = streaming.Envelope{} // corrupt: not a mission we can resume
		}
		return io.EOF // only the first record is needed
	}); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read start: %w", err)
	}
	if start.Type != streaming.TypeStartMission || payload.MissionID == "" {
		b.logger().Warn("WebSocket spool does not start with a resumable mission; discarding", "dir", dir)
		return errSpoolDiscarded
	}

	c := newConnection(b.logger(), b.cfg.connOptions())
	c.wsURL, c.secret = b.cfg.URL, b.cfg.Secret
	conn, err := c.dialOnce()
	if err != nil {
		return err
	}
	defer conn.Close()

	// A mission the addon discarded is not resent: the server only needs to
	// hear the discard it did not confirm. Read once the server is reachable:
	// it scans the spool's last segment.
	discardSeq, err := spooledDiscard(paths)
	if err != nil {
		return fmt.Errorf("read end: %w", err)
	}
	discarding := discardSeq > 0

	// Give up when the backend closes.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-done:
				_ = conn.Close()
				return
			case <-stop:
				return
			}
		}
	}()

	resume, err := marshalEnvelope(streaming.TypeResume, streaming.ResumePayload{MissionID: payload.MissionID, ProducerID: payload.ProducerID}, 0)
	if err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	if err := writeData(conn, resume); err != nil {
		return err
	}
	reply, err := readServerMessage(conn, func(m streaming.AckMessage) bool {
		return m.Type == streaming.ServerResumeOK || m.Type == streaming.ServerResumeUnknown || m.Type == streaming.ServerResumeCompleted
	})
	if err != nil {
		var netErr interface{ Timeout() bool }
		if errors.As(err, &netErr) && netErr.Timeout() {
			// An older server ignores resume. A discarded spool exists only
			// for a server that could discard, so it is kept as well: the
			// server is more likely slow than downgraded.
			return errRecoveryUnsupported
		}
		return fmt.Errorf("resume: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	if discarding {
		switch {
		case reply.Type == streaming.ServerResumeCompleted:
			return errDiscardFinished // the server confirmed the discard before
		case reply.ProtocolVersion < streaming.ProtocolVersionDiscard:
			// The server was replaced by an older one: nothing more it can do,
			// and it saves what it holds as partial.
			return errDiscardFinished
		}
		// resume_ok: the server still holds the session. resume_unknown: it
		// may still keep the mission, saved as partial after the producer
		// went away; the discard names it.
	}

	from := uint64(1) // resume_unknown: the whole mission, start included
	switch reply.Type {
	case streaming.ServerResumeCompleted:
		return errAlreadySaved
	case streaming.ServerResumeOK:
		from = reply.Seq + 1
	case streaming.ServerResumeUnknown:
		// A server older than ProtocolVersionDiscard takes any start as the
		// producer's next mission and saves its live one as partial. This
		// process' own mission waits until it records none.
		if !discarding && payload.ProducerID == b.producerID && b.recording() && !b.discardable() {
			return errRecoveryDeferred
		}
	}

	// Read acknowledgements concurrently: the server acknowledges as it goes,
	// and closes the connection if it cannot save the mission.
	ackFor := streaming.TypeEndMission
	if discarding {
		ackFor = streaming.TypeDiscardMission
	}
	ended := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				ended <- fmt.Errorf("connection lost before the mission was finished: %w", err)
				return
			}
			var m streaming.AckMessage
			if json.Unmarshal(raw, &m) == nil && m.Type == streaming.ServerAck && m.For == ackFor {
				ended <- nil
				return
			}
		}
	}()

	if discarding {
		b.logger().Info("WebSocket finishing a discarded mission", "missionId", payload.MissionID, "serverHasIt", reply.Type == streaming.ServerResumeOK)
		// On a resumed session, numbered after everything the server holds,
		// so it is not taken for a duplicate. Without one, it is not numbered.
		var seq uint64
		if reply.Type == streaming.ServerResumeOK {
			seq = max(discardSeq, reply.Seq+1)
		}
		discardPayload := streaming.DiscardMissionPayload{MissionID: payload.MissionID, ProducerID: payload.ProducerID}
		discard, err := marshalEnvelope(streaming.TypeDiscardMission, discardPayload, seq)
		if err != nil {
			return err
		}
		if err := writeData(conn, discard); err != nil {
			return err
		}
		if err := b.awaitRecovered(ended, done); err != nil {
			return err
		}
		return errDiscardFinished
	}
	b.logger().Info("WebSocket recovering mission", "missionId", payload.MissionID, "resumeFrom", from, "serverHasIt", reply.Type == streaming.ServerResumeOK)

	var lastSeq uint64
	var lastData []byte
	first := true
	pace := newPacer(b.cfg.connOptions().withDefaults().resendRate)
	if err := scanSpool(paths, func(seq uint64, data []byte) error {
		lastSeq, lastData = seq, data
		if seq < from {
			first = false
			return nil
		}
		if first {
			// A resent start must not take the producer's live mission
			// for one it left behind.
			first = false
			var err error
			if data, err = markRecovered(data); err != nil {
				return fmt.Errorf("mark start recovered: %w", err)
			}
		}
		if !pace.wait(len(data), done, nil) {
			return errConnectionClosed
		}
		return writeData(conn, data)
	}); err != nil {
		return err
	}

	// A mission cut short by a crash has no end_mission: end it, marked
	// interrupted so the server saves it as partial. Number it after
	// everything the server holds: a crash loses the records still buffered
	// for the spool, so the server may be ahead of the disk, and a smaller
	// number would be discarded as a duplicate.
	var last streaming.Envelope
	if json.Unmarshal(lastData, &last) != nil || last.Type != streaming.TypeEndMission {
		endSeq := max(lastSeq, from-1) + 1
		end, err := marshalEnvelope(streaming.TypeEndMission, streaming.EndMissionPayload{Interrupted: true}, endSeq)
		if err != nil {
			return err
		}
		if err := writeData(conn, end); err != nil {
			return err
		}
	}
	return b.awaitRecovered(ended, done)
}

// awaitRecovered waits for the server to acknowledge the recovered mission's
// end or discard.
func (b *Backend) awaitRecovered(ended <-chan error, done <-chan struct{}) error {
	select {
	case err := <-ended:
		return err
	case <-time.After(b.saveTimeout()):
		return fmt.Errorf("timeout waiting for the server to finish the recovered mission")
	case <-done:
		return errConnectionClosed
	}
}

// markRecovered sets Recovered on a spooled start_mission envelope, keeping
// every other field as recorded.
func markRecovered(data []byte) ([]byte, error) {
	var env streaming.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(env.Payload, &fields); err != nil {
		return nil, err
	}
	fields["recovered"] = json.RawMessage("true")
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return encodeEnvelope(env.Type, raw, env.Seq)
}

// spooledDiscard returns the sequence number of the spool's discard_mission
// record, or 0 when the spool does not end with one.
func spooledDiscard(paths []string) (uint64, error) {
	// Only the last record matters; a rotated segment may still be empty.
	for _, path := range slices.Backward(paths) {
		var lastSeq uint64
		var lastData []byte
		if err := scanSegment(path, func(seq uint64, data []byte) error {
			lastSeq, lastData = seq, data
			return nil
		}); err != nil {
			return 0, err
		}
		if lastData == nil {
			continue
		}
		var last streaming.Envelope
		if json.Unmarshal(lastData, &last) != nil || last.Type != streaming.TypeDiscardMission {
			return 0, nil
		}
		return lastSeq, nil
	}
	return 0, nil
}
