// internal/storage/memory/memory.go
package memory

import (
	"fmt"
	"sync"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/pkg/core"
	v1 "github.com/OCAP2/extension/v5/internal/storage/memory/export/v1"
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

	lastExportPath     string           // path to the last exported file
	lastExportMetadata core.UploadMetadata // cached metadata from last export

	soldiers      map[uint16]*SoldierRecord // keyed by ObjectID
	vehicles      map[uint16]*VehicleRecord // keyed by ObjectID
	markers       map[string]*MarkerRecord  // keyed by MarkerName
	markersByID   map[uint]*MarkerRecord   // keyed by Marker.ID
	nextMarkerID  uint                      // auto-increment ID for markers

	generalEvents         []core.GeneralEvent
	hitEvents             []core.HitEvent
	killEvents            []core.KillEvent
	chatEvents            []core.ChatEvent
	radioEvents           []core.RadioEvent
	serverFpsEvents       []core.ServerFpsEvent
	telemetryEvents       []core.TelemetryEvent
	timeStates            []core.TimeState
	ace3DeathEvents       []core.Ace3DeathEvent
	ace3UnconsciousEvents []core.Ace3UnconsciousEvent
	projectileEvents      []core.ProjectileEvent

	mu sync.RWMutex
}

// New creates a new memory backend
func New(cfg config.MemoryConfig) *Backend {
	return &Backend{
		cfg:      cfg,
		soldiers:    make(map[uint16]*SoldierRecord),
		vehicles:    make(map[uint16]*VehicleRecord),
		markers:     make(map[string]*MarkerRecord),
		markersByID: make(map[uint]*MarkerRecord),
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
	b.lastExportPath = ""
	b.resetCollections()

	return nil
}

// EndMission finalizes and exports the mission data
func (b *Backend) EndMission() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mission == nil {
		return fmt.Errorf("no mission to end: mission was never started")
	}

	// Cache export metadata before clearing data (needed for upload after export)
	b.lastExportMetadata = b.computeExportMetadata()

	if err := b.exportJSON(); err != nil {
		return err
	}

	// Clear all recorded data so subsequent recordings within the same
	// mission start fresh (e.g. manual start/stop recording in Liberation).
	b.mission = nil
	b.world = nil
	b.resetCollections()

	return nil
}

// resetCollections clears all entity and event data.
// Caller must hold b.mu.Lock.
func (b *Backend) resetCollections() {
	b.soldiers = make(map[uint16]*SoldierRecord)
	b.vehicles = make(map[uint16]*VehicleRecord)
	b.markers = make(map[string]*MarkerRecord)
	b.markersByID = make(map[uint]*MarkerRecord)
	b.nextMarkerID = 0
	b.generalEvents = nil
	b.hitEvents = nil
	b.killEvents = nil
	b.chatEvents = nil
	b.radioEvents = nil
	b.serverFpsEvents = nil
	b.telemetryEvents = nil
	b.timeStates = nil
	b.ace3DeathEvents = nil
	b.ace3UnconsciousEvents = nil
	b.projectileEvents = nil
}

// AddSoldier registers a new soldier.
// The soldier's ID is their ObjectID (game identifier).
func (b *Backend) AddSoldier(s *core.Soldier) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// ID is the ObjectID, set by caller
	b.soldiers[s.ID] = &SoldierRecord{
		Soldier: *s,
		States:  make([]core.SoldierState, 0),
	}
	return nil
}

// AddVehicle registers a new vehicle.
// The vehicle's ID is their ObjectID (game identifier).
func (b *Backend) AddVehicle(v *core.Vehicle) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// ID is the ObjectID, set by caller
	b.vehicles[v.ID] = &VehicleRecord{
		Vehicle: *v,
		States:  make([]core.VehicleState, 0),
	}
	return nil
}

// AddMarker registers a new marker.
// Assigns an auto-increment ID so marker state updates can reference it.
// Returns the assigned ID.
func (b *Backend) AddMarker(m *core.Marker) (uint, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextMarkerID++
	id := b.nextMarkerID

	markerCopy := *m
	markerCopy.ID = id

	record := &MarkerRecord{
		Marker: markerCopy,
		States: make([]core.MarkerState, 0),
	}
	b.markers[m.MarkerName] = record
	b.markersByID[id] = record
	return id, nil
}

// RecordSoldierState records a soldier state update.
// SoldierID must be set to the soldier's ObjectID.
func (b *Backend) RecordSoldierState(s *core.SoldierState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find soldier by ObjectID (SoldierState.SoldierID is the ObjectID)
	if record, ok := b.soldiers[s.SoldierID]; ok {
		record.States = append(record.States, *s)
	}
	return nil
}

// RecordVehicleState records a vehicle state update.
// VehicleID must be set to the vehicle's ObjectID.
func (b *Backend) RecordVehicleState(v *core.VehicleState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find vehicle by ObjectID (VehicleState.VehicleID is the ObjectID)
	if record, ok := b.vehicles[v.VehicleID]; ok {
		record.States = append(record.States, *v)
	}
	return nil
}

// RecordMarkerState records a marker state update
func (b *Backend) RecordMarkerState(s *core.MarkerState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if record, ok := b.markersByID[s.MarkerID]; ok {
		record.States = append(record.States, *s)
	}
	return nil
}

// DeleteMarker sets the end frame for a marker, marking it as deleted at that frame
func (b *Backend) DeleteMarker(dm *core.DeleteMarker) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if record, ok := b.markers[dm.Name]; ok {
		record.Marker.EndFrame = int(dm.EndFrame)
	}
	return nil
}

// RecordFiredEvent records a fired event.
// SoldierID must be set to the soldier's ObjectID.
func (b *Backend) RecordFiredEvent(e *core.FiredEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find soldier by ObjectID (FiredEvent.SoldierID is the ObjectID)
	if record, ok := b.soldiers[e.SoldierID]; ok {
		record.FiredEvents = append(record.FiredEvents, *e)
	}
	return nil
}

// RecordProjectileEvent records a raw projectile event
func (b *Backend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.projectileEvents = append(b.projectileEvents, *e)
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

// RecordTelemetryEvent records a telemetry event and extracts FPS data.
func (b *Backend) RecordTelemetryEvent(e *core.TelemetryEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.telemetryEvents = append(b.telemetryEvents, *e)
	b.serverFpsEvents = append(b.serverFpsEvents, core.ServerFpsEvent{
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		FpsAverage:   e.FpsAverage,
		FpsMin:       e.FpsMin,
	})
	return nil
}

// RecordTimeState records a time synchronization state
func (b *Backend) RecordTimeState(t *core.TimeState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.timeStates = append(b.timeStates, *t)
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

// GetExportedFilePath returns the path to the last exported file.
func (b *Backend) GetExportedFilePath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastExportPath
}

// GetExportMetadata returns metadata about the last export.
// If a mission is active (before EndMission), computes metadata from live data.
// After EndMission, returns cached metadata from the export.
func (b *Backend) GetExportMetadata() core.UploadMetadata {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.mission != nil && b.world != nil {
		return b.computeExportMetadata()
	}
	return b.lastExportMetadata
}

// computeExportMetadata builds upload metadata from the current in-memory data.
// Caller must hold at least b.mu.RLock.
func (b *Backend) computeExportMetadata() core.UploadMetadata {
	if b.mission == nil || b.world == nil {
		return core.UploadMetadata{}
	}

	var endFrame uint
	for _, record := range b.soldiers {
		for _, state := range record.States {
			if state.CaptureFrame > endFrame {
				endFrame = state.CaptureFrame
			}
		}
	}
	for _, record := range b.vehicles {
		for _, state := range record.States {
			if state.CaptureFrame > endFrame {
				endFrame = state.CaptureFrame
			}
		}
	}

	duration := float64(endFrame) * float64(b.mission.CaptureDelay) / 1000.0

	return core.UploadMetadata{
		WorldName:       b.world.WorldName,
		MissionName:     b.mission.MissionName,
		MissionDuration: duration,
		Tag:             b.mission.Tag,
	}
}

// BuildExport creates a v1 export from the current mission data.
// This is safe for concurrent use.
func (b *Backend) BuildExport() v1.Export {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.buildExportUnlocked()
}

// buildExportUnlocked creates a v1 export from the current mission data.
// Caller must hold at least b.mu.RLock.
func (b *Backend) buildExportUnlocked() v1.Export {
	data := &v1.MissionData{
		Mission:          b.mission,
		World:            b.world,
		Soldiers:         make(map[uint16]*v1.SoldierRecord),
		Vehicles:         make(map[uint16]*v1.VehicleRecord),
		Markers:          make(map[string]*v1.MarkerRecord),
		GeneralEvents:    b.generalEvents,
		HitEvents:        b.hitEvents,
		KillEvents:       b.killEvents,
		TimeStates:       b.timeStates,
		ProjectileEvents: b.projectileEvents,
	}

	for id, record := range b.soldiers {
		data.Soldiers[id] = &v1.SoldierRecord{
			Soldier:     record.Soldier,
			States:      record.States,
			FiredEvents: record.FiredEvents,
		}
	}

	for id, record := range b.vehicles {
		data.Vehicles[id] = &v1.VehicleRecord{
			Vehicle: record.Vehicle,
			States:  record.States,
		}
	}

	for name, record := range b.markers {
		data.Markers[name] = &v1.MarkerRecord{
			Marker: record.Marker,
			States: record.States,
		}
	}

	return v1.Build(data)
}
