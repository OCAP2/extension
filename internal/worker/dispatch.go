package worker

import (
	"fmt"
	"time"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/convert"
)

// RegisterHandlers registers all event handlers with the dispatcher.
// This replaces the channel-based StartAsyncProcessors approach.
func (m *Manager) RegisterHandlers(d *dispatcher.Dispatcher) {
	// Entity creation - sync (need to cache before states arrive)
	d.Register(":NEW:SOLDIER:", m.handleNewSoldier)
	d.Register(":NEW:VEHICLE:", m.handleNewVehicle)

	// High-volume state updates - buffered
	d.Register(":NEW:SOLDIER:STATE:", m.handleSoldierState, dispatcher.Buffered(10000))
	d.Register(":NEW:VEHICLE:STATE:", m.handleVehicleState, dispatcher.Buffered(10000))

	// Combat events - buffered
	d.Register(":FIRED:", m.handleFiredEvent, dispatcher.Buffered(10000))
	d.Register(":PROJECTILE:", m.handleProjectileEvent, dispatcher.Buffered(5000))
	d.Register(":HIT:", m.handleHitEvent, dispatcher.Buffered(2000))
	d.Register(":KILL:", m.handleKillEvent, dispatcher.Buffered(2000))

	// General events - buffered
	d.Register(":EVENT:", m.handleGeneralEvent, dispatcher.Buffered(1000))
	d.Register(":CHAT:", m.handleChatEvent, dispatcher.Buffered(1000))
	d.Register(":RADIO:", m.handleRadioEvent, dispatcher.Buffered(1000))
	d.Register(":FPS:", m.handleFpsEvent, dispatcher.Buffered(1000))

	// ACE3 events - buffered
	d.Register(":ACE3:DEATH:", m.handleAce3DeathEvent, dispatcher.Buffered(1000))
	d.Register(":ACE3:UNCONSCIOUS:", m.handleAce3UnconsciousEvent, dispatcher.Buffered(1000))

	// Marker events - buffered
	d.Register(":MARKER:CREATE:", m.handleMarkerCreate, dispatcher.Buffered(500))
	d.Register(":MARKER:MOVE:", m.handleMarkerMove, dispatcher.Buffered(1000))
	d.Register(":MARKER:DELETE:", m.handleMarkerDelete, dispatcher.Buffered(500))
}

func (m *Manager) handleNewSoldier(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogNewSoldier(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new soldier: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.SoldierToCore(obj)
		m.backend.AddSoldier(&coreObj)
	} else {
		m.queues.Soldiers.Push([]model.Soldier{obj})
	}

	return "ok", nil
}

func (m *Manager) handleNewVehicle(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogNewVehicle(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new vehicle: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.VehicleToCore(obj)
		m.backend.AddVehicle(&coreObj)
	} else {
		m.queues.Vehicles.Push([]model.Vehicle{obj})
	}

	return "ok", nil
}

func (m *Manager) handleSoldierState(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogSoldierState(e.Args)
	if err != nil {
		// Silent skip for early state (before entity registered)
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.SoldierStateToCore(obj)
		m.backend.RecordSoldierState(&coreObj)
	} else {
		m.queues.SoldierStates.Push([]model.SoldierState{obj})
	}

	return nil, nil
}

func (m *Manager) handleVehicleState(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogVehicleState(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.VehicleStateToCore(obj)
		m.backend.RecordVehicleState(&coreObj)
	} else {
		m.queues.VehicleStates.Push([]model.VehicleState{obj})
	}

	return nil, nil
}

func (m *Manager) handleFiredEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogFiredEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.FiredEventToCore(obj)
		m.backend.RecordFiredEvent(&coreObj)
	} else {
		m.queues.FiredEvents.Push([]model.FiredEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleProjectileEvent(e dispatcher.Event) (any, error) {
	// Projectile events use linestringzm geo format, not supported by SQLite
	if !m.deps.IsDatabaseValid() || m.deps.ShouldSaveLocal() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogProjectileEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	m.queues.ProjectileEvents.Push([]model.ProjectileEvent{obj})
	return nil, nil
}

func (m *Manager) handleGeneralEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogGeneralEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log general event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.GeneralEventToCore(obj)
		m.backend.RecordGeneralEvent(&coreObj)
	} else {
		m.queues.GeneralEvents.Push([]model.GeneralEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleHitEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogHitEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.HitEventToCore(obj)
		m.backend.RecordHitEvent(&coreObj)
	} else {
		m.queues.HitEvents.Push([]model.HitEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleKillEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogKillEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.KillEventToCore(obj)
		m.backend.RecordKillEvent(&coreObj)
	} else {
		m.queues.KillEvents.Push([]model.KillEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleChatEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogChatEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.ChatEventToCore(obj)
		m.backend.RecordChatEvent(&coreObj)
	} else {
		m.queues.ChatEvents.Push([]model.ChatEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleRadioEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogRadioEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.RadioEventToCore(obj)
		m.backend.RecordRadioEvent(&coreObj)
	} else {
		m.queues.RadioEvents.Push([]model.RadioEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleFpsEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogFpsEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.ServerFpsEventToCore(obj)
		m.backend.RecordServerFpsEvent(&coreObj)
	} else {
		m.queues.FpsEvents.Push([]model.ServerFpsEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleAce3DeathEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogAce3DeathEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.Ace3DeathEventToCore(obj)
		m.backend.RecordAce3DeathEvent(&coreObj)
	} else {
		m.queues.Ace3DeathEvents.Push([]model.Ace3DeathEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleAce3UnconsciousEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogAce3UnconsciousEvent(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.Ace3UnconsciousEventToCore(obj)
		m.backend.RecordAce3UnconsciousEvent(&coreObj)
	} else {
		m.queues.Ace3UnconsciousEvents.Push([]model.Ace3UnconsciousEvent{obj})
	}

	return nil, nil
}

func (m *Manager) handleMarkerCreate(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	marker, err := m.deps.HandlerService.LogMarkerCreate(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create marker: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.MarkerToCore(marker)
		m.backend.AddMarker(&coreObj)
	} else {
		m.queues.Markers.Push([]model.Marker{marker})
	}

	return nil, nil
}

func (m *Manager) handleMarkerMove(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	markerState, err := m.deps.HandlerService.LogMarkerMove(e.Args)
	if err != nil {
		return nil, nil
	}

	if m.hasBackend() {
		coreObj := convert.MarkerStateToCore(markerState)
		m.backend.RecordMarkerState(&coreObj)
	} else {
		m.queues.MarkerStates.Push([]model.MarkerState{markerState})
	}

	return nil, nil
}

func (m *Manager) handleMarkerDelete(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() {
		return nil, nil
	}

	markerName, frameNo, err := m.deps.HandlerService.LogMarkerDelete(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to delete marker: %w", err)
	}

	markerID, ok := m.deps.MarkerCache.Get(markerName)
	if ok {
		deleteState := model.MarkerState{
			MissionID:    m.deps.HandlerService.GetMissionContext().GetMission().ID,
			MarkerID:     markerID,
			CaptureFrame: frameNo,
			Time:         time.Now(),
			Alpha:        0,
		}
		m.queues.MarkerStates.Push([]model.MarkerState{deleteState})
		if m.deps.DB != nil {
			m.deps.DB.Model(&model.Marker{}).Where("id = ?", markerID).Update("is_deleted", true)
		}
	}

	return nil, nil
}
