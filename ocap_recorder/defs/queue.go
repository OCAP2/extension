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

type FiredEventsQueue struct {
	mu    sync.Mutex // protects q
	Queue []FiredEvent
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *FiredEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *FiredEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *FiredEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *FiredEventsQueue) Push(n []FiredEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *FiredEventsQueue) Pop() FiredEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return FiredEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *FiredEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *FiredEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []FiredEvent{}
	return len(q.Queue)
}

func (q *FiredEventsQueue) GetAndEmpty() []FiredEvent {
	defer q.Clear()
	return q.Queue
}

type GeneralEventsQueue struct {
	mu    sync.Mutex // protects q
	Queue []GeneralEvent
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *GeneralEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *GeneralEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *GeneralEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *GeneralEventsQueue) Push(n []GeneralEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *GeneralEventsQueue) Pop() GeneralEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return GeneralEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *GeneralEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *GeneralEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []GeneralEvent{}
	return len(q.Queue)
}

func (q *GeneralEventsQueue) GetAndEmpty() []GeneralEvent {
	defer q.Clear()
	return q.Queue
}

type HitEventsQueue struct {
	mu    sync.Mutex // protects q
	Queue []HitEvent
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *HitEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *HitEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *HitEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *HitEventsQueue) Push(n []HitEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *HitEventsQueue) Pop() HitEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return HitEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *HitEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *HitEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []HitEvent{}
	return len(q.Queue)
}

func (q *HitEventsQueue) GetAndEmpty() []HitEvent {
	defer q.Clear()
	return q.Queue
}

type KillEventsQueue struct {
	mu    sync.Mutex // protects q
	Queue []KillEvent
}

// lock, unlock, empty, push, pop, len, clear, getandempty
func (q *KillEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *KillEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *KillEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *KillEventsQueue) Push(n []KillEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *KillEventsQueue) Pop() KillEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return KillEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *KillEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *KillEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []KillEvent{}
	return len(q.Queue)
}

func (q *KillEventsQueue) GetAndEmpty() []KillEvent {
	defer q.Clear()
	return q.Queue
}