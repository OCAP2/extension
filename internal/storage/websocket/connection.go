package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/OCAP2/extension/v5/pkg/streaming"
)

const (
	defaultBufferBytes  = 256 << 20 // unacknowledged envelopes held in memory
	defaultAckStall     = 20 * time.Second
	defaultReconnectMin = time.Second
	defaultReconnectMax = 30 * time.Second
	ackChSize           = 16
	writeWait           = 10 * time.Second
)

// handshakeTimeout bounds waiting for the server's answer to resume; tests
// lower it.
var handshakeTimeout = 10 * time.Second

// ackTimeout bounds waiting for the start_mission acknowledgement; tests
// lower it.
var ackTimeout = 10 * time.Second

var errConnectionClosed = errors.New("websocket connection closed")

// connOptions tunes buffering and recovery. Zero values select the defaults.
type connOptions struct {
	bufferBytes  int
	ackStall     time.Duration
	reconnectMin time.Duration
	reconnectMax time.Duration
	resendRate   int // bytes per second for catch-up traffic; < 0 disables pacing
}

func (o connOptions) withDefaults() connOptions {
	if o.bufferBytes <= 0 {
		o.bufferBytes = defaultBufferBytes
	}
	if o.resendRate == 0 {
		o.resendRate = defaultResendBytesPerSec
	}
	if o.ackStall <= 0 {
		o.ackStall = defaultAckStall
	}
	if o.reconnectMin <= 0 {
		o.reconnectMin = defaultReconnectMin
	}
	if o.reconnectMax < o.reconnectMin {
		o.reconnectMax = max(defaultReconnectMax, o.reconnectMin)
	}
	return o
}

// socket is one WebSocket connection and the goroutines serving it.
type socket struct {
	conn     *ws.Conn
	dead     chan struct{} // closed once the socket is abandoned
	lostOnce sync.Once
}

// connection keeps one mission's stream alive across WebSocket connections.
//
// Envelopes go into an outbound log first. Each socket has a writer that
// sends the log from a cursor. On a resumable connection entries stay in the
// log until the server acknowledges them, and a replacement connection
// resumes the mission and resends whatever the server did not receive. On a
// legacy server an entry is released once written and a replacement
// connection replays start_mission, as before.
type connection struct {
	// startMu orders reconnect handshakes with mission start registration:
	// a handshake sees either no mission or a mission whose start is logged.
	startMu sync.Mutex

	mu           sync.Mutex
	sock         *socket // current socket, nil while disconnected
	closed       bool
	fatal        error // set when the server rejects the credentials
	done         chan struct{}
	reconnecting atomic.Bool

	log   *outboundLog
	ackCh chan streaming.AckMessage

	// Mission state for reconnect handshakes. Guarded by mu.
	startData  []byte // start_mission envelope (seq 1)
	resumeData []byte // resume envelope for this mission
	resumable  bool   // the server acknowledged start with resume support

	acked      atomic.Uint64 // highest seq the server acknowledged
	sent       atomic.Uint64 // highest seq written on the current socket
	resumes    atomic.Uint64 // replacement connections that resumed the mission
	reconnects atomic.Uint64 // replacement connections attached
	restarts   atomic.Uint64 // replacement connections that found the mission lost by the server

	// progressAt (unix nanos) is when the link last made progress: an
	// acknowledgement advanced, or a write went out with nothing outstanding.
	// Whether anything is outstanding is read from sent and acked themselves,
	// so an acknowledgement racing the writer cannot leave a stale state.
	progressAt atomic.Int64

	wsURL  string
	secret string
	opts   connOptions
	logger *slog.Logger
}

func newConnection(logger *slog.Logger, opts connOptions) *connection {
	opts = opts.withDefaults()
	return &connection{
		done:   make(chan struct{}),
		log:    newOutboundLog(opts.bufferBytes),
		ackCh:  make(chan streaming.AckMessage, ackChSize),
		opts:   opts,
		logger: logger,
	}
}

// dial connects to the WebSocket server and starts serving the socket.
func (c *connection) dial(rawURL, secret string) error {
	c.wsURL = rawURL
	c.secret = secret

	conn, err := c.dialOnce()
	if err != nil {
		return err
	}
	return c.attach(conn)
}

// fatalDialError marks a handshake rejection that retrying cannot fix.
type fatalDialError struct{ status int }

func (e *fatalDialError) Error() string {
	return fmt.Sprintf("websocket dial failed: server rejected credentials (HTTP %d)", e.status)
}

// dialOnce performs a single WebSocket dial with the secret query param.
func (c *connection) dialOnce() (*ws.Conn, error) {
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid websocket URL: %w", err)
	}
	q := u.Query()
	q.Set("secret", c.secret)
	u.RawQuery = q.Encode()

	conn, resp, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, &fatalDialError{status: resp.StatusCode}
		}
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	return conn, nil
}

// attach restores the mission on a freshly dialed socket and starts its
// reader, writer and acknowledgement watchdog.
func (c *connection) attach(conn *ws.Conn) error {
	cursor, epoch, err := c.handshake(conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	s := &socket{conn: conn, dead: make(chan struct{})}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return errConnectionClosed
	}
	c.sock = s
	c.mu.Unlock()

	c.sent.Store(cursor)
	c.progressAt.Store(time.Now().UnixNano())
	// Everything recorded so far is a backlog this socket catches up on at
	// the paced rate; what is recorded from now on goes out as it comes.
	backlog := c.log.lastSeq()
	go c.readLoop(s)
	go c.writeLoop(s, cursor, epoch, backlog)
	go c.watchdog(s)
	return nil
}

// handshake brings a new socket up to date with the current mission before
// any queued envelope is written. It returns the writer's starting cursor.
func (c *connection) handshake(conn *ws.Conn) (uint64, uint64, error) {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	c.mu.Lock()
	start, resume, resumable := c.startData, c.resumeData, c.resumable
	c.mu.Unlock()
	epoch := c.log.currentEpoch()
	if start == nil {
		return 0, epoch, nil // no mission yet: the writer sends from the beginning
	}

	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return 0, 0, err
	}
	defer conn.SetReadDeadline(time.Time{})

	if !resumable {
		// Legacy server, or start_mission not yet acknowledged: replay the
		// start; its logged copy (seq 1) is skipped by the writer.
		if err := writeData(conn, start); err != nil {
			return 0, 0, err
		}
		return 1, epoch, nil
	}

	if err := writeData(conn, resume); err != nil {
		return 0, 0, err
	}
	reply, err := readServerMessage(conn, func(m streaming.AckMessage) bool {
		return m.Type == streaming.ServerResumeOK || m.Type == streaming.ServerResumeUnknown || m.Type == streaming.ServerResumeCompleted
	})
	if err != nil {
		return 0, 0, fmt.Errorf("resume: %w", err)
	}

	switch reply.Type {
	case streaming.ServerResumeOK:
		c.logger.Info("WebSocket mission resumed", "serverSeq", reply.Seq, "localSeq", c.log.lastSeq())
		c.resumes.Add(1)
		c.ackUpTo(reply.Seq)
		return reply.Seq, epoch, nil

	case streaming.ServerResumeCompleted:
		// The server saved or discarded the mission but its end_mission or
		// discard_mission ack was lost. It stands in for whichever one is
		// awaited; waitAck skips the other.
		c.logger.Info("WebSocket mission already finished by server")
		c.ackUpTo(c.log.lastSeq())
		c.deliverAck(streaming.AckMessage{Type: streaming.ServerAck, For: streaming.TypeEndMission})
		c.deliverAck(streaming.AckMessage{Type: streaming.ServerAck, For: streaming.TypeDiscardMission})
		return c.log.lastSeq(), epoch, nil

	default: // resume_unknown
		// The server lost the mission (for example it restarted or the grace
		// period expired). Start it again. With a healthy spool every envelope
		// is resent and the server replaces the partial segments it saved;
		// without one, only the envelopes it never acknowledged follow, and the
		// numbering gap marks the new recording partial.
		c.restarts.Add(1)
		if sp := c.log.currentSpool(); sp != nil && sp.healthy() {
			c.logger.Warn("WebSocket server lost the mission, resending it in full from the spool")
		} else {
			c.logger.Warn("WebSocket server lost the mission, starting a new segment")
		}
		if err := writeData(conn, start); err != nil {
			return 0, 0, err
		}
		if _, err := readServerMessage(conn, func(m streaming.AckMessage) bool {
			return m.Type == streaming.ServerAck && m.For == streaming.TypeStartMission
		}); err != nil {
			return 0, 0, fmt.Errorf("restart mission: %w", err)
		}
		// Acknowledgements now count from the new session's start.
		c.acked.Store(1)
		return 1, epoch, nil
	}
}

func writeData(conn *ws.Conn, data []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteMessage(ws.TextMessage, data)
}

// readServerMessage reads until a server message matches. Acknowledgements
// read on the way are applied.
func readServerMessage(conn *ws.Conn, match func(streaming.AckMessage) bool) (streaming.AckMessage, error) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return streaming.AckMessage{}, err
		}
		var m streaming.AckMessage
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if match(m) {
			return m, nil
		}
	}
}

// writeLoop sends the outbound log from cursor until the socket is lost.
//
// Envelopes up to backlog, recorded before the socket was attached, are
// paced to opts.resendRate.
func (c *connection) writeLoop(s *socket, cursor, epoch, backlog uint64) {
	pace := newPacer(c.opts.resendRate)
	for {
		entries, current, changed := c.log.after(cursor)
		if current != epoch {
			// A new mission reset the log; its numbering starts again and
			// none of it is a backlog.
			epoch, cursor, backlog = current, 0, 0
			continue
		}
		if len(entries) == 0 {
			select {
			case <-changed:
				continue
			case <-s.dead:
				return
			case <-c.done:
				return
			}
		}
		for _, e := range entries {
			select {
			case <-s.dead:
				return
			case <-c.done:
				return
			default:
			}
			if e.seq <= backlog && !pace.wait(len(e.data), c.done, s.dead) {
				return
			}
			if err := writeData(s.conn, e.data); err != nil {
				c.logger.Warn("WebSocket write error", "error", err)
				c.lost(s)
				return
			}
			cursor = e.seq
			c.noteWritten(e.seq)
			if !c.isResumable() {
				c.log.ack(e.seq) // a legacy server has no acknowledgement to wait for
			}
		}
	}
}

// readLoop applies server acknowledgements until the socket is lost.
func (c *connection) readLoop(s *socket) {
	for {
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case <-c.done:
			case <-s.dead:
			default:
				c.logger.Warn("WebSocket read error", "error", err)
			}
			c.lost(s)
			return
		}

		var m streaming.AckMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			c.logger.Debug("Non-ack message received", "raw", string(raw))
			continue
		}
		if m.Type != streaming.ServerAck {
			c.logger.Debug("Unexpected server message", "type", m.Type)
			continue
		}
		if m.Seq > 0 {
			c.ackUpTo(m.Seq)
		}
		if m.For != "" {
			c.deliverAck(m)
		}
	}
}

// watchdog abandons a resumable socket whose written envelopes go
// unacknowledged for too long: a half-open TCP connection accepts writes
// that never arrive.
func (c *connection) watchdog(s *socket) {
	interval := min(c.opts.ackStall/4, time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.dead:
			return
		case <-c.done:
			return
		case now := <-ticker.C:
			if c.isResumable() && c.stalled(now) {
				c.logger.Warn("WebSocket acknowledgements stalled, reconnecting",
					"sent", c.sent.Load(), "acked", c.acked.Load(), "stall", c.opts.ackStall)
				c.lost(s)
				return
			}
		}
	}
}

// noteWritten records that envelope seq went out. A write into a link with
// nothing outstanding starts the stall clock; one behind outstanding
// envelopes does not reset it.
func (c *connection) noteWritten(seq uint64) {
	if c.acked.Load() >= c.sent.Load() {
		c.progressAt.Store(time.Now().UnixNano())
	}
	c.sent.Store(seq)
}

// stalled reports whether written envelopes have waited longer than the
// stall timeout without the server acknowledging anything.
func (c *connection) stalled(now time.Time) bool {
	if c.sent.Load() <= c.acked.Load() {
		return false // nothing outstanding, however the writer and reader interleaved
	}
	return now.Sub(time.Unix(0, c.progressAt.Load())) > c.opts.ackStall
}

// ackUpTo releases every envelope the server holds.
func (c *connection) ackUpTo(seq uint64) {
	for {
		old := c.acked.Load()
		if seq <= old {
			return
		}
		if c.acked.CompareAndSwap(old, seq) {
			break
		}
	}
	c.log.ack(seq)
	c.progressAt.Store(time.Now().UnixNano()) // progress restarts the stall clock
}

func (c *connection) deliverAck(m streaming.AckMessage) {
	select {
	case c.ackCh <- m:
	default:
		c.logger.Debug("Ack channel full, dropping", "for", m.For)
	}
}

func (c *connection) isResumable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resumable
}

// lost abandons a socket and starts reconnecting. Safe to call repeatedly.
func (c *connection) lost(s *socket) {
	s.lostOnce.Do(func() {
		close(s.dead)
		_ = s.conn.Close()
		c.mu.Lock()
		if c.sock == s {
			c.sock = nil
		}
		closed := c.closed
		c.mu.Unlock()
		if !closed {
			go c.reconnect()
		}
	})
}

// reconnect dials until a replacement socket is attached, the connection is
// closed, or the server rejects the credentials. It does not give up on
// transient failures: the outbound log keeps recording meanwhile.
func (c *connection) reconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // another reconnection is already in progress
	}
	for {
		c.reconnectOnce()
		c.reconnecting.Store(false)
		// A socket lost while we were attaching could not start its own
		// reconnect; pick it up here.
		c.mu.Lock()
		again := c.sock == nil && !c.closed && c.fatal == nil
		c.mu.Unlock()
		if !again || !c.reconnecting.CompareAndSwap(false, true) {
			return
		}
	}
}

func (c *connection) reconnectOnce() {
	backoff := c.opts.reconnectMin
	for attempt := 1; ; attempt++ {
		wait := jitter(backoff)
		c.logger.Info("Reconnecting to WebSocket", "attempt", attempt, "backoff", wait)
		select {
		case <-time.After(wait):
		case <-c.done:
			return
		}

		conn, err := c.dialOnce()
		if err != nil {
			var fatal *fatalDialError
			if errors.As(err, &fatal) {
				c.logger.Error("WebSocket reconnect stopped", "error", err)
				c.mu.Lock()
				c.fatal = err
				c.mu.Unlock()
				return
			}
			c.logger.Warn("Reconnect dial failed", "attempt", attempt, "error", err)
			backoff = min(backoff*2, c.opts.reconnectMax)
			continue
		}
		if err := c.attach(conn); err != nil {
			if errors.Is(err, errConnectionClosed) {
				return
			}
			c.logger.Warn("Failed to restore WebSocket mission", "attempt", attempt, "error", err)
			backoff = min(backoff*2, c.opts.reconnectMax)
			continue
		}
		_, bytes, _ := c.log.stats()
		c.reconnects.Add(1)
		c.logger.Info("WebSocket reconnected", "attempt", attempt, "bufferedBytes", bytes)
		return
	}
}

// jitter spreads reconnects of several game servers by ±20%.
func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

// beginMission resets the log for a new mission and logs its start_mission
// envelope as seq 1. resume is the envelope a replacement connection sends;
// sp, when not nil, keeps the mission on disk.
func (c *connection) beginMission(build func(seq uint64) ([]byte, error), resume []byte, sp *spool) error {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	c.log.reset(sp)
	c.acked.Store(0)
	c.sent.Store(0)
	c.progressAt.Store(time.Now().UnixNano())
	var start []byte
	if _, err := c.log.append(func(seq uint64) ([]byte, error) {
		data, err := build(seq)
		start = data
		return data, err
	}); err != nil {
		return err
	}
	c.mu.Lock()
	c.startData = start
	c.resumeData = resume
	c.resumable = false
	c.mu.Unlock()
	return nil
}

// setResumable records whether the server acknowledged resume support.
func (c *connection) setResumable(v bool) {
	c.mu.Lock()
	c.resumable = v
	c.mu.Unlock()
}

// endMission forgets the mission so later sockets do not replay it.
//
// It also drops whatever the log still holds. An unconfirmed mission is then
// resent only by spool recovery, never also by this connection's writer
// after a reconnect, which would replay it without a handshake.
func (c *connection) endMission() {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	c.mu.Lock()
	c.startData = nil
	c.resumeData = nil
	c.resumable = false
	c.mu.Unlock()
	c.log.reset(nil)
	c.progressAt.Store(time.Now().UnixNano())
}

// send logs one envelope; build receives its sequence number.
func (c *connection) send(build func(seq uint64) ([]byte, error)) error {
	select {
	case <-c.done:
		return errConnectionClosed
	default:
	}
	_, err := c.log.append(build)
	return err
}

// drainAcks discards acknowledgements no one waited for.
func (c *connection) drainAcks() {
	for {
		select {
		case <-c.ackCh:
		default:
			return
		}
	}
}

// waitAck blocks until the server acknowledges ackFor, the timeout expires,
// or the connection is closed. Reconnects in between do not interrupt it.
func (c *connection) waitAck(ackFor string, timeout time.Duration) (streaming.AckMessage, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ack := <-c.ackCh:
			if ack.For == ackFor {
				return ack, nil
			}
		case <-timer.C:
			return streaming.AckMessage{}, fmt.Errorf("timeout waiting for ack of %q", ackFor)
		case <-c.done:
			return streaming.AckMessage{}, fmt.Errorf("connection closed while waiting for ack of %q", ackFor)
		}
	}
}

// close shuts down all goroutines and closes the underlying socket.
func (c *connection) close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	s := c.sock
	c.sock = nil
	c.mu.Unlock()

	if s != nil {
		s.lostOnce.Do(func() { close(s.dead) })
		// The socket may already be closed by a loss being handled.
		if err := s.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}
	return nil
}
