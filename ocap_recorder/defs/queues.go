package defs

import "sync"

type ArraysQueue struct {
	mu    sync.Mutex // protects q
	queue [][]string
}

func (q *ArraysQueue) Push(n [][]string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, n...)
}

func (q *ArraysQueue) Pop() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil
	}
	n := q.queue[0]
	q.queue = q.queue[1:]
	return n
}

func (q *ArraysQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

func (q *ArraysQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue) == 0
}

func (q *ArraysQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = [][]string{}
}

func (q *ArraysQueue) GetAndEmpty() [][]string {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.queue
}

type SoldiersQueue struct {
	mu    sync.Mutex // protects q
	Queue []Soldier
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *SoldiersQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *SoldiersQueue) Unlock() {
	q.mu.Unlock()
}

func (q *SoldiersQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *SoldiersQueue) Push(n []Soldier) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *SoldiersQueue) Pop() Soldier {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return Soldier{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *SoldiersQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *SoldiersQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []Soldier{}
	return len(q.Queue)
}

func (q *SoldiersQueue) GetAndEmpty() []Soldier {
	defer q.Clear()
	return q.Queue
}

type SoldierStatesQueue struct {
	mu    sync.Mutex // protects q
	Queue []SoldierState
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *SoldierStatesQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *SoldierStatesQueue) Unlock() {
	q.mu.Unlock()
}

func (q *SoldierStatesQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *SoldierStatesQueue) Push(n []SoldierState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *SoldierStatesQueue) Pop() SoldierState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return SoldierState{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *SoldierStatesQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *SoldierStatesQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []SoldierState{}
	return len(q.Queue)
}

func (q *SoldierStatesQueue) GetAndEmpty() []SoldierState {
	defer q.Clear()
	return q.Queue
}

type VehiclesQueue struct {
	mu    sync.Mutex // protects q
	Queue []Vehicle
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *VehiclesQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *VehiclesQueue) Unlock() {
	q.mu.Unlock()
}

func (q *VehiclesQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *VehiclesQueue) Push(n []Vehicle) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *VehiclesQueue) Pop() Vehicle {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return Vehicle{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *VehiclesQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *VehiclesQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []Vehicle{}
	return len(q.Queue)
}

func (q *VehiclesQueue) GetAndEmpty() []Vehicle {
	defer q.Clear()
	return q.Queue
}

type VehicleStatesQueue struct {
	mu    sync.Mutex // protects q
	Queue []VehicleState
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *VehicleStatesQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *VehicleStatesQueue) Unlock() {
	q.mu.Unlock()
}

func (q *VehicleStatesQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *VehicleStatesQueue) Push(n []VehicleState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *VehicleStatesQueue) Pop() VehicleState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return VehicleState{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *VehicleStatesQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *VehicleStatesQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []VehicleState{}
	return len(q.Queue)
}

func (q *VehicleStatesQueue) GetAndEmpty() []VehicleState {
	defer q.Clear()
	return q.Queue
}
