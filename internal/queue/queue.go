package queue

import (
	"fmt"
	"sync"
)

// Queue is a generic thread-safe queue.
type Queue[T any] struct {
	mu    sync.Mutex
	items []T
}

// New creates a new empty queue.
func New[T any]() *Queue[T] {
	return &Queue[T]{
		items: make([]T, 0),
	}
}

// Push appends items to the queue.
func (q *Queue[T]) Push(items ...T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, items...)
}

// Pop removes and returns the first item. Returns zero value if empty.
func (q *Queue[T]) Pop() T {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		var zero T
		return zero
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item
}

// Empty returns true if the queue has no items.
func (q *Queue[T]) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) == 0
}

// Len returns the number of items in the queue.
func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Clear removes all items from the queue.
func (q *Queue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = q.items[:0]
}

// GetAndEmpty returns all items and clears the queue.
func (q *Queue[T]) GetAndEmpty() []T {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := q.items
	q.items = make([]T, 0, cap(q.items))
	return result
}

// SoldierStatesMap processes soldier states for write out to JSON.
// This is a separate data structure, not a queue.
type SoldierStatesMap struct {
	frameData map[uint][]any
	lastState []any
}

// NewSoldierStatesMap creates a new SoldierStatesMap.
func NewSoldierStatesMap() *SoldierStatesMap {
	return &SoldierStatesMap{
		frameData: make(map[uint][]any),
	}
}

// Set stores state data for a frame.
func (m *SoldierStatesMap) Set(frame uint, state []any) {
	m.frameData[frame] = state
}

// Len returns the number of frames stored.
func (m *SoldierStatesMap) Len() int {
	return len(m.frameData)
}

// GetStateAtFrame returns the state at a given frame, searching forward if not found.
func (m *SoldierStatesMap) GetStateAtFrame(frame uint, endFrame uint) ([]any, error) {
	state, ok := m.frameData[frame]
	if !ok {
		for i := frame; i <= endFrame; i++ {
			state, ok := m.frameData[i]
			if ok {
				m.lastState = state
				return state, nil
			}
		}
		return []any{}, fmt.Errorf("no soldier state found for frame %d", frame)
	}
	return state, nil
}

// GetLastState returns the last state that was retrieved.
func (m *SoldierStatesMap) GetLastState() []any {
	return m.lastState
}
