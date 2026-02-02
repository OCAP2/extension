package cache

import (
	"sync"
	"testing"

	"github.com/OCAP2/extension/v5/internal/model"
)

func TestEntityCache_NewEntityCache(t *testing.T) {
	cache := NewEntityCache()

	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if cache.Soldiers == nil {
		t.Error("expected Soldiers map to be initialized")
	}
	if cache.Vehicles == nil {
		t.Error("expected Vehicles map to be initialized")
	}
	if len(cache.Soldiers) != 0 {
		t.Errorf("expected empty Soldiers map, got %d entries", len(cache.Soldiers))
	}
	if len(cache.Vehicles) != 0 {
		t.Errorf("expected empty Vehicles map, got %d entries", len(cache.Vehicles))
	}
}

func TestEntityCache_AddAndGetSoldier(t *testing.T) {
	cache := NewEntityCache()

	soldier := model.Soldier{
		OcapID:   42,
		UnitName: "Test Soldier",
	}

	cache.AddSoldier(soldier)

	got, ok := cache.GetSoldier(42)
	if !ok {
		t.Fatal("expected to find soldier with OcapID 42")
	}
	if got.OcapID != 42 {
		t.Errorf("expected OcapID 42, got %d", got.OcapID)
	}
	if got.UnitName != "Test Soldier" {
		t.Errorf("expected UnitName 'Test Soldier', got %s", got.UnitName)
	}
}

func TestEntityCache_GetSoldier_NotFound(t *testing.T) {
	cache := NewEntityCache()

	_, ok := cache.GetSoldier(999)
	if ok {
		t.Error("expected not to find soldier with OcapID 999")
	}
}

func TestEntityCache_AddAndGetVehicle(t *testing.T) {
	cache := NewEntityCache()

	vehicle := model.Vehicle{
		OcapID:    99,
		ClassName: "Test_Vehicle",
	}

	cache.AddVehicle(vehicle)

	got, ok := cache.GetVehicle(99)
	if !ok {
		t.Fatal("expected to find vehicle with OcapID 99")
	}
	if got.OcapID != 99 {
		t.Errorf("expected OcapID 99, got %d", got.OcapID)
	}
	if got.ClassName != "Test_Vehicle" {
		t.Errorf("expected className 'Test_Vehicle', got %s", got.ClassName)
	}
}

func TestEntityCache_GetVehicle_NotFound(t *testing.T) {
	cache := NewEntityCache()

	_, ok := cache.GetVehicle(999)
	if ok {
		t.Error("expected not to find vehicle with OcapID 999")
	}
}

func TestEntityCache_Reset(t *testing.T) {
	cache := NewEntityCache()

	// Add some data
	cache.AddSoldier(model.Soldier{OcapID: 1, UnitName: "Soldier 1"})
	cache.AddSoldier(model.Soldier{OcapID: 2, UnitName: "Soldier 2"})
	cache.AddVehicle(model.Vehicle{OcapID: 10, ClassName: "Vehicle 1"})

	// Verify data exists
	if len(cache.Soldiers) != 2 {
		t.Errorf("expected 2 soldiers before reset, got %d", len(cache.Soldiers))
	}
	if len(cache.Vehicles) != 1 {
		t.Errorf("expected 1 vehicle before reset, got %d", len(cache.Vehicles))
	}

	// Reset
	cache.Reset()

	// Verify data is cleared
	if len(cache.Soldiers) != 0 {
		t.Errorf("expected 0 soldiers after reset, got %d", len(cache.Soldiers))
	}
	if len(cache.Vehicles) != 0 {
		t.Errorf("expected 0 vehicles after reset, got %d", len(cache.Vehicles))
	}

	// Verify we can still add data after reset
	cache.AddSoldier(model.Soldier{OcapID: 3, UnitName: "Soldier 3"})
	_, ok := cache.GetSoldier(3)
	if !ok {
		t.Error("expected to find soldier added after reset")
	}
}

func TestEntityCache_LockUnlock(t *testing.T) {
	cache := NewEntityCache()

	// Test Lock/Unlock don't cause deadlock
	cache.Lock()
	// Directly modify the map while holding the lock
	cache.Soldiers[1] = model.Soldier{OcapID: 1, UnitName: "Direct Add"}
	cache.Unlock()

	// Verify the data was added
	got, ok := cache.GetSoldier(1)
	if !ok {
		t.Fatal("expected to find soldier added while holding lock")
	}
	if got.UnitName != "Direct Add" {
		t.Errorf("expected UnitName 'Direct Add', got %s", got.UnitName)
	}
}

func TestEntityCache_Concurrent(t *testing.T) {
	cache := NewEntityCache()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := uint16(0); i < 100; i++ {
		wg.Add(2)
		go func(id uint16) {
			defer wg.Done()
			cache.AddSoldier(model.Soldier{OcapID: id, UnitName: "Soldier"})
		}(i)
		go func(id uint16) {
			defer wg.Done()
			cache.AddVehicle(model.Vehicle{OcapID: id, ClassName: "Vehicle"})
		}(i)
	}
	wg.Wait()

	// Verify counts
	if len(cache.Soldiers) != 100 {
		t.Errorf("expected 100 soldiers, got %d", len(cache.Soldiers))
	}
	if len(cache.Vehicles) != 100 {
		t.Errorf("expected 100 vehicles, got %d", len(cache.Vehicles))
	}

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
	if c.Value() != 0 {
		t.Errorf("expected initial value 0, got %d", c.Value())
	}
}

func TestSafeCounter_Set(t *testing.T) {
	c := &SafeCounter{}

	c.Set(42)
	if c.Value() != 42 {
		t.Errorf("expected value 42, got %d", c.Value())
	}

	c.Set(100)
	if c.Value() != 100 {
		t.Errorf("expected value 100, got %d", c.Value())
	}

	c.Set(0)
	if c.Value() != 0 {
		t.Errorf("expected value 0, got %d", c.Value())
	}
}

func TestSafeCounter_Inc(t *testing.T) {
	c := &SafeCounter{}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("expected value 1, got %d", c.Value())
	}

	c.Inc()
	c.Inc()
	if c.Value() != 3 {
		t.Errorf("expected value 3, got %d", c.Value())
	}
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

	if c.Value() != 1000 {
		t.Errorf("expected value 1000, got %d", c.Value())
	}
}
