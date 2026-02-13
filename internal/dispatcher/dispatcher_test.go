package dispatcher

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testLogger implements Logger for testing
type testLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *testLogger) Debug(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprintf("DEBUG: %s %v", msg, keysAndValues))
}

func (l *testLogger) Info(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprintf("INFO: %s %v", msg, keysAndValues))
}

func (l *testLogger) Error(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, fmt.Sprintf("ERROR: %s %v", msg, keysAndValues))
}

func newTestDispatcher(t *testing.T) (*Dispatcher, *testLogger) {
	logger := &testLogger{}

	d, err := New(logger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}

	return d, logger
}

func TestDispatcher_SyncHandler(t *testing.T) {
	d, _ := newTestDispatcher(t)

	called := false
	d.Register(":TEST:", func(e Event) (any, error) {
		called = true
		return "result", nil
	})

	result, err := d.Dispatch(Event{Command: ":TEST:", Args: []string{"arg1"}})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result != "result" {
		t.Errorf("expected 'result', got %v", result)
	}
}

func TestDispatcher_UnknownCommand(t *testing.T) {
	d, _ := newTestDispatcher(t)

	_, err := d.Dispatch(Event{Command: ":UNKNOWN:"})

	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestDispatcher_BufferedHandler(t *testing.T) {
	d, _ := newTestDispatcher(t)

	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	d.Register(":BUFFERED:", func(e Event) (any, error) {
		processed.Add(1)
		wg.Done()
		return nil, nil
	}, Buffered(100))

	// Dispatch 3 events
	for i := 0; i < 3; i++ {
		result, err := d.Dispatch(Event{Command: ":BUFFERED:"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "queued" {
			t.Errorf("expected 'queued', got %v", result)
		}
	}

	// Wait for processing
	wg.Wait()

	if processed.Load() != 3 {
		t.Errorf("expected 3 processed, got %d", processed.Load())
	}
}

func TestDispatcher_BufferedDropsWhenFull(t *testing.T) {
	d, _ := newTestDispatcher(t)

	// Block the handler so queue fills up
	block := make(chan struct{})
	d.Register(":FULL:", func(e Event) (any, error) {
		<-block
		return nil, nil
	}, Buffered(2))

	// Fill the queue (2 items) + 1 being processed
	d.Dispatch(Event{Command: ":FULL:"}) // being processed
	d.Dispatch(Event{Command: ":FULL:"}) // queued
	d.Dispatch(Event{Command: ":FULL:"}) // queued

	// This should be dropped
	_, err := d.Dispatch(Event{Command: ":FULL:"})

	if err == nil {
		t.Error("expected error when queue is full")
	}

	close(block)
}

func TestDispatcher_BufferedBlocking(t *testing.T) {
	d, _ := newTestDispatcher(t)

	block := make(chan struct{})
	d.Register(":BLOCKING:", func(e Event) (any, error) {
		<-block
		return nil, nil
	}, Buffered(1), Blocking())

	// First event starts processing
	d.Dispatch(Event{Command: ":BLOCKING:"})
	// Second event fills the queue
	d.Dispatch(Event{Command: ":BLOCKING:"})

	// Third event should block (test with timeout)
	done := make(chan struct{})
	go func() {
		d.Dispatch(Event{Command: ":BLOCKING:"})
		close(done)
	}()

	select {
	case <-done:
		t.Error("dispatch should have blocked")
	case <-time.After(50 * time.Millisecond):
		// Expected - dispatch is blocking
	}

	close(block)
}

func TestDispatcher_LoggedHandler(t *testing.T) {
	d, logger := newTestDispatcher(t)

	d.Register(":LOGGED:", func(e Event) (any, error) {
		return nil, nil
	}, Logged())

	d.Dispatch(Event{Command: ":LOGGED:", Args: []string{"a", "b"}})

	// Give time for logging
	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	defer logger.mu.Unlock()

	if len(logger.messages) < 2 {
		t.Errorf("expected at least 2 log messages, got %d", len(logger.messages))
	}
}

func TestDispatcher_LoggedHandlerError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	d.Register(":ERROR:", func(e Event) (any, error) {
		return nil, fmt.Errorf("test error")
	}, Logged())

	d.Dispatch(Event{Command: ":ERROR:"})

	logger.mu.Lock()
	defer logger.mu.Unlock()

	hasError := false
	for _, msg := range logger.messages {
		if len(msg) >= 5 && msg[:5] == "ERROR" {
			hasError = true
			break
		}
	}

	if !hasError {
		t.Error("expected error log message")
	}
}

func TestDispatcher_HasHandler(t *testing.T) {
	d, _ := newTestDispatcher(t)

	d.Register(":EXISTS:", func(e Event) (any, error) { return nil, nil })

	if !d.HasHandler(":EXISTS:") {
		t.Error("expected handler to exist")
	}

	if d.HasHandler(":NOT_EXISTS:") {
		t.Error("expected handler to not exist")
	}
}

func TestDispatcher_CombinedOptions(t *testing.T) {
	d, logger := newTestDispatcher(t)

	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)

	d.Register(":COMBINED:", func(e Event) (any, error) {
		processed.Add(1)
		wg.Done()
		return "done", nil
	}, Buffered(100), Logged())

	result, err := d.Dispatch(Event{Command: ":COMBINED:"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "queued" {
		t.Errorf("expected 'queued', got %v", result)
	}

	wg.Wait()

	if processed.Load() != 1 {
		t.Errorf("expected 1 processed, got %d", processed.Load())
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()

	if len(logger.messages) < 2 {
		t.Errorf("expected log messages, got %d", len(logger.messages))
	}
}

func TestDispatcher_GatedHandler(t *testing.T) {
	d, _ := newTestDispatcher(t)

	ready := make(chan struct{})
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	d.Register(":GATED:", func(e Event) (any, error) {
		processed.Add(1)
		wg.Done()
		return nil, nil
	}, Buffered(10), Gated(ready))

	// Dispatch events before gate opens
	for i := 0; i < 3; i++ {
		result, err := d.Dispatch(Event{Command: ":GATED:"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "queued" {
			t.Errorf("expected 'queued', got %v", result)
		}
	}

	// Verify nothing processed yet
	time.Sleep(50 * time.Millisecond)
	if processed.Load() != 0 {
		t.Errorf("expected 0 processed before gate opens, got %d", processed.Load())
	}

	// Open the gate
	close(ready)

	// Wait for processing
	wg.Wait()

	if processed.Load() != 3 {
		t.Errorf("expected 3 processed after gate opens, got %d", processed.Load())
	}
}

func TestDispatcher_GatedHandlerProcessesInOrder(t *testing.T) {
	d, _ := newTestDispatcher(t)

	ready := make(chan struct{})
	var order []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(3)

	d.Register(":ORDERED:", func(e Event) (any, error) {
		mu.Lock()
		order = append(order, e.Args[0])
		mu.Unlock()
		wg.Done()
		return nil, nil
	}, Buffered(10), Gated(ready))

	// Dispatch in order
	d.Dispatch(Event{Command: ":ORDERED:", Args: []string{"first"}})
	d.Dispatch(Event{Command: ":ORDERED:", Args: []string{"second"}})
	d.Dispatch(Event{Command: ":ORDERED:", Args: []string{"third"}})

	// Open gate
	close(ready)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 items, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Errorf("expected [first, second, third], got %v", order)
	}
}

func TestDispatcher_GatedWithBlocking(t *testing.T) {
	d, _ := newTestDispatcher(t)

	ready := make(chan struct{})
	var processed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	d.Register(":GATED_BLOCKING:", func(e Event) (any, error) {
		processed.Add(1)
		wg.Done()
		return nil, nil
	}, Buffered(1), Blocking(), Gated(ready))

	// First event fills buffer
	d.Dispatch(Event{Command: ":GATED_BLOCKING:"})

	// Second event should block since buffer is full and gate closed
	dispatched := make(chan struct{})
	go func() {
		d.Dispatch(Event{Command: ":GATED_BLOCKING:"})
		close(dispatched)
	}()

	// Verify dispatch is blocking
	select {
	case <-dispatched:
		t.Error("dispatch should block when buffer full")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	// Open gate - should unblock and process
	close(ready)
	wg.Wait()

	if processed.Load() != 2 {
		t.Errorf("expected 2 processed, got %d", processed.Load())
	}
}

func TestDispatcher_ConcurrentRegisterAndDispatch(t *testing.T) {
	d, _ := newTestDispatcher(t)

	// Pre-register a handler so Dispatch/HasHandler have something to hit
	d.Register(":EXISTING:", func(e Event) (any, error) {
		return "ok", nil
	})

	var wg sync.WaitGroup

	// Goroutine 1: continuously register new handlers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cmd := fmt.Sprintf(":REG_%d:", i)
			d.Register(cmd, func(e Event) (any, error) {
				return nil, nil
			}, Buffered(10))
		}
	}()

	// Goroutine 2: continuously check HasHandler and Dispatch
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			d.HasHandler(":EXISTING:")
			d.Dispatch(Event{Command: ":EXISTING:"})
		}
	}()

	wg.Wait()
}

func TestDispatcher_GatedLogsWhenOpened(t *testing.T) {
	d, logger := newTestDispatcher(t)

	ready := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	d.Register(":GATED_LOG:", func(e Event) (any, error) {
		wg.Done()
		return nil, nil
	}, Buffered(10), Gated(ready))

	d.Dispatch(Event{Command: ":GATED_LOG:"})
	close(ready)
	wg.Wait()

	// Allow time for log
	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	defer logger.mu.Unlock()

	found := false
	for _, msg := range logger.messages {
		if len(msg) >= 4 && msg[:4] == "INFO" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected INFO log when gate opens")
	}
}
