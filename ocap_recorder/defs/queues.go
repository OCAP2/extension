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
	queue []Soldier
}

func (q *SoldiersQueue) Push(n []Soldier) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, n...)
}

func (q *SoldiersQueue) Pop() Soldier {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return Soldier{}
	}
	n := q.queue[0]
	q.queue = q.queue[1:]
	return n
}

func (q *SoldiersQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

func (q *SoldiersQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = []Soldier{}
	return len(q.queue)
}

func (q *SoldiersQueue) GetAndEmpty() []Soldier {
	defer q.Clear()
	return q.queue
}

type SoldierStatesQueue struct {
	mu    sync.Mutex // protects q
	queue []SoldierState
}

func (q *SoldierStatesQueue) Push(n []SoldierState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, n...)
}

func (q *SoldierStatesQueue) Pop() SoldierState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return SoldierState{}
	}
	n := q.queue[0]
	q.queue = q.queue[1:]
	return n
}

func (q *SoldierStatesQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

func (q *SoldierStatesQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = []SoldierState{}
	return len(q.queue)
}

func (q *SoldierStatesQueue) GetAndEmpty() []SoldierState {
	defer q.Clear()
	return q.queue
}
