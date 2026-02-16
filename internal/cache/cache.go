package cache

import (
	"sync"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// EntityCache caches soldiers and vehicles when they are created to avoid subsequent db reads.
// Latency in these calls is critical to quickly process incoming data.
type EntityCache struct {
	m        sync.Mutex
	Soldiers map[uint16]core.Soldier
	Vehicles map[uint16]core.Vehicle
}

func NewEntityCache() *EntityCache {
	return &EntityCache{
		m:        sync.Mutex{},
		Soldiers: make(map[uint16]core.Soldier),
		Vehicles: make(map[uint16]core.Vehicle),
	}
}

func (c *EntityCache) Reset() {
	c.m.Lock()
	defer c.m.Unlock()
	c.Soldiers = make(map[uint16]core.Soldier)
	c.Vehicles = make(map[uint16]core.Vehicle)
}

func (c *EntityCache) Lock() {
	c.m.Lock()
}

func (c *EntityCache) Unlock() {
	c.m.Unlock()
}

func (c *EntityCache) GetSoldier(id uint16) (core.Soldier, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	if s, ok := c.Soldiers[id]; ok {
		return s, true
	}
	return core.Soldier{}, false
}

func (c *EntityCache) GetVehicle(id uint16) (core.Vehicle, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	if v, ok := c.Vehicles[id]; ok {
		return v, true
	}
	return core.Vehicle{}, false
}

func (c *EntityCache) AddSoldier(s core.Soldier) {
	c.m.Lock()
	defer c.m.Unlock()
	c.Soldiers[s.ID] = s
}

func (c *EntityCache) AddVehicle(v core.Vehicle) {
	c.m.Lock()
	defer c.m.Unlock()
	c.Vehicles[v.ID] = v
}

// SafeCounter is a thread-safe counter
type SafeCounter struct {
	mu sync.Mutex
	v  int
}

func (c *SafeCounter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

func (c *SafeCounter) Set(v int) {
	c.mu.Lock()
	c.v = v
	c.mu.Unlock()
}

func (c *SafeCounter) Inc() {
	c.mu.Lock()
	c.v++
	c.mu.Unlock()
}
