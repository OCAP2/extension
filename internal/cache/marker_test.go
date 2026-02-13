package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkerCache_NewMarkerCache(t *testing.T) {
	cache := NewMarkerCache()

	require.NotNil(t, cache)
	assert.NotNil(t, cache.markers)
}

func TestMarkerCache_SetAndGet(t *testing.T) {
	cache := NewMarkerCache()

	cache.Set("marker1", 42)

	id, ok := cache.Get("marker1")
	require.True(t, ok, "expected to find marker1")
	assert.Equal(t, uint(42), id)
}

func TestMarkerCache_Get_NotFound(t *testing.T) {
	cache := NewMarkerCache()

	_, ok := cache.Get("nonexistent")
	assert.False(t, ok, "expected not to find nonexistent marker")
}

func TestMarkerCache_Delete(t *testing.T) {
	cache := NewMarkerCache()

	cache.Set("marker1", 1)
	cache.Set("marker2", 2)

	// Verify marker exists
	_, ok := cache.Get("marker1")
	require.True(t, ok, "expected to find marker1 before delete")

	// Delete marker
	cache.Delete("marker1")

	// Verify marker is deleted
	_, ok = cache.Get("marker1")
	assert.False(t, ok, "expected not to find marker1 after delete")

	// Verify other marker still exists
	_, ok = cache.Get("marker2")
	assert.True(t, ok, "expected marker2 to still exist")
}

func TestMarkerCache_Delete_NonExistent(t *testing.T) {
	cache := NewMarkerCache()

	// Should not panic when deleting non-existent marker
	cache.Delete("nonexistent")
}

func TestMarkerCache_Reset(t *testing.T) {
	cache := NewMarkerCache()

	cache.Set("marker1", 1)
	cache.Set("marker2", 2)
	cache.Set("marker3", 3)

	cache.Reset()

	// Verify all markers are cleared
	_, ok := cache.Get("marker1")
	assert.False(t, ok, "expected marker1 to be cleared after reset")

	_, ok = cache.Get("marker2")
	assert.False(t, ok, "expected marker2 to be cleared after reset")

	_, ok = cache.Get("marker3")
	assert.False(t, ok, "expected marker3 to be cleared after reset")

	// Verify we can still add markers after reset
	cache.Set("marker4", 4)
	_, ok = cache.Get("marker4")
	assert.True(t, ok, "expected to find marker4 after reset")
}

func TestMarkerCache_OverwriteExisting(t *testing.T) {
	cache := NewMarkerCache()

	cache.Set("marker1", 1)
	cache.Set("marker1", 100)

	id, ok := cache.Get("marker1")
	require.True(t, ok, "expected to find marker1")
	assert.Equal(t, uint(100), id)
}

func TestMarkerCache_Concurrent(t *testing.T) {
	cache := NewMarkerCache()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.Set("marker"+string(rune('A'+id%26)), uint(id))
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.Get("marker" + string(rune('A'+id%26)))
		}(i)
	}
	wg.Wait()

	// Concurrent deletes
	for i := 0; i < 26; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.Delete("marker" + string(rune('A'+id)))
		}(i)
	}
	wg.Wait()
}

func TestMarkerCache_ConcurrentReadWrite(t *testing.T) {
	cache := NewMarkerCache()
	var wg sync.WaitGroup

	// Mixed concurrent operations
	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func(id int) {
			defer wg.Done()
			cache.Set("marker", uint(id))
		}(i)

		go func() {
			defer wg.Done()
			cache.Get("marker")
		}()

		go func() {
			defer wg.Done()
			cache.Delete("marker")
		}()
	}

	wg.Wait()
}
