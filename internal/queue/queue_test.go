package queue

import (
	"sync"
	"testing"
)

// testItem is a simple struct for testing the generic queue
type testItem struct {
	ID   int
	Name string
}

func TestQueue_New(t *testing.T) {
	q := New[testItem]()
	if q == nil {
		t.Fatal("expected non-nil queue")
	}
	if !q.Empty() {
		t.Error("expected empty queue")
	}
	if q.Len() != 0 {
		t.Errorf("expected length 0, got %d", q.Len())
	}
}

func TestQueue_Push(t *testing.T) {
	q := New[testItem]()

	q.Push(testItem{ID: 1, Name: "first"})
	if q.Len() != 1 {
		t.Errorf("expected length 1, got %d", q.Len())
	}

	q.Push(testItem{ID: 2}, testItem{ID: 3})
	if q.Len() != 3 {
		t.Errorf("expected length 3, got %d", q.Len())
	}
}

func TestQueue_Pop(t *testing.T) {
	q := New[testItem]()

	// Pop from empty queue returns zero value
	result := q.Pop()
	if result.ID != 0 || result.Name != "" {
		t.Errorf("expected zero value, got %+v", result)
	}

	// Pop from non-empty queue
	q.Push(testItem{ID: 1, Name: "first"}, testItem{ID: 2, Name: "second"})
	first := q.Pop()
	if first.ID != 1 || first.Name != "first" {
		t.Errorf("expected {1, first}, got %+v", first)
	}
	if q.Len() != 1 {
		t.Errorf("expected length 1, got %d", q.Len())
	}
}

func TestQueue_Empty(t *testing.T) {
	q := New[testItem]()

	if !q.Empty() {
		t.Error("expected empty queue")
	}

	q.Push(testItem{ID: 1})
	if q.Empty() {
		t.Error("expected non-empty queue")
	}

	q.Pop()
	if !q.Empty() {
		t.Error("expected empty queue after pop")
	}
}

func TestQueue_Len(t *testing.T) {
	q := New[testItem]()

	if q.Len() != 0 {
		t.Errorf("expected 0, got %d", q.Len())
	}

	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})
	if q.Len() != 3 {
		t.Errorf("expected 3, got %d", q.Len())
	}
}

func TestQueue_Clear(t *testing.T) {
	q := New[testItem]()
	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})

	q.Clear()

	if !q.Empty() {
		t.Error("expected empty queue after clear")
	}
	if q.Len() != 0 {
		t.Errorf("expected length 0, got %d", q.Len())
	}
}

func TestQueue_GetAndEmpty(t *testing.T) {
	q := New[testItem]()
	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})

	result := q.GetAndEmpty()

	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 2 || result[2].ID != 3 {
		t.Errorf("unexpected items: %+v", result)
	}
	if !q.Empty() {
		t.Error("expected empty queue after GetAndEmpty")
	}
}

func TestQueue_Concurrent(t *testing.T) {
	q := New[testItem]()
	var wg sync.WaitGroup

	// Concurrent pushes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			q.Push(testItem{ID: id})
		}(i)
	}
	wg.Wait()

	if q.Len() != 100 {
		t.Errorf("expected 100 items, got %d", q.Len())
	}

	// Concurrent pops
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Pop()
		}()
	}
	wg.Wait()

	if q.Len() != 50 {
		t.Errorf("expected 50 items after pops, got %d", q.Len())
	}
}

func TestQueue_ConcurrentGetAndEmpty(t *testing.T) {
	q := New[testItem]()

	// Fill queue
	for i := 0; i < 100; i++ {
		q.Push(testItem{ID: i})
	}

	var wg sync.WaitGroup
	results := make(chan []testItem, 10)

	// Concurrent GetAndEmpty calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- q.GetAndEmpty()
		}()
	}
	wg.Wait()
	close(results)

	// Total items across all results should be 100
	total := 0
	for r := range results {
		total += len(r)
	}
	if total != 100 {
		t.Errorf("expected total 100 items, got %d", total)
	}
}

// Test with different types to ensure generics work correctly

func TestQueue_StringType(t *testing.T) {
	q := New[string]()
	q.Push("hello", "world")

	first := q.Pop()
	if first != "hello" {
		t.Errorf("expected 'hello', got '%s'", first)
	}
}

func TestQueue_IntType(t *testing.T) {
	q := New[int]()
	q.Push(1, 2, 3, 4, 5)

	sum := 0
	for !q.Empty() {
		sum += q.Pop()
	}
	if sum != 15 {
		t.Errorf("expected sum 15, got %d", sum)
	}
}

func TestQueue_SliceType(t *testing.T) {
	q := New[[]string]()
	q.Push([]string{"a", "b"}, []string{"c", "d"})

	first := q.Pop()
	if len(first) != 2 || first[0] != "a" {
		t.Errorf("expected [a, b], got %v", first)
	}
}

// Test SoldierStatesMap

func TestSoldierStatesMap_New(t *testing.T) {
	m := NewSoldierStatesMap()
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if m.Len() != 0 {
		t.Errorf("expected empty map, got length %d", m.Len())
	}
}

func TestSoldierStatesMap_SetAndLen(t *testing.T) {
	m := NewSoldierStatesMap()

	m.Set(0, []any{"state0"})
	m.Set(10, []any{"state10"})
	m.Set(20, []any{"state20"})

	if m.Len() != 3 {
		t.Errorf("expected length 3, got %d", m.Len())
	}
}

func TestSoldierStatesMap_GetStateAtFrame(t *testing.T) {
	m := NewSoldierStatesMap()
	m.Set(0, []any{"state0"})
	m.Set(10, []any{"state10"})
	m.Set(20, []any{"state20"})

	// Exact frame match
	state, err := m.GetStateAtFrame(10, 100)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(state) != 1 || state[0] != "state10" {
		t.Errorf("expected [state10], got %v", state)
	}

	// No exact match, should find next frame
	state, err = m.GetStateAtFrame(5, 100)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(state) != 1 || state[0] != "state10" {
		t.Errorf("expected [state10], got %v", state)
	}

	// Frame not found
	_, err = m.GetStateAtFrame(50, 60)
	if err == nil {
		t.Error("expected error for missing frame")
	}
}

func TestSoldierStatesMap_GetLastState(t *testing.T) {
	m := NewSoldierStatesMap()
	m.Set(10, []any{"state10"})

	// Initially nil
	if m.GetLastState() != nil {
		t.Error("expected nil last state initially")
	}

	// After GetStateAtFrame with fallback search
	m.GetStateAtFrame(5, 100)
	lastState := m.GetLastState()
	if len(lastState) != 1 || lastState[0] != "state10" {
		t.Errorf("expected [state10], got %v", lastState)
	}
}
