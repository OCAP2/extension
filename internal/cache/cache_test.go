package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
)

func TestEntityCache_NewEntityCache(t *testing.T) {
	cache := NewEntityCache()

	require.NotNil(t, cache)
	assert.Equal(t, 0, cache.SoldierCount())
	assert.Equal(t, 0, cache.VehicleCount())
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
	assert.Equal(t, 2, cache.SoldierCount())
	assert.Equal(t, 1, cache.VehicleCount())

	// Reset
	cache.Reset()

	// Verify data is cleared
	assert.Equal(t, 0, cache.SoldierCount())
	assert.Equal(t, 0, cache.VehicleCount())

	// Verify we can still add data after reset
	cache.AddSoldier(core.Soldier{ID: 3, UnitName: "Soldier 3"})
	_, ok := cache.GetSoldier(3)
	assert.True(t, ok, "expected to find soldier added after reset")
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
	assert.Equal(t, 100, cache.SoldierCount())
	assert.Equal(t, 100, cache.VehicleCount())

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
