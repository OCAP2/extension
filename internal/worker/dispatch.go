package worker

import (
	"encoding/json"
	"fmt"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/parser"
)

// RegisterHandlers registers all event handlers with the dispatcher.
func (m *Manager) RegisterHandlers(d *dispatcher.Dispatcher) {
	// Entity creation - sync (need to cache before states arrive)
	d.Register(":NEW:SOLDIER:", m.handleNewSoldier, dispatcher.Logged())
	d.Register(":NEW:VEHICLE:", m.handleNewVehicle, dispatcher.Logged())

	// High-volume state updates - buffered
	d.Register(":NEW:SOLDIER:STATE:", m.handleSoldierState, dispatcher.Buffered(10000), dispatcher.Logged())
	d.Register(":NEW:VEHICLE:STATE:", m.handleVehicleState, dispatcher.Buffered(10000), dispatcher.Logged())

	// Time state tracking - buffered
	d.Register(":NEW:TIME:STATE:", m.handleTimeState, dispatcher.Buffered(100), dispatcher.Logged())

	// Combat events - buffered
	d.Register(":PROJECTILE:", m.handleProjectileEvent, dispatcher.Buffered(5000), dispatcher.Logged())
	d.Register(":KILL:", m.handleKillEvent, dispatcher.Buffered(2000), dispatcher.Logged())

	// General events - buffered
	d.Register(":EVENT:", m.handleGeneralEvent, dispatcher.Buffered(1000), dispatcher.Logged())
	d.Register(":CHAT:", m.handleChatEvent, dispatcher.Buffered(1000), dispatcher.Logged())
	d.Register(":RADIO:", m.handleRadioEvent, dispatcher.Buffered(1000), dispatcher.Logged())
	d.Register(":FPS:", m.handleFpsEvent, dispatcher.Buffered(1000), dispatcher.Logged())

	// ACE3 events - buffered
	d.Register(":ACE3:DEATH:", m.handleAce3DeathEvent, dispatcher.Buffered(1000), dispatcher.Logged())
	d.Register(":ACE3:UNCONSCIOUS:", m.handleAce3UnconsciousEvent, dispatcher.Buffered(1000), dispatcher.Logged())

	// Marker creation - sync (need to cache before states arrive)
	d.Register(":NEW:MARKER:", m.handleMarkerCreate, dispatcher.Logged())
	// Marker updates - buffered
	d.Register(":NEW:MARKER:STATE:", m.handleMarkerMove, dispatcher.Buffered(1000), dispatcher.Logged())
	d.Register(":DELETE:MARKER:", m.handleMarkerDelete, dispatcher.Buffered(500), dispatcher.Logged())
}

func (m *Manager) handleNewSoldier(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseSoldier(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new soldier: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddSoldier(obj)

	m.backend.AddSoldier(&obj)

	return nil, nil
}

func (m *Manager) handleNewVehicle(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseVehicle(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new vehicle: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddVehicle(obj)

	m.backend.AddVehicle(&obj)

	return nil, nil
}

func (m *Manager) handleSoldierState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseSoldierState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log soldier state: %w", err)
	}

	// Validate soldier exists in cache; fill GroupID/Side if empty
	soldier, ok := m.deps.EntityCache.GetSoldier(obj.SoldierID)
	if !ok {
		return nil, ErrTooEarlyForStateAssociation
	}
	if obj.GroupID == "" {
		obj.GroupID = soldier.GroupID
	}
	if obj.Side == "" {
		obj.Side = soldier.Side
	}

	m.backend.RecordSoldierState(&obj)

	return nil, nil
}

func (m *Manager) handleVehicleState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseVehicleState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log vehicle state: %w", err)
	}

	// Validate vehicle exists in cache
	if _, ok := m.deps.EntityCache.GetVehicle(obj.VehicleID); !ok {
		return nil, ErrTooEarlyForStateAssociation
	}

	m.backend.RecordVehicleState(&obj)

	return nil, nil
}

func (m *Manager) handleTimeState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseTimeState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log time state: %w", err)
	}

	m.backend.RecordTimeState(&obj)

	return nil, nil
}

func (m *Manager) handleProjectileEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseProjectileEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log projectile event: %w", err)
	}
	m.classifyHitParts(&parsed)
	m.backend.RecordProjectileEvent(&parsed.Event)
	return nil, nil
}

// classifyHitParts classifies each RawHitPart as soldier or vehicle hit using EntityCache.
func (m *Manager) classifyHitParts(parsed *parser.ParsedProjectileEvent) {
	for _, hp := range parsed.HitParts {
		if _, ok := m.deps.EntityCache.GetSoldier(hp.EntityID); ok {
			soldierID := hp.EntityID
			parsed.Event.Hits = append(parsed.Event.Hits, core.ProjectileHit{
				SoldierID:     &soldierID,
				CaptureFrame:  hp.CaptureFrame,
				Position:      hp.Position,
				ComponentsHit: json.RawMessage(hp.ComponentsHit),
			})
		} else if _, ok := m.deps.EntityCache.GetVehicle(hp.EntityID); ok {
			vehicleID := hp.EntityID
			parsed.Event.Hits = append(parsed.Event.Hits, core.ProjectileHit{
				VehicleID:     &vehicleID,
				CaptureFrame:  hp.CaptureFrame,
				Position:      hp.Position,
				ComponentsHit: json.RawMessage(hp.ComponentsHit),
			})
		} else {
			m.deps.LogManager.Logger().Warn("Hit entity not found in cache", "hitEntityID", hp.EntityID)
		}
	}
}

func (m *Manager) handleGeneralEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseGeneralEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log general event: %w", err)
	}

	m.backend.RecordGeneralEvent(&obj)

	return nil, nil
}

func (m *Manager) handleKillEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseKillEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log kill event: %w", err)
	}

	// Classify victim as soldier or vehicle
	if _, ok := m.deps.EntityCache.GetSoldier(parsed.VictimID); ok {
		victimID := uint(parsed.VictimID)
		parsed.Event.VictimSoldierID = &victimID
	} else if _, ok := m.deps.EntityCache.GetVehicle(parsed.VictimID); ok {
		victimID := uint(parsed.VictimID)
		parsed.Event.VictimVehicleID = &victimID
	} else {
		m.deps.LogManager.Logger().Warn("Kill event victim not found in cache", "victimID", parsed.VictimID)
	}

	// Classify killer as soldier or vehicle
	if _, ok := m.deps.EntityCache.GetSoldier(parsed.KillerID); ok {
		killerID := uint(parsed.KillerID)
		parsed.Event.KillerSoldierID = &killerID
	} else if _, ok := m.deps.EntityCache.GetVehicle(parsed.KillerID); ok {
		killerID := uint(parsed.KillerID)
		parsed.Event.KillerVehicleID = &killerID
	} else {
		m.deps.LogManager.Logger().Warn("Kill event killer not found in cache", "killerID", parsed.KillerID)
	}

	m.backend.RecordKillEvent(&parsed.Event)

	return nil, nil
}

func (m *Manager) handleChatEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseChatEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log chat event: %w", err)
	}

	// Validate sender exists in cache if set
	if obj.SoldierID != nil {
		if _, ok := m.deps.EntityCache.GetSoldier(uint16(*obj.SoldierID)); !ok {
			return nil, fmt.Errorf("could not find soldier with ocap id %d for chat event", *obj.SoldierID)
		}
	}

	m.backend.RecordChatEvent(&obj)

	return nil, nil
}

func (m *Manager) handleRadioEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseRadioEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log radio event: %w", err)
	}

	// Validate sender exists in cache if set
	if obj.SoldierID != nil {
		if _, ok := m.deps.EntityCache.GetSoldier(uint16(*obj.SoldierID)); !ok {
			return nil, fmt.Errorf("could not find soldier with ocap id %d for radio event", *obj.SoldierID)
		}
	}

	m.backend.RecordRadioEvent(&obj)

	return nil, nil
}

func (m *Manager) handleFpsEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseFpsEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log fps event: %w", err)
	}

	m.backend.RecordServerFpsEvent(&obj)

	return nil, nil
}

func (m *Manager) handleAce3DeathEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseAce3DeathEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 death event: %w", err)
	}

	// Validate soldier exists in cache
	if _, ok := m.deps.EntityCache.GetSoldier(uint16(obj.SoldierID)); !ok {
		return nil, fmt.Errorf("could not find soldier with ocap id %d for ace3 death event", obj.SoldierID)
	}

	// Validate lastDamageSource exists in cache if set
	if obj.LastDamageSourceID != nil {
		sourceID := uint16(*obj.LastDamageSourceID)
		if _, ok := m.deps.EntityCache.GetSoldier(sourceID); !ok {
			if _, ok := m.deps.EntityCache.GetVehicle(sourceID); !ok {
				return nil, fmt.Errorf("could not find entity with ocap id %d for ace3 death last damage source", sourceID)
			}
		}
	}

	m.backend.RecordAce3DeathEvent(&obj)

	return nil, nil
}

func (m *Manager) handleAce3UnconsciousEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseAce3UnconsciousEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 unconscious event: %w", err)
	}

	// Validate soldier exists in cache
	if _, ok := m.deps.EntityCache.GetSoldier(uint16(obj.SoldierID)); !ok {
		return nil, fmt.Errorf("could not find soldier with ocap id %d for ace3 unconscious event", obj.SoldierID)
	}

	m.backend.RecordAce3UnconsciousEvent(&obj)

	return nil, nil
}

func (m *Manager) handleMarkerCreate(e dispatcher.Event) (any, error) {
	marker, err := m.deps.ParserService.ParseMarkerCreate(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create marker: %w", err)
	}

	m.backend.AddMarker(&marker)
	// Cache the assigned ID so state updates can find this marker
	m.deps.MarkerCache.Set(marker.MarkerName, marker.ID)

	return nil, nil
}

func (m *Manager) handleMarkerMove(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseMarkerMove(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log marker move: %w", err)
	}

	// Resolve marker name to ID via cache
	markerID, ok := m.deps.MarkerCache.Get(parsed.MarkerName)
	if !ok {
		return nil, fmt.Errorf("marker %q not found in cache", parsed.MarkerName)
	}
	parsed.State.MarkerID = markerID

	m.backend.RecordMarkerState(&parsed.State)

	return nil, nil
}

func (m *Manager) handleMarkerDelete(e dispatcher.Event) (any, error) {
	markerName, frameNo, err := m.deps.ParserService.ParseMarkerDelete(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to delete marker: %w", err)
	}

	m.backend.DeleteMarker(markerName, frameNo)
	return nil, nil
}
