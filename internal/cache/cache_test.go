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
	cache.AddPlacedObject(core.PlacedObject{ID: 50, DisplayName: "Mine"})

	// Verify data exists
	assert.Equal(t, 2, cache.SoldierCount())
	assert.Equal(t, 1, cache.VehicleCount())
	_, ok := cache.GetPlacedObject(50)
	require.True(t, ok)

	// Reset
	cache.Reset()

	// Verify data is cleared
	assert.Equal(t, 0, cache.SoldierCount())
	assert.Equal(t, 0, cache.VehicleCount())
	_, ok = cache.GetPlacedObject(50)
	assert.False(t, ok, "expected placed object to be cleared after reset")

	// Verify we can still add data after reset
	cache.AddSoldier(core.Soldier{ID: 3, UnitName: "Soldier 3"})
	_, ok = cache.GetSoldier(3)
	assert.True(t, ok, "expected to find soldier added after reset")
}

func TestEntityCache_UpdateSoldier(t *testing.T) {
	cache := NewEntityCache()

	// Add an AI soldier
	cache.AddSoldier(core.Soldier{ID: 10, UnitName: "Habibzai", IsPlayer: false})

	// Update: player takes over
	cache.UpdateSoldier(core.Soldier{ID: 10, UnitName: "zigster", IsPlayer: true})

	got, ok := cache.GetSoldier(10)
	require.True(t, ok)
	assert.Equal(t, "zigster", got.UnitName)
	assert.True(t, got.IsPlayer)
}

func TestEntityCache_AddAndGetPlacedObject(t *testing.T) {
	cache := NewEntityCache()

	placed := core.PlacedObject{
		ID:          100,
		ClassName:   "APERSMine_Range_Ammo",
		DisplayName: "APERS Mine",
	}

	cache.AddPlacedObject(placed)

	got, ok := cache.GetPlacedObject(100)
	require.True(t, ok, "expected to find placed object with ID 100")
	assert.Equal(t, uint16(100), got.ID)
	assert.Equal(t, "APERS Mine", got.DisplayName)
}

func TestEntityCache_GetPlacedObject_NotFound(t *testing.T) {
	cache := NewEntityCache()

	_, ok := cache.GetPlacedObject(999)
	assert.False(t, ok, "expected not to find placed object with ID 999")
}

func TestEntityCache_Reset_ClearsPlacedObjects(t *testing.T) {
	cache := NewEntityCache()

	cache.AddPlacedObject(core.PlacedObject{ID: 100, DisplayName: "Mine 1"})
	cache.AddPlacedObject(core.PlacedObject{ID: 101, DisplayName: "Mine 2"})

	// Verify data exists
	_, ok := cache.GetPlacedObject(100)
	require.True(t, ok)

	// Reset
	cache.Reset()

	// Verify placed objects are cleared
	_, ok = cache.GetPlacedObject(100)
	assert.False(t, ok, "expected placed object 100 to be cleared after reset")
	_, ok = cache.GetPlacedObject(101)
	assert.False(t, ok, "expected placed object 101 to be cleared after reset")

	// Verify we can still add data after reset
	cache.AddPlacedObject(core.PlacedObject{ID: 200, DisplayName: "New Mine"})
	got, ok := cache.GetPlacedObject(200)
	assert.True(t, ok, "expected to find placed object added after reset")
	assert.Equal(t, "New Mine", got.DisplayName)
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
