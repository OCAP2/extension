package queue

import (
	"sync"
	"testing"

	"github.com/OCAP2/extension/v5/internal/model"
)

// Test ArraysQueue

func TestArraysQueue_Push(t *testing.T) {
	q := &ArraysQueue{}

	q.Push([][]string{{"a", "b"}, {"c", "d"}})

	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}
}

func TestArraysQueue_Pop(t *testing.T) {
	q := &ArraysQueue{}

	// Pop from empty queue
	result := q.Pop()
	if result != nil {
		t.Errorf("expected nil from empty queue, got %v", result)
	}

	// Pop from non-empty queue
	q.Push([][]string{{"a", "b"}, {"c", "d"}})
	first := q.Pop()
	if len(first) != 2 || first[0] != "a" || first[1] != "b" {
		t.Errorf("expected [a, b], got %v", first)
	}
	if q.Len() != 1 {
		t.Errorf("expected length 1 after pop, got %d", q.Len())
	}
}

func TestArraysQueue_Empty(t *testing.T) {
	q := &ArraysQueue{}

	if !q.Empty() {
		t.Error("expected empty queue")
	}

	q.Push([][]string{{"a"}})
	if q.Empty() {
		t.Error("expected non-empty queue")
	}
}

func TestArraysQueue_Clear(t *testing.T) {
	q := &ArraysQueue{}
	q.Push([][]string{{"a"}, {"b"}, {"c"}})

	q.Clear()

	if !q.Empty() {
		t.Error("expected empty queue after clear")
	}
}

func TestArraysQueue_GetAndEmpty(t *testing.T) {
	q := &ArraysQueue{}
	q.Push([][]string{{"a"}, {"b"}, {"c"}})

	result := q.GetAndEmpty()

	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if !q.Empty() {
		t.Error("expected empty queue after GetAndEmpty")
	}
}

// Test SoldiersQueue

func TestSoldiersQueue_Push(t *testing.T) {
	q := &SoldiersQueue{}

	q.Push([]model.Soldier{{OcapID: 1}, {OcapID: 2}})

	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}
}

func TestSoldiersQueue_Pop(t *testing.T) {
	q := &SoldiersQueue{}

	// Pop from empty queue
	result := q.Pop()
	if result.OcapID != 0 {
		t.Errorf("expected empty soldier from empty queue, got OcapID %d", result.OcapID)
	}

	// Pop from non-empty queue
	q.Push([]model.Soldier{{OcapID: 1}, {OcapID: 2}})
	first := q.Pop()
	if first.OcapID != 1 {
		t.Errorf("expected OcapID 1, got %d", first.OcapID)
	}
	if q.Len() != 1 {
		t.Errorf("expected length 1 after pop, got %d", q.Len())
	}
}

func TestSoldiersQueue_Empty(t *testing.T) {
	q := &SoldiersQueue{}

	if !q.Empty() {
		t.Error("expected empty queue")
	}

	q.Push([]model.Soldier{{OcapID: 1}})
	if q.Empty() {
		t.Error("expected non-empty queue")
	}
}

func TestSoldiersQueue_LockUnlock(t *testing.T) {
	q := &SoldiersQueue{}

	locked := q.Lock()
	if !locked {
		t.Error("expected Lock() to return true")
	}
	// Direct access while holding lock
	q.Queue = append(q.Queue, model.Soldier{OcapID: 99})
	q.Unlock()

	if q.Len() != 1 {
		t.Errorf("expected length 1, got %d", q.Len())
	}
}

func TestSoldiersQueue_Clear(t *testing.T) {
	q := &SoldiersQueue{}
	q.Push([]model.Soldier{{OcapID: 1}, {OcapID: 2}})

	result := q.Clear()

	if result != 0 {
		t.Errorf("expected Clear to return 0, got %d", result)
	}
	if !q.Empty() {
		t.Error("expected empty queue after clear")
	}
}

func TestSoldiersQueue_GetAndEmpty(t *testing.T) {
	q := &SoldiersQueue{}
	q.Push([]model.Soldier{{OcapID: 1}, {OcapID: 2}})

	result := q.GetAndEmpty()

	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
	if !q.Empty() {
		t.Error("expected empty queue after GetAndEmpty")
	}
}

// Test SoldierStatesQueue (representative of state queues)

func TestSoldierStatesQueue_Operations(t *testing.T) {
	q := &SoldierStatesQueue{}

	// Push
	q.Push([]model.SoldierState{{SoldierID: 1}, {SoldierID: 2}})
	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}

	// Pop
	first := q.Pop()
	if first.SoldierID != 1 {
		t.Errorf("expected SoldierID 1, got %d", first.SoldierID)
	}

	// Lock/Unlock
	q.Lock()
	q.Queue = append(q.Queue, model.SoldierState{SoldierID: 3})
	q.Unlock()

	// Clear
	q.Clear()
	if !q.Empty() {
		t.Error("expected empty queue after clear")
	}
}

// Test VehiclesQueue

func TestVehiclesQueue_Operations(t *testing.T) {
	q := &VehiclesQueue{}

	q.Push([]model.Vehicle{{OcapID: 1}, {OcapID: 2}})
	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}

	first := q.Pop()
	if first.OcapID != 1 {
		t.Errorf("expected OcapID 1, got %d", first.OcapID)
	}

	q.Lock()
	q.Unlock()

	q.Clear()
	if !q.Empty() {
		t.Error("expected empty queue after clear")
	}
}

// Test VehicleStatesQueue

func TestVehicleStatesQueue_Operations(t *testing.T) {
	q := &VehicleStatesQueue{}

	q.Push([]model.VehicleState{{VehicleID: 1}, {VehicleID: 2}})
	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}

	first := q.Pop()
	if first.VehicleID != 1 {
		t.Errorf("expected VehicleID 1, got %d", first.VehicleID)
	}

	q.Lock()
	q.Unlock()

	result := q.GetAndEmpty()
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
}

// Test FiredEventsQueue

func TestFiredEventsQueue_Operations(t *testing.T) {
	q := &FiredEventsQueue{}

	q.Push([]model.FiredEvent{{SoldierID: 1}})
	if q.Empty() {
		t.Error("expected non-empty queue")
	}

	q.Pop()
	if !q.Empty() {
		t.Error("expected empty queue after pop")
	}

	q.Lock()
	q.Unlock()
	q.Clear()
}

// Test GeneralEventsQueue

func TestGeneralEventsQueue_Operations(t *testing.T) {
	q := &GeneralEventsQueue{}

	q.Push([]model.GeneralEvent{{Name: "test"}})
	result := q.GetAndEmpty()
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}

	q.Lock()
	q.Unlock()
	q.Clear()
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

// Test MarkersQueue

func TestMarkersQueue_Operations(t *testing.T) {
	q := &MarkersQueue{}

	q.Push([]model.Marker{{MarkerName: "marker1"}})
	if q.Len() != 1 {
		t.Errorf("expected length 1, got %d", q.Len())
	}

	first := q.Pop()
	if first.MarkerName != "marker1" {
		t.Errorf("expected marker1, got %s", first.MarkerName)
	}

	q.Lock()
	q.Unlock()
	q.Clear()
	q.GetAndEmpty()
}

// Test MarkerStatesQueue

func TestMarkerStatesQueue_Operations(t *testing.T) {
	q := &MarkerStatesQueue{}

	q.Push([]model.MarkerState{{MarkerID: 1}})
	if q.Empty() {
		t.Error("expected non-empty queue")
	}

	q.Pop()
	q.Lock()
	q.Unlock()
	q.Clear()
	q.GetAndEmpty()
}

// Test concurrent operations

func TestSoldiersQueue_Concurrent(t *testing.T) {
	q := &SoldiersQueue{}
	var wg sync.WaitGroup

	// Concurrent pushes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id uint16) {
			defer wg.Done()
			q.Push([]model.Soldier{{OcapID: id}})
		}(uint16(i))
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

// Test remaining queue types to ensure coverage

func TestProjectileEventsQueue_Operations(t *testing.T) {
	q := &ProjectileEventsQueue{}
	q.Push([]model.ProjectileEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestHitEventsQueue_Operations(t *testing.T) {
	q := &HitEventsQueue{}
	q.Push([]model.HitEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestKillEventsQueue_Operations(t *testing.T) {
	q := &KillEventsQueue{}
	q.Push([]model.KillEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestChatEventsQueue_Operations(t *testing.T) {
	q := &ChatEventsQueue{}
	q.Push([]model.ChatEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestRadioEventsQueue_Operations(t *testing.T) {
	q := &RadioEventsQueue{}
	q.Push([]model.RadioEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestFpsEventsQueue_Operations(t *testing.T) {
	q := &FpsEventsQueue{}
	q.Push([]model.ServerFpsEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestAce3DeathEventsQueue_Operations(t *testing.T) {
	q := &Ace3DeathEventsQueue{}
	q.Push([]model.Ace3DeathEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}

func TestAce3UnconsciousEventsQueue_Operations(t *testing.T) {
	q := &Ace3UnconsciousEventsQueue{}
	q.Push([]model.Ace3UnconsciousEvent{{}})
	q.Pop()
	q.Lock()
	q.Unlock()
	q.Empty()
	q.Len()
	q.Clear()
	q.GetAndEmpty()
}
