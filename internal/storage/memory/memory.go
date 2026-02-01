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

// Init initializes the backend
func (b *Backend) Init() error {
	return nil
}

// Close cleans up resources
func (b *Backend) Close() error {
	return nil
}

// StartMission begins recording a new mission
func (b *Backend) StartMission(mission *core.Mission, world *core.World) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.mission = mission
	b.world = world

	// Reset all collections
	b.soldiers = make(map[uint16]*SoldierRecord)
	b.vehicles = make(map[uint16]*VehicleRecord)
	b.markers = make(map[string]*MarkerRecord)
	b.generalEvents = nil
	b.hitEvents = nil
	b.killEvents = nil
	b.chatEvents = nil
	b.radioEvents = nil
	b.serverFpsEvents = nil
	b.ace3DeathEvents = nil
	b.ace3UnconsciousEvents = nil
	b.idCounter = 0

	return nil
}

// EndMission finalizes and exports the mission data
func (b *Backend) EndMission() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.exportJSON()
}

// exportJSON writes the mission data to a JSON file
func (b *Backend) exportJSON() error {
	// TODO: implement in Task 4.5
	return nil
}

// AddSoldier registers a new soldier
func (b *Backend) AddSoldier(s *core.Soldier) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.idCounter++
	s.ID = b.idCounter

	b.soldiers[s.OcapID] = &SoldierRecord{
		Soldier: *s,
		States:  make([]core.SoldierState, 0),
	}
	return nil
}

// AddVehicle registers a new vehicle
func (b *Backend) AddVehicle(v *core.Vehicle) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.idCounter++
	v.ID = b.idCounter

	b.vehicles[v.OcapID] = &VehicleRecord{
		Vehicle: *v,
		States:  make([]core.VehicleState, 0),
	}
	return nil
}

// AddMarker registers a new marker
func (b *Backend) AddMarker(m *core.Marker) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.idCounter++
	m.ID = b.idCounter

	b.markers[m.MarkerName] = &MarkerRecord{
		Marker: *m,
		States: make([]core.MarkerState, 0),
	}
	return nil
}

// GetSoldierByOcapID looks up a soldier by their OcapID
func (b *Backend) GetSoldierByOcapID(ocapID uint16) (*core.Soldier, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if record, ok := b.soldiers[ocapID]; ok {
		return &record.Soldier, true
	}
	return nil, false
}

// GetVehicleByOcapID looks up a vehicle by its OcapID
func (b *Backend) GetVehicleByOcapID(ocapID uint16) (*core.Vehicle, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if record, ok := b.vehicles[ocapID]; ok {
		return &record.Vehicle, true
	}
	return nil, false
}

// GetMarkerByName looks up a marker by name
func (b *Backend) GetMarkerByName(name string) (*core.Marker, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if record, ok := b.markers[name]; ok {
		return &record.Marker, true
	}
	return nil, false
}

// RecordSoldierState records a soldier state update
func (b *Backend) RecordSoldierState(s *core.SoldierState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find soldier by looking up via SoldierID
	for _, record := range b.soldiers {
		if record.Soldier.ID == s.SoldierID {
			record.States = append(record.States, *s)
			return nil
		}
	}
	return nil // silently ignore if soldier not found
}

// RecordVehicleState records a vehicle state update
func (b *Backend) RecordVehicleState(v *core.VehicleState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, record := range b.vehicles {
		if record.Vehicle.ID == v.VehicleID {
			record.States = append(record.States, *v)
			return nil
		}
	}
	return nil
}

// RecordMarkerState records a marker state update
func (b *Backend) RecordMarkerState(s *core.MarkerState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, record := range b.markers {
		if record.Marker.ID == s.MarkerID {
			record.States = append(record.States, *s)
			return nil
		}
	}
	return nil
}

// RecordFiredEvent records a fired event
func (b *Backend) RecordFiredEvent(e *core.FiredEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, record := range b.soldiers {
		if record.Soldier.ID == e.SoldierID {
			record.FiredEvents = append(record.FiredEvents, *e)
			return nil
		}
	}
	return nil
}

// RecordGeneralEvent records a general event
func (b *Backend) RecordGeneralEvent(e *core.GeneralEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.generalEvents = append(b.generalEvents, *e)
	return nil
}

// RecordHitEvent records a hit event
func (b *Backend) RecordHitEvent(e *core.HitEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hitEvents = append(b.hitEvents, *e)
	return nil
}

// RecordKillEvent records a kill event
func (b *Backend) RecordKillEvent(e *core.KillEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.killEvents = append(b.killEvents, *e)
	return nil
}

// RecordChatEvent records a chat event
func (b *Backend) RecordChatEvent(e *core.ChatEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chatEvents = append(b.chatEvents, *e)
	return nil
}

// RecordRadioEvent records a radio event
func (b *Backend) RecordRadioEvent(e *core.RadioEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.radioEvents = append(b.radioEvents, *e)
	return nil
}

// RecordServerFpsEvent records a server FPS event
func (b *Backend) RecordServerFpsEvent(e *core.ServerFpsEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.serverFpsEvents = append(b.serverFpsEvents, *e)
	return nil
}

// RecordAce3DeathEvent records an ACE3 death event
func (b *Backend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ace3DeathEvents = append(b.ace3DeathEvents, *e)
	return nil
}

// RecordAce3UnconsciousEvent records an ACE3 unconscious event
func (b *Backend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ace3UnconsciousEvents = append(b.ace3UnconsciousEvents, *e)
	return nil
}
