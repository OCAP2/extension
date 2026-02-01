// internal/storage/memory/memory.go
package memory

import (
	"sync"

	"github.com/OCAP2/extension/internal/config"
	"github.com/OCAP2/extension/internal/model/core"
)

// SoldierRecord groups a soldier with all its time-series data
type SoldierRecord struct {
	Soldier     core.Soldier
	States      []core.SoldierState
	FiredEvents []core.FiredEvent
}

// VehicleRecord groups a vehicle with all its time-series data
type VehicleRecord struct {
	Vehicle core.Vehicle
	States  []core.VehicleState
}

// MarkerRecord groups a marker with all its state changes
type MarkerRecord struct {
	Marker core.Marker
	States []core.MarkerState
}

// Backend stores mission data in memory and exports to JSON
type Backend struct {
	cfg     config.MemoryConfig
	mission *core.Mission
	world   *core.World

	soldiers map[uint16]*SoldierRecord // keyed by OcapID
	vehicles map[uint16]*VehicleRecord // keyed by OcapID
	markers  map[string]*MarkerRecord  // keyed by MarkerName

	generalEvents         []core.GeneralEvent
	hitEvents             []core.HitEvent
	killEvents            []core.KillEvent
	chatEvents            []core.ChatEvent
	radioEvents           []core.RadioEvent
	serverFpsEvents       []core.ServerFpsEvent
	ace3DeathEvents       []core.Ace3DeathEvent
	ace3UnconsciousEvents []core.Ace3UnconsciousEvent

	idCounter uint
	mu        sync.RWMutex
}

// New creates a new memory backend
func New(cfg config.MemoryConfig) *Backend {
	return &Backend{
		cfg:      cfg,
		soldiers: make(map[uint16]*SoldierRecord),
		vehicles: make(map[uint16]*VehicleRecord),
		markers:  make(map[string]*MarkerRecord),
	}
}
