package dispatcher

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "failed to create dispatcher")

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

	assert.NoError(t, err)
	assert.True(t, called, "handler was not called")
	assert.Equal(t, "result", result)
}

func TestDispatcher_UnknownCommand(t *testing.T) {
	d, _ := newTestDispatcher(t)

	_, err := d.Dispatch(Event{Command: ":UNKNOWN:"})

	assert.Error(t, err)
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
		assert.NoError(t, err)
		assert.Equal(t, "queued", result)
	}

	// Wait for processing
	wg.Wait()

	assert.Equal(t, int32(3), processed.Load())
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

	assert.Error(t, err)

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

	assert.GreaterOrEqual(t, len(logger.messages), 2)
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

	assert.True(t, hasError, "expected error log message")
}

func TestDispatcher_HasHandler(t *testing.T) {
	d, _ := newTestDispatcher(t)

	d.Register(":EXISTS:", func(e Event) (any, error) { return nil, nil })

	assert.True(t, d.HasHandler(":EXISTS:"), "expected handler to exist")
	assert.False(t, d.HasHandler(":NOT_EXISTS:"), "expected handler to not exist")
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

	assert.NoError(t, err)
	assert.Equal(t, "queued", result)

	wg.Wait()

	assert.Equal(t, int32(1), processed.Load())

	logger.mu.Lock()
	defer logger.mu.Unlock()

	assert.GreaterOrEqual(t, len(logger.messages), 2)
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
		assert.NoError(t, err)
		assert.Equal(t, "queued", result)
	}

	// Verify nothing processed yet
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), processed.Load())

	// Open the gate
	close(ready)

	// Wait for processing
	wg.Wait()

	assert.Equal(t, int32(3), processed.Load())
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

	require.Len(t, order, 3)
	assert.Equal(t, []string{"first", "second", "third"}, order)
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

	assert.Equal(t, int32(2), processed.Load())
}

func TestDispatcher_BufferedPanicRecovery(t *testing.T) {
	d, logger := newTestDispatcher(t)

	var wg sync.WaitGroup
	wg.Add(3)

	callCount := atomic.Int32{}

	d.Register(":PANIC:", func(e Event) (any, error) {
		defer wg.Done()
		n := callCount.Add(1)
		if n == 2 {
			panic("test panic in handler")
		}
		return nil, nil
	}, Buffered(10))

	// Dispatch 3 events: #1 ok, #2 panics, #3 should still be processed
	for i := 0; i < 3; i++ {
		_, err := d.Dispatch(Event{Command: ":PANIC:"})
		assert.NoError(t, err)
	}

	wg.Wait()

	// All 3 events were processed (goroutine survived the panic)
	assert.Equal(t, int32(3), callCount.Load())

	// Panic was logged
	logger.mu.Lock()
	defer logger.mu.Unlock()
	hasPanicLog := false
	for _, msg := range logger.messages {
		if strings.HasPrefix(msg, "ERROR") && strings.Contains(msg, "panic") {
			hasPanicLog = true
			break
		}
	}
	assert.True(t, hasPanicLog, "expected panic recovery to be logged")
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

	assert.True(t, found, "expected INFO log when gate opens")
}
