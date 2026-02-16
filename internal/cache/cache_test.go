package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/model/core"
)

func TestEntityCache_NewEntityCache(t *testing.T) {
	cache := NewEntityCache()

	require.NotNil(t, cache)
	assert.NotNil(t, cache.Soldiers)
	assert.NotNil(t, cache.Vehicles)
	assert.Len(t, cache.Soldiers, 0)
	assert.Len(t, cache.Vehicles, 0)
}

func TestEntityCache_AddAndGetSoldier(t *testing.T) {
	cache := NewEntityCache()

	soldier := core.Soldier{
		ID:       42,
		UnitName: "Test Soldier",
	}

	cache.AddSoldier(soldier)

	got, ok := cache.GetSoldier(42)
	require.True(t, ok, "expected to find soldier with ID 42")
	assert.Equal(t, uint16(42), got.ID)
	assert.Equal(t, "Test Soldier", got.UnitName)
}

func TestEntityCache_GetSoldier_NotFound(t *testing.T) {
	cache := NewEntityCache()

	_, ok := cache.GetSoldier(999)
	assert.False(t, ok, "expected not to find soldier with ID 999")
}

func TestEntityCache_AddAndGetVehicle(t *testing.T) {
	cache := NewEntityCache()

	vehicle := core.Vehicle{
		ID:        99,
		ClassName: "Test_Vehicle",
	}

	cache.AddVehicle(vehicle)

	got, ok := cache.GetVehicle(99)
	require.True(t, ok, "expected to find vehicle with ID 99")
	assert.Equal(t, uint16(99), got.ID)
	assert.Equal(t, "Test_Vehicle", got.ClassName)
}

func TestEntityCache_GetVehicle_NotFound(t *testing.T) {
	cache := NewEntityCache()

	_, ok := cache.GetVehicle(999)
	assert.False(t, ok, "expected not to find vehicle with ID 999")
}

func TestEntityCache_Reset(t *testing.T) {
	cache := NewEntityCache()

	// Add some data
	cache.AddSoldier(core.Soldier{ID: 1, UnitName: "Soldier 1"})
	cache.AddSoldier(core.Soldier{ID: 2, UnitName: "Soldier 2"})
	cache.AddVehicle(core.Vehicle{ID: 10, ClassName: "Vehicle 1"})

	// Verify data exists
	assert.Len(t, cache.Soldiers, 2)
	assert.Len(t, cache.Vehicles, 1)

	// Reset
	cache.Reset()

	// Verify data is cleared
	assert.Len(t, cache.Soldiers, 0)
	assert.Len(t, cache.Vehicles, 0)

	// Verify we can still add data after reset
	cache.AddSoldier(core.Soldier{ID: 3, UnitName: "Soldier 3"})
	_, ok := cache.GetSoldier(3)
	assert.True(t, ok, "expected to find soldier added after reset")
}

func TestEntityCache_LockUnlock(t *testing.T) {
	cache := NewEntityCache()

	// Test Lock/Unlock don't cause deadlock
	cache.Lock()
	// Directly modify the map while holding the lock
	cache.Soldiers[1] = core.Soldier{ID: 1, UnitName: "Direct Add"}
	cache.Unlock()

	// Verify the data was added
	got, ok := cache.GetSoldier(1)
	require.True(t, ok, "expected to find soldier added while holding lock")
	assert.Equal(t, "Direct Add", got.UnitName)
}

func TestEntityCache_Concurrent(t *testing.T) {
	cache := NewEntityCache()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := uint16(0); i < 100; i++ {
		wg.Add(2)
		go func(id uint16) {
			defer wg.Done()
			cache.AddSoldier(core.Soldier{ID: id, UnitName: "Soldier"})
		}(i)
		go func(id uint16) {
			defer wg.Done()
			cache.AddVehicle(core.Vehicle{ID: id, ClassName: "Vehicle"})
		}(i)
	}
	wg.Wait()

	// Verify counts
	assert.Len(t, cache.Soldiers, 100)
	assert.Len(t, cache.Vehicles, 100)

	// Concurrent reads
	for i := uint16(0); i < 100; i++ {
		wg.Add(2)
		go func(id uint16) {
			defer wg.Done()
			cache.GetSoldier(id)
		}(i)
		go func(id uint16) {
			defer wg.Done()
			cache.GetVehicle(id)
		}(i)
	}
	wg.Wait()
}

// SafeCounter tests

func TestSafeCounter_InitialValue(t *testing.T) {
	c := &SafeCounter{}
	assert.Equal(t, int(0), c.Value())
}

func TestSafeCounter_Set(t *testing.T) {
	c := &SafeCounter{}

	c.Set(42)
	assert.Equal(t, int(42), c.Value())

	c.Set(100)
	assert.Equal(t, int(100), c.Value())

	c.Set(0)
	assert.Equal(t, int(0), c.Value())
}

func TestSafeCounter_Inc(t *testing.T) {
	c := &SafeCounter{}

	c.Inc()
	assert.Equal(t, int(1), c.Value())

	c.Inc()
	c.Inc()
	assert.Equal(t, int(3), c.Value())
}

func TestSafeCounter_Concurrent(t *testing.T) {
	c := &SafeCounter{}
	var wg sync.WaitGroup

	// Concurrent increments
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Inc()
		}()
	}
	wg.Wait()

	assert.Equal(t, int(1000), c.Value())
}
