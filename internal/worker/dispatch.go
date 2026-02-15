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
	d.Register(":PROJECTILE:", m.handleProjectileEvent, dispatcher.Buffered(5000))
	d.Register(":KILL:", m.handleKillEvent, dispatcher.Buffered(2000))

	// General events - buffered
	d.Register(":EVENT:", m.handleGeneralEvent, dispatcher.Buffered(1000))
	d.Register(":CHAT:", m.handleChatEvent, dispatcher.Buffered(1000))
	d.Register(":RADIO:", m.handleRadioEvent, dispatcher.Buffered(1000))
	d.Register(":FPS:", m.handleFpsEvent, dispatcher.Buffered(1000))

	// ACE3 events - buffered
	d.Register(":ACE3:DEATH:", m.handleAce3DeathEvent, dispatcher.Buffered(1000))
	d.Register(":ACE3:UNCONSCIOUS:", m.handleAce3UnconsciousEvent, dispatcher.Buffered(1000))

	// Marker creation - sync (need to cache before states arrive)
	d.Register(":NEW:MARKER:", m.handleMarkerCreate)
	// Marker updates - buffered
	d.Register(":NEW:MARKER:STATE:", m.handleMarkerMove, dispatcher.Buffered(1000))
	d.Register(":DELETE:MARKER:", m.handleMarkerDelete, dispatcher.Buffered(500))
}

func (m *Manager) handleNewSoldier(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogNewSoldier(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new soldier: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddSoldier(obj)

	if m.hasBackend() {
		coreObj := convert.SoldierToCore(obj)
		m.backend.AddSoldier(&coreObj)
	} else {
		m.queues.Soldiers.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleNewVehicle(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogNewVehicle(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new vehicle: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddVehicle(obj)

	if m.hasBackend() {
		coreObj := convert.VehicleToCore(obj)
		m.backend.AddVehicle(&coreObj)
	} else {
		m.queues.Vehicles.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleSoldierState(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogSoldierState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log soldier state: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.SoldierStateToCore(obj)
		m.backend.RecordSoldierState(&coreObj)
	} else {
		m.queues.SoldierStates.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleVehicleState(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogVehicleState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log vehicle state: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.VehicleStateToCore(obj)
		m.backend.RecordVehicleState(&coreObj)
	} else {
		m.queues.VehicleStates.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleProjectileEvent(e dispatcher.Event) (any, error) {
	// For memory backend, convert projectile to appropriate format
	if m.hasBackend() {
		obj, err := m.deps.HandlerService.LogProjectileEvent(e.Args)
		if err != nil {
			return nil, fmt.Errorf("failed to log projectile event: %w", err)
		}

		// Thrown projectiles (grenades, smokes) become markers
		if obj.Weapon == "throw" {
			marker, states := convert.ProjectileEventToProjectileMarker(obj)
			m.backend.AddMarker(&marker) // AddMarker assigns a new ID
			for i := range states {
				states[i].MarkerID = marker.ID // sync with backend-assigned ID
				m.backend.RecordMarkerState(&states[i])
			}
		} else {
			// Other projectiles become fire lines
			coreObj := convert.ProjectileEventToFiredEvent(obj)
			m.backend.RecordFiredEvent(&coreObj)
		}

		// Record hit events from projectile hitParts
		hitEvents := convert.ProjectileEventToHitEvents(obj)
		for i := range hitEvents {
			m.backend.RecordHitEvent(&hitEvents[i])
		}

		return nil, nil
	}

	// Projectile events use linestringzm geo format, not supported by SQLite
	if !m.deps.IsDatabaseValid() || m.deps.ShouldSaveLocal() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogProjectileEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log projectile event: %w", err)
	}

	m.queues.ProjectileEvents.Push(obj)
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
		m.queues.GeneralEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleKillEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogKillEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log kill event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.KillEventToCore(obj)
		m.backend.RecordKillEvent(&coreObj)
	} else {
		m.queues.KillEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleChatEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogChatEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log chat event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.ChatEventToCore(obj)
		m.backend.RecordChatEvent(&coreObj)
	} else {
		m.queues.ChatEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleRadioEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogRadioEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log radio event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.RadioEventToCore(obj)
		m.backend.RecordRadioEvent(&coreObj)
	} else {
		m.queues.RadioEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleFpsEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogFpsEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log fps event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.ServerFpsEventToCore(obj)
		m.backend.RecordServerFpsEvent(&coreObj)
	} else {
		m.queues.FpsEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleAce3DeathEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogAce3DeathEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 death event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.Ace3DeathEventToCore(obj)
		m.backend.RecordAce3DeathEvent(&coreObj)
	} else {
		m.queues.Ace3DeathEvents.Push(obj)
	}

	return nil, nil
}

func (m *Manager) handleAce3UnconsciousEvent(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	obj, err := m.deps.HandlerService.LogAce3UnconsciousEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 unconscious event: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.Ace3UnconsciousEventToCore(obj)
		m.backend.RecordAce3UnconsciousEvent(&coreObj)
	} else {
		m.queues.Ace3UnconsciousEvents.Push(obj)
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
		// Cache the assigned ID so state updates can find this marker
		m.deps.MarkerCache.Set(marker.MarkerName, coreObj.ID)
	} else {
		m.queues.Markers.Push(marker)
	}

	return nil, nil
}

func (m *Manager) handleMarkerMove(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	markerState, err := m.deps.HandlerService.LogMarkerMove(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log marker move: %w", err)
	}

	if m.hasBackend() {
		coreObj := convert.MarkerStateToCore(markerState)
		m.backend.RecordMarkerState(&coreObj)
	} else {
		m.queues.MarkerStates.Push(markerState)
	}

	return nil, nil
}

func (m *Manager) handleMarkerDelete(e dispatcher.Event) (any, error) {
	if !m.deps.IsDatabaseValid() && !m.hasBackend() {
		return nil, nil
	}

	markerName, frameNo, err := m.deps.HandlerService.LogMarkerDelete(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to delete marker: %w", err)
	}

	if m.hasBackend() {
		m.backend.DeleteMarker(markerName, frameNo)
		return nil, nil
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
		m.queues.MarkerStates.Push(deleteState)
		if m.deps.DB != nil {
			m.deps.DB.Model(&model.Marker{}).Where("id = ?", markerID).Update("is_deleted", true)
		}
	}

	return nil, nil
}

