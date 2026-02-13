package queue

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testItem is a simple struct for testing the generic queue
type testItem struct {
	ID   int
	Name string
}

func TestQueue_New(t *testing.T) {
	q := New[testItem]()
	require.NotNil(t, q)
	assert.True(t, q.Empty(), "expected empty queue")
	assert.Equal(t, 0, q.Len())
}

func TestQueue_Push(t *testing.T) {
	q := New[testItem]()

	q.Push(testItem{ID: 1, Name: "first"})
	assert.Equal(t, 1, q.Len())

	q.Push(testItem{ID: 2}, testItem{ID: 3})
	assert.Equal(t, 3, q.Len())
}

func TestQueue_Pop(t *testing.T) {
	q := New[testItem]()

	// Pop from empty queue returns zero value
	result := q.Pop()
	assert.Equal(t, 0, result.ID)
	assert.Equal(t, "", result.Name)

	// Pop from non-empty queue
	q.Push(testItem{ID: 1, Name: "first"}, testItem{ID: 2, Name: "second"})
	first := q.Pop()
	assert.Equal(t, 1, first.ID)
	assert.Equal(t, "first", first.Name)
	assert.Equal(t, 1, q.Len())
}

func TestQueue_Empty(t *testing.T) {
	q := New[testItem]()

	assert.True(t, q.Empty(), "expected empty queue")

	q.Push(testItem{ID: 1})
	assert.False(t, q.Empty(), "expected non-empty queue")

	q.Pop()
	assert.True(t, q.Empty(), "expected empty queue after pop")
}

func TestQueue_Len(t *testing.T) {
	q := New[testItem]()

	assert.Equal(t, 0, q.Len())

	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})
	assert.Equal(t, 3, q.Len())
}

func TestQueue_Clear(t *testing.T) {
	q := New[testItem]()
	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})

	q.Clear()

	assert.True(t, q.Empty(), "expected empty queue after clear")
	assert.Equal(t, 0, q.Len())
}

func TestQueue_GetAndEmpty(t *testing.T) {
	q := New[testItem]()
	q.Push(testItem{ID: 1}, testItem{ID: 2}, testItem{ID: 3})

	result := q.GetAndEmpty()

	require.Len(t, result, 3)
	assert.Equal(t, 1, result[0].ID)
	assert.Equal(t, 2, result[1].ID)
	assert.Equal(t, 3, result[2].ID)
	assert.True(t, q.Empty(), "expected empty queue after GetAndEmpty")
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

	assert.Equal(t, 100, q.Len())

	// Concurrent pops
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Pop()
		}()
	}
	wg.Wait()

	assert.Equal(t, 50, q.Len())
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
	assert.Equal(t, 100, total)
}

// Test with different types to ensure generics work correctly

func TestQueue_StringType(t *testing.T) {
	q := New[string]()
	q.Push("hello", "world")

	first := q.Pop()
	assert.Equal(t, "hello", first)
}

func TestQueue_IntType(t *testing.T) {
	q := New[int]()
	q.Push(1, 2, 3, 4, 5)

	sum := 0
	for !q.Empty() {
		sum += q.Pop()
	}
	assert.Equal(t, 15, sum)
}

func TestQueue_SliceType(t *testing.T) {
	q := New[[]string]()
	q.Push([]string{"a", "b"}, []string{"c", "d"})

	first := q.Pop()
	require.Len(t, first, 2)
	assert.Equal(t, "a", first[0])
}

// Test SoldierStatesMap

func TestSoldierStatesMap_New(t *testing.T) {
	m := NewSoldierStatesMap()
	require.NotNil(t, m)
	assert.Equal(t, 0, m.Len())
}

func TestSoldierStatesMap_SetAndLen(t *testing.T) {
	m := NewSoldierStatesMap()

	m.Set(0, []any{"state0"})
	m.Set(10, []any{"state10"})
	m.Set(20, []any{"state20"})

	assert.Equal(t, 3, m.Len())
}

func TestSoldierStatesMap_GetStateAtFrame(t *testing.T) {
	m := NewSoldierStatesMap()
	m.Set(0, []any{"state0"})
	m.Set(10, []any{"state10"})
	m.Set(20, []any{"state20"})

	// Exact frame match
	state, err := m.GetStateAtFrame(10, 100)
	assert.NoError(t, err)
	require.Len(t, state, 1)
	assert.Equal(t, "state10", state[0])

	// No exact match, should find next frame
	state, err = m.GetStateAtFrame(5, 100)
	assert.NoError(t, err)
	require.Len(t, state, 1)
	assert.Equal(t, "state10", state[0])

	// Frame not found
	_, err = m.GetStateAtFrame(50, 60)
	assert.Error(t, err)
}

func TestSoldierStatesMap_GetLastState(t *testing.T) {
	m := NewSoldierStatesMap()
	m.Set(10, []any{"state10"})

	// Initially nil
	assert.Nil(t, m.GetLastState())

	// After GetStateAtFrame with fallback search
	m.GetStateAtFrame(5, 100)
	lastState := m.GetLastState()
	require.Len(t, lastState, 1)
	assert.Equal(t, "state10", lastState[0])
}
