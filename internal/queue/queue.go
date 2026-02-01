package queue

import (
	"fmt"
	"sync"

	"github.com/OCAP2/extension/v5/internal/model"
)

type ArraysQueue struct {
	mu    sync.Mutex
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
	mu    sync.Mutex
	Queue []model.Soldier
}

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

func (q *SoldiersQueue) Push(n []model.Soldier) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *SoldiersQueue) Pop() model.Soldier {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.Soldier{}
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
	q.Queue = []model.Soldier{}
	return len(q.Queue)
}

func (q *SoldiersQueue) GetAndEmpty() []model.Soldier {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type SoldierStatesQueue struct {
	mu    sync.Mutex
	Queue []model.SoldierState
}

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

func (q *SoldierStatesQueue) Push(n []model.SoldierState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *SoldierStatesQueue) Pop() model.SoldierState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.SoldierState{}
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
	q.Queue = []model.SoldierState{}
	return len(q.Queue)
}

func (q *SoldierStatesQueue) GetAndEmpty() []model.SoldierState {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type VehiclesQueue struct {
	mu    sync.Mutex
	Queue []model.Vehicle
}

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

func (q *VehiclesQueue) Push(n []model.Vehicle) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *VehiclesQueue) Pop() model.Vehicle {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.Vehicle{}
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
	q.Queue = []model.Vehicle{}
	return len(q.Queue)
}

func (q *VehiclesQueue) GetAndEmpty() []model.Vehicle {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type VehicleStatesQueue struct {
	mu    sync.Mutex
	Queue []model.VehicleState
}

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

func (q *VehicleStatesQueue) Push(n []model.VehicleState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *VehicleStatesQueue) Pop() model.VehicleState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.VehicleState{}
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
	q.Queue = []model.VehicleState{}
	return len(q.Queue)
}

func (q *VehicleStatesQueue) GetAndEmpty() []model.VehicleState {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type FiredEventsQueue struct {
	mu    sync.Mutex
	Queue []model.FiredEvent
}

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

func (q *FiredEventsQueue) Push(n []model.FiredEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *FiredEventsQueue) Pop() model.FiredEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.FiredEvent{}
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
	q.Queue = []model.FiredEvent{}
	return len(q.Queue)
}

func (q *FiredEventsQueue) GetAndEmpty() []model.FiredEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type ProjectileEventsQueue struct {
	mu    sync.Mutex
	Queue []model.ProjectileEvent
}

func (q *ProjectileEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *ProjectileEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *ProjectileEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *ProjectileEventsQueue) Push(n []model.ProjectileEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *ProjectileEventsQueue) Pop() model.ProjectileEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.ProjectileEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *ProjectileEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *ProjectileEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.ProjectileEvent{}
	return len(q.Queue)
}

func (q *ProjectileEventsQueue) GetAndEmpty() []model.ProjectileEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type GeneralEventsQueue struct {
	mu    sync.Mutex
	Queue []model.GeneralEvent
}

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

func (q *GeneralEventsQueue) Push(n []model.GeneralEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *GeneralEventsQueue) Pop() model.GeneralEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.GeneralEvent{}
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
	q.Queue = []model.GeneralEvent{}
	return len(q.Queue)
}

func (q *GeneralEventsQueue) GetAndEmpty() []model.GeneralEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type HitEventsQueue struct {
	mu    sync.Mutex
	Queue []model.HitEvent
}

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

func (q *HitEventsQueue) Push(n []model.HitEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *HitEventsQueue) Pop() model.HitEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.HitEvent{}
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
	q.Queue = []model.HitEvent{}
	return len(q.Queue)
}

func (q *HitEventsQueue) GetAndEmpty() []model.HitEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type KillEventsQueue struct {
	mu    sync.Mutex
	Queue []model.KillEvent
}

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

func (q *KillEventsQueue) Push(n []model.KillEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *KillEventsQueue) Pop() model.KillEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.KillEvent{}
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
	q.Queue = []model.KillEvent{}
	return len(q.Queue)
}

func (q *KillEventsQueue) GetAndEmpty() []model.KillEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type ChatEventsQueue struct {
	mu    sync.Mutex
	Queue []model.ChatEvent
}

func (q *ChatEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *ChatEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *ChatEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *ChatEventsQueue) Push(n []model.ChatEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *ChatEventsQueue) Pop() model.ChatEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.ChatEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *ChatEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *ChatEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.ChatEvent{}
	return len(q.Queue)
}

func (q *ChatEventsQueue) GetAndEmpty() []model.ChatEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type RadioEventsQueue struct {
	mu    sync.Mutex
	Queue []model.RadioEvent
}

func (q *RadioEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *RadioEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *RadioEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *RadioEventsQueue) Push(n []model.RadioEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *RadioEventsQueue) Pop() model.RadioEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.RadioEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *RadioEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *RadioEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.RadioEvent{}
	return len(q.Queue)
}

func (q *RadioEventsQueue) GetAndEmpty() []model.RadioEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type FpsEventsQueue struct {
	mu    sync.Mutex
	Queue []model.ServerFpsEvent
}

func (q *FpsEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *FpsEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *FpsEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *FpsEventsQueue) Push(n []model.ServerFpsEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *FpsEventsQueue) Pop() model.ServerFpsEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.ServerFpsEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *FpsEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *FpsEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.ServerFpsEvent{}
	return len(q.Queue)
}

func (q *FpsEventsQueue) GetAndEmpty() []model.ServerFpsEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type Ace3DeathEventsQueue struct {
	mu    sync.Mutex
	Queue []model.Ace3DeathEvent
}

func (q *Ace3DeathEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *Ace3DeathEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *Ace3DeathEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *Ace3DeathEventsQueue) Push(n []model.Ace3DeathEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *Ace3DeathEventsQueue) Pop() model.Ace3DeathEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.Ace3DeathEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *Ace3DeathEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *Ace3DeathEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.Ace3DeathEvent{}
	return len(q.Queue)
}

func (q *Ace3DeathEventsQueue) GetAndEmpty() []model.Ace3DeathEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type Ace3UnconsciousEventsQueue struct {
	mu    sync.Mutex
	Queue []model.Ace3UnconsciousEvent
}

func (q *Ace3UnconsciousEventsQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *Ace3UnconsciousEventsQueue) Unlock() {
	q.mu.Unlock()
}

func (q *Ace3UnconsciousEventsQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *Ace3UnconsciousEventsQueue) Push(n []model.Ace3UnconsciousEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *Ace3UnconsciousEventsQueue) Pop() model.Ace3UnconsciousEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.Ace3UnconsciousEvent{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *Ace3UnconsciousEventsQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *Ace3UnconsciousEventsQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.Ace3UnconsciousEvent{}
	return len(q.Queue)
}

func (q *Ace3UnconsciousEventsQueue) GetAndEmpty() []model.Ace3UnconsciousEvent {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

// SoldierStatesMap processes soldier states for write out to JSON
type SoldierStatesMap struct {
	frameData map[uint][]any
	lastState []any
}

func NewSoldierStatesMap() *SoldierStatesMap {
	return &SoldierStatesMap{
		frameData: make(map[uint][]any),
	}
}

func (q *SoldierStatesMap) Set(frame uint, state []any) {
	q.frameData[frame] = state
}

func (q *SoldierStatesMap) Len() int {
	return len(q.frameData)
}

func (q *SoldierStatesMap) GetStateAtFrame(frame uint, endFrame uint) ([]any, error) {
	state, ok := q.frameData[frame]
	if !ok {
		for i := frame; i <= endFrame; i++ {
			state, ok := q.frameData[i]
			if ok {
				q.lastState = state
				return state, nil
			}
		}
		return []any{}, fmt.Errorf("no soldier state found for frame %d", frame)
	}
	return state, nil
}

func (q *SoldierStatesMap) GetLastState() []any {
	return q.lastState
}

type MarkersQueue struct {
	mu    sync.Mutex
	Queue []model.Marker
}

func (q *MarkersQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *MarkersQueue) Unlock() {
	q.mu.Unlock()
}

func (q *MarkersQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *MarkersQueue) Push(n []model.Marker) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *MarkersQueue) Pop() model.Marker {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.Marker{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *MarkersQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *MarkersQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.Marker{}
	return len(q.Queue)
}

func (q *MarkersQueue) GetAndEmpty() []model.Marker {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}

type MarkerStatesQueue struct {
	mu    sync.Mutex
	Queue []model.MarkerState
}

func (q *MarkerStatesQueue) Lock() bool {
	q.mu.Lock()
	return true
}

func (q *MarkerStatesQueue) Unlock() {
	q.mu.Unlock()
}

func (q *MarkerStatesQueue) Empty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue) == 0
}

func (q *MarkerStatesQueue) Push(n []model.MarkerState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = append(q.Queue, n...)
}

func (q *MarkerStatesQueue) Pop() model.MarkerState {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Queue) == 0 {
		return model.MarkerState{}
	}
	n := q.Queue[0]
	q.Queue = q.Queue[1:]
	return n
}

func (q *MarkerStatesQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Queue)
}

func (q *MarkerStatesQueue) Clear() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Queue = []model.MarkerState{}
	return len(q.Queue)
}

func (q *MarkerStatesQueue) GetAndEmpty() []model.MarkerState {
	q.mu.Lock()
	defer q.Clear()
	defer q.mu.Unlock()
	return q.Queue
}
