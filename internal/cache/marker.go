package cache

import "sync"

// MarkerCache maps marker names to their database IDs for the current mission
type MarkerCache struct {
	mu      sync.RWMutex
	markers map[string]uint
}

// NewMarkerCache creates a new MarkerCache
func NewMarkerCache() *MarkerCache {
	return &MarkerCache{
		markers: make(map[string]uint),
	}
}

// Get retrieves a marker ID by name
func (c *MarkerCache) Get(name string) (uint, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.markers[name]
	return id, ok
}

// Set stores a marker ID by name
func (c *MarkerCache) Set(name string, id uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markers[name] = id
}

// Delete removes a marker by name
func (c *MarkerCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.markers, name)
}

// Reset clears all markers from the cache
func (c *MarkerCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markers = make(map[string]uint)
}
