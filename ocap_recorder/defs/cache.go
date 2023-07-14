package defs

import "sync"

type OcapIDCache struct {
	mu    sync.Mutex // protects q
	cache map[uint16]uint
}

func (q *OcapIDCache) Init() {
	q.cache = make(map[uint16]uint)
}

func (q *OcapIDCache) Set(ocapId uint16, id uint) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cache[ocapId] = id
}

func (q *OcapIDCache) Get(ocapId uint16) (uint, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	id, ok := q.cache[ocapId]
	return id, ok
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
