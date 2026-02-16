package cache

import (
	"sync"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// EntityCache caches soldiers and vehicles when they are created to avoid subsequent db reads.
// Latency in these calls is critical to quickly process incoming data.
type EntityCache struct {
	m        sync.Mutex
	soldiers map[uint16]core.Soldier
	vehicles map[uint16]core.Vehicle
}

func NewEntityCache() *EntityCache {
	return &EntityCache{
		m:        sync.Mutex{},
		soldiers: make(map[uint16]core.Soldier),
		vehicles: make(map[uint16]core.Vehicle),
	}
}

func (c *EntityCache) Reset() {
	c.m.Lock()
	defer c.m.Unlock()
	c.soldiers = make(map[uint16]core.Soldier)
	c.vehicles = make(map[uint16]core.Vehicle)
}

func (c *EntityCache) SoldierCount() int {
	c.m.Lock()
	defer c.m.Unlock()
	return len(c.soldiers)
}

func (c *EntityCache) VehicleCount() int {
	c.m.Lock()
	defer c.m.Unlock()
	return len(c.vehicles)
}

func (c *EntityCache) GetSoldier(id uint16) (core.Soldier, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	if s, ok := c.soldiers[id]; ok {
		return s, true
	}
	return core.Soldier{}, false
}

func (c *EntityCache) GetVehicle(id uint16) (core.Vehicle, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	if v, ok := c.vehicles[id]; ok {
		return v, true
	}
	return core.Vehicle{}, false
}

func (c *EntityCache) AddSoldier(s core.Soldier) {
	c.m.Lock()
	defer c.m.Unlock()
	c.soldiers[s.ID] = s
}

func (c *EntityCache) AddVehicle(v core.Vehicle) {
	c.m.Lock()
	defer c.m.Unlock()
	c.vehicles[v.ID] = v
}
