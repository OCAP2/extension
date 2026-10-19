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
	"sort"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/pkg/streaming"
)

const (
	defaultRecoveryInterval = time.Minute
	spoolClaimDir           = "claim"
	spoolMaxAge             = 7 * 24 * time.Hour
)

var (
	// spoolStaleAfter is how long a spool's live file may go untouched before
	// its process is presumed dead; tests lower it.
	spoolStaleAfter = 2 * time.Minute
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
		if !recoverable(dir) || !claimSpool(dir) {
			continue
		}
		err := b.recoverMission(dir, done)
		switch {
		case err == nil:
			b.recovered.Add(1)
			b.logger().Info("WebSocket recovered mission from spool", "dir", dir)
			_ = os.RemoveAll(dir)
		case errors.Is(err, errAlreadySaved):
			b.logger().Info("WebSocket spooled mission was already saved by the server; spool removed", "dir", dir)
			_ = os.RemoveAll(dir)
		case errors.Is(err, errSpoolDiscarded):
			_ = os.RemoveAll(dir)
		case errors.Is(err, errRecoveryUnsupported):
			releaseSpool(dir)
			// Asking an older server every minute only fills both logs.
			b.recoveryPausedUntil.Store(time.Now().Add(recoveryUnsupportedPause).UnixNano())
			b.logger().Warn("WebSocket server cannot recover missions; spools kept until it is upgraded",
				"dir", dir, "retryIn", recoveryUnsupportedPause)
			return // the same server cannot take the others either
		default:
			releaseSpool(dir)
			b.logger().Warn("WebSocket mission recovery failed; will retry", "dir", dir, "error", err)
		}
	}
}

// recoverable reports whether no live process owns the spool.
func recoverable(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, spoolLiveFile))
	if err == nil && time.Since(info.ModTime()) < spoolStaleAfter {
		return false // its process is still writing it
	}
	return err == nil || os.IsNotExist(err)
}

// claimSpool reserves a spool for this process. The claim is a directory,
// created atomically; a claim left by a process that died is broken once
// stale. A recovering process refreshes its claim while it works.
func claimSpool(dir string) bool {
	claim := filepath.Join(dir, spoolClaimDir)
	if err := os.Mkdir(claim, 0o755); err == nil {
		return true
	} else if !os.IsExist(err) {
		return false
	}
	info, err := os.Stat(claim)
	if err != nil || time.Since(info.ModTime()) < spoolStaleAfter {
		return false
	}
	if os.Remove(claim) != nil {
		return false
	}
	return os.Mkdir(claim, 0o755) == nil
}

func releaseSpool(dir string) { _ = os.Remove(filepath.Join(dir, spoolClaimDir)) }

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

	// Keep the claim fresh while working, and give up when the backend closes.
	stop := make(chan struct{})
	defer close(stop)
	refresh := spoolStaleAfter / 4
	go func() {
		ticker := time.NewTicker(refresh)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				_ = os.Chtimes(filepath.Join(dir, spoolClaimDir), now, now)
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
			return errRecoveryUnsupported // an older server ignores resume
		}
		return fmt.Errorf("resume: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	from := uint64(1) // resume_unknown: the whole mission, start included
	switch reply.Type {
	case streaming.ServerResumeCompleted:
		return errAlreadySaved
	case streaming.ServerResumeOK:
		from = reply.Seq + 1
	}
	b.logger().Info("WebSocket recovering mission", "missionId", payload.MissionID, "resumeFrom", from, "serverHasIt", reply.Type == streaming.ServerResumeOK)

	// Read acknowledgements concurrently: the server acknowledges as it goes,
	// and closes the connection if it cannot save the mission.
	ended := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				ended <- fmt.Errorf("connection lost before the mission was saved: %w", err)
				return
			}
			var m streaming.AckMessage
			if json.Unmarshal(raw, &m) == nil && m.Type == streaming.ServerAck && m.For == streaming.TypeEndMission {
				ended <- nil
				return
			}
		}
	}()

	var lastSeq uint64
	var lastData []byte
	pace := newPacer(b.cfg.connOptions().withDefaults().resendRate)
	if err := scanSpool(paths, func(seq uint64, data []byte) error {
		lastSeq, lastData = seq, data
		if seq < from {
			return nil
		}
		if !pace.wait(len(data), done, nil) {
			return errConnectionClosed
		}
		return writeData(conn, data)
	}); err != nil {
		return err
	}

	// A mission cut short by a crash has no end_mission: end it, marked
	// interrupted so the server saves it as partial.
	var last streaming.Envelope
	if json.Unmarshal(lastData, &last) != nil || last.Type != streaming.TypeEndMission {
		end, err := marshalEnvelope(streaming.TypeEndMission, streaming.EndMissionPayload{Interrupted: true}, lastSeq+1)
		if err != nil {
			return err
		}
		if err := writeData(conn, end); err != nil {
			return err
		}
	}

	timeout := b.cfg.SaveTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	select {
	case err := <-ended:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for the server to save the recovered mission")
	case <-done:
		return errConnectionClosed
	}
}
