package ocapdefs

import (
	"sync"
)

// cache soldiers and vehicles when they are created to avoid subsequent db reads. latency in these calls is critical to quickly process incoming data
type EntityCacheStruct struct {
	m        sync.Mutex
	Soldiers map[uint16]Soldier
	Vehicles map[uint16]Vehicle
}

func NewEntityCache() *EntityCacheStruct {
	return &EntityCacheStruct{
		m:        sync.Mutex{},
		Soldiers: make(map[uint16]Soldier),
		Vehicles: make(map[uint16]Vehicle),
	}
}

func (c *EntityCacheStruct) Reset() {
	c.m.Lock()
	defer c.m.Unlock()
	c.Soldiers = make(map[uint16]Soldier)
	c.Vehicles = make(map[uint16]Vehicle)
}

func (c *EntityCacheStruct) Lock() {
	c.m.Lock()
}

func (c *EntityCacheStruct) Unlock() {
	c.m.Unlock()
}

func (c *EntityCacheStruct) GetSoldier(id uint16) (
	Soldier, bool,
) {
	c.m.Lock()
	defer c.m.Unlock()
	// check if exists
	if s, ok := c.Soldiers[id]; ok {
		return s, true
	} else {
		return Soldier{}, false
	}
}

func (c *EntityCacheStruct) GetVehicle(id uint16) (
	Vehicle, bool,
) {
	c.m.Lock()
	defer c.m.Unlock()
	// check if exists
	if v, ok := c.Vehicles[id]; ok {
		return v, true
	} else {
		return Vehicle{}, false
	}
}

func (c *EntityCacheStruct) AddSoldier(s Soldier) {
	c.m.Lock()
	defer c.m.Unlock()
	c.Soldiers[s.OcapID] = s
}

func (c *EntityCacheStruct) AddVehicle(v Vehicle) {
	c.m.Lock()
	defer c.m.Unlock()
	c.Vehicles[v.OcapID] = v
}

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
