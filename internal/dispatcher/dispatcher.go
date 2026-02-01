package dispatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Event represents an incoming command from ArmA.
type Event struct {
	Command   string
	Args      []string
	Timestamp time.Time
}

// HandlerFunc processes an event and returns a result.
type HandlerFunc func(Event) (any, error)

// Logger interface for pluggable logging.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// Option configures handler registration.
type Option func(*config)

type config struct {
	bufferSize int
	blocking   bool
	logged     bool
}

// Buffered makes the handler async with a queue of the given size.
func Buffered(size int) Option {
	return func(c *config) {
		c.bufferSize = size
	}
}

// Blocking makes a buffered handler block when the queue is full instead of dropping.
func Blocking() Option {
	return func(c *config) {
		c.blocking = true
	}
}

// Logged adds debug logging to the handler.
func Logged() Option {
	return func(c *config) {
		c.logged = true
	}
}

// Dispatcher routes events to registered handlers.
type Dispatcher struct {
	handlers map[string]HandlerFunc
	logger   Logger

	// OTEL metrics
	queueSize metric.Int64ObservableGauge
	processed metric.Int64Counter
	dropped   metric.Int64Counter

	// Track buffers for gauge callback
	mu      sync.RWMutex
	buffers map[string]chan Event
}

// New creates a new Dispatcher with the given logger.
// Uses the global OTel meter for metrics (no-op if not configured).
func New(logger Logger) (*Dispatcher, error) {
	d := &Dispatcher{
		handlers: make(map[string]HandlerFunc),
		buffers:  make(map[string]chan Event),
		logger:   logger,
	}

	// Get meter from global OTel provider (returns no-op if not configured)
	m := meter()

	var err error

	d.queueSize, err = m.Int64ObservableGauge(
		"dispatcher.queue.size",
		metric.WithDescription("Current number of events in queue"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating queue size gauge: %w", err)
	}

	_, err = m.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			d.mu.RLock()
			defer d.mu.RUnlock()
			for cmd, buf := range d.buffers {
				o.ObserveInt64(d.queueSize, int64(len(buf)),
					metric.WithAttributes(attribute.String("command", cmd)))
			}
			return nil
		},
		d.queueSize,
	)
	if err != nil {
		return nil, fmt.Errorf("registering queue callback: %w", err)
	}

	d.processed, err = m.Int64Counter(
		"dispatcher.events.processed",
		metric.WithDescription("Total events processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating processed counter: %w", err)
	}

	d.dropped, err = m.Int64Counter(
		"dispatcher.events.dropped",
		metric.WithDescription("Total events dropped due to full queue"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating dropped counter: %w", err)
	}

	return d, nil
}

// Register adds a handler for the given command with optional configuration.
func (d *Dispatcher) Register(command string, h HandlerFunc, opts ...Option) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	handler := h

	if cfg.bufferSize > 0 {
		handler = d.withBuffer(command, cfg.bufferSize, cfg.blocking, handler)
	}

	if cfg.logged {
		handler = d.withLogging(command, handler)
	}

	d.handlers[command] = handler
}

// Dispatch routes an event to its registered handler.
func (d *Dispatcher) Dispatch(e Event) (any, error) {
	h, ok := d.handlers[e.Command]
	if !ok {
		return nil, fmt.Errorf("unknown command: %s", e.Command)
	}
	return h(e)
}

// HasHandler returns true if a handler is registered for the command.
func (d *Dispatcher) HasHandler(command string) bool {
	_, ok := d.handlers[command]
	return ok
}

func (d *Dispatcher) withBuffer(command string, size int, blocking bool, h HandlerFunc) HandlerFunc {
	buffer := make(chan Event, size)

	d.mu.Lock()
	d.buffers[command] = buffer
	d.mu.Unlock()

	cmdAttr := attribute.String("command", command)

	go func() {
		for e := range buffer {
			h(e)
			d.processed.Add(context.Background(), 1, metric.WithAttributes(cmdAttr))
		}
	}()

	if blocking {
		return func(e Event) (any, error) {
			buffer <- e
			return "queued", nil
		}
	}

	return func(e Event) (any, error) {
		select {
		case buffer <- e:
			return "queued", nil
		default:
			d.dropped.Add(context.Background(), 1, metric.WithAttributes(cmdAttr))
			return nil, fmt.Errorf("queue full: %s", command)
		}
	}
}

func (d *Dispatcher) withLogging(command string, h HandlerFunc) HandlerFunc {
	return func(e Event) (any, error) {
		start := time.Now()
		d.logger.Debug("handling event", "command", command, "args", len(e.Args))

		result, err := h(e)

		if err != nil {
			d.logger.Error("event failed", "command", command, "duration", time.Since(start), "error", err)
		} else {
			d.logger.Debug("event complete", "command", command, "duration", time.Since(start))
		}

		return result, err
	}
}
