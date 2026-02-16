package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
)

const (
	sendChSize   = 10_000
	ackChSize    = 16
	maxReconnect = 10
	maxBackoff   = 30 * time.Second
	writeWait    = 10 * time.Second
	ackTimeout   = 10 * time.Second
)

// connection manages a WebSocket connection with a single write goroutine.
type connection struct {
	mu     sync.Mutex
	conn   *ws.Conn
	sendCh chan []byte
	ackCh  chan AckMessage
	done   chan struct{} // closed on shutdown
	closed bool

	wsURL  string
	secret string

	// Cached start_mission message for reconnect replay.
	cachedStartMsg []byte

	logger *slog.Logger
}

func newConnection(logger *slog.Logger) *connection {
	return &connection{
		sendCh: make(chan []byte, sendChSize),
		ackCh:  make(chan AckMessage, ackChSize),
		done:   make(chan struct{}),
		logger: logger,
	}
}

// dial connects to the WebSocket server and starts read/write loops.
func (c *connection) dial(rawURL, secret string) error {
	c.wsURL = rawURL
	c.secret = secret

	conn, err := c.dialOnce()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	go c.writeLoop()
	go c.readLoop()

	return nil
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

	conn, _, err := ws.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	return conn, nil
}

// writeLoop drains sendCh and writes messages to the WebSocket.
// Only one writeLoop runs at a time; it returns on error or shutdown.
func (c *connection) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case data := <-c.sendCh:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				continue
			}

			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("WebSocket SetWriteDeadline error", "error", err)
				go c.reconnect()
				return
			}
			if err := conn.WriteMessage(ws.TextMessage, data); err != nil {
				c.logger.Warn("WebSocket write error", "error", err)
				go c.reconnect()
				return
			}
		}
	}
}

// readLoop reads ack messages from the server and routes them to ackCh.
func (c *connection) readLoop() {
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			c.logger.Warn("WebSocket read error", "error", err)
			go c.reconnect()
			return
		}

		var ack AckMessage
		if err := json.Unmarshal(message, &ack); err != nil {
			c.logger.Debug("Non-ack message received", "raw", string(message))
			continue
		}

		if ack.Type == "ack" {
			select {
			case c.ackCh <- ack:
			default:
				c.logger.Debug("Ack channel full, dropping", "for", ack.For)
			}
		}
	}
}

// reconnect attempts to re-establish the WebSocket connection with
// exponential backoff. On success it replays the cached start_mission
// message and restarts the read/write loops.
func (c *connection) reconnect() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	backoff := time.Second
	for attempt := 1; attempt <= maxReconnect; attempt++ {
		select {
		case <-c.done:
			return
		default:
		}

		c.logger.Info("Reconnecting to WebSocket", "attempt", attempt, "backoff", backoff)
		time.Sleep(backoff)

		conn, err := c.dialOnce()
		if err != nil {
			c.logger.Warn("Reconnect dial failed", "attempt", attempt, "error", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		c.mu.Lock()
		c.conn = conn
		cached := c.cachedStartMsg
		c.mu.Unlock()

		// Replay start_mission so the server knows which mission we're recording.
		if cached != nil {
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("Failed to set deadline for start_mission replay", "error", err)
				_ = conn.Close()
				continue
			}
			if err := conn.WriteMessage(ws.TextMessage, cached); err != nil {
				c.logger.Warn("Failed to replay start_mission after reconnect", "error", err)
				_ = conn.Close()
				continue
			}
		}

		c.logger.Info("WebSocket reconnected", "attempt", attempt)
		go c.writeLoop()
		go c.readLoop()
		return
	}

	c.logger.Error("WebSocket reconnect failed after max attempts", "maxAttempts", maxReconnect)
}

// send pushes data to the write loop. Non-blocking; drops if channel full.
func (c *connection) send(data []byte) {
	select {
	case c.sendCh <- data:
	default:
		c.logger.Warn("WebSocket send channel full, dropping message")
	}
}

// sendAndWait sends data and blocks until the server acknowledges with a
// matching ack message or the timeout expires.
func (c *connection) sendAndWait(data []byte, ackFor string, timeout time.Duration) error {
	c.send(data)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case ack := <-c.ackCh:
			if ack.For == ackFor {
				return nil
			}
			// Not our ack, keep waiting.
		case <-timer.C:
			return fmt.Errorf("timeout waiting for ack of %q", ackFor)
		case <-c.done:
			return fmt.Errorf("connection closed while waiting for ack of %q", ackFor)
		}
	}
}

// close sends a WebSocket close frame and shuts down all goroutines.
func (c *connection) close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		_ = conn.WriteMessage(
			ws.CloseMessage,
			ws.FormatCloseMessage(ws.CloseNormalClosure, ""),
		)
		return conn.Close()
	}
	return nil
}
