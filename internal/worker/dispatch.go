package worker

import (
	"fmt"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/pkg/core"
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
	d.Register(":TELEMETRY:", m.handleTelemetryEvent, dispatcher.Buffered(100), dispatcher.Logged())

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

	if err := m.backend.AddSoldier(&obj); err != nil {
		return nil, fmt.Errorf("add soldier: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleNewVehicle(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseVehicle(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new vehicle: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddVehicle(obj)

	if err := m.backend.AddVehicle(&obj); err != nil {
		return nil, fmt.Errorf("add vehicle: %w", err)
	}
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

	// Player takeover: update cached entity when isPlayer escalates or player name changes
	if obj.IsPlayer && (!soldier.IsPlayer || soldier.UnitName != obj.UnitName) {
		soldier.IsPlayer = true
		soldier.UnitName = obj.UnitName
		m.deps.EntityCache.UpdateSoldier(soldier)
	}

	if err := m.backend.RecordSoldierState(&obj); err != nil {
		return nil, fmt.Errorf("record soldier state: %w", err)
	}
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

	if err := m.backend.RecordVehicleState(&obj); err != nil {
		return nil, fmt.Errorf("record vehicle state: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleTimeState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseTimeState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log time state: %w", err)
	}

	if err := m.backend.RecordTimeState(&obj); err != nil {
		return nil, fmt.Errorf("record time state: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleProjectileEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseProjectileEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log projectile event: %w", err)
	}

	coreEvent := core.ProjectileEvent{
		CaptureFrame:    parsed.CaptureFrame,
		FirerObjectID:   parsed.FirerObjectID,
		VehicleObjectID: parsed.VehicleObjectID,
		WeaponDisplay:   parsed.WeaponDisplay,
		MagazineDisplay: parsed.MagazineDisplay,
		MuzzleDisplay:   parsed.MuzzleDisplay,
		SimulationType:  parsed.SimulationType,
		MagazineIcon:    parsed.MagazineIcon,
		Trajectory:      parsed.Trajectory,
		Hits:            m.classifyHitParts(parsed.HitParts),
	}

	if err := m.backend.RecordProjectileEvent(&coreEvent); err != nil {
		return nil, fmt.Errorf("record projectile event: %w", err)
	}
	return nil, nil
}

// classifyHitParts classifies each HitPart as soldier or vehicle hit using EntityCache.
func (m *Manager) classifyHitParts(hitParts []parser.HitPart) []core.ProjectileHit {
	var hits []core.ProjectileHit
	for _, hp := range hitParts {
		if _, ok := m.deps.EntityCache.GetSoldier(hp.EntityID); ok {
			soldierID := hp.EntityID
			hits = append(hits, core.ProjectileHit{
				SoldierID:     &soldierID,
				CaptureFrame:  hp.CaptureFrame,
				Position:      hp.Position,
				ComponentsHit: hp.ComponentsHit,
			})
		} else if _, ok := m.deps.EntityCache.GetVehicle(hp.EntityID); ok {
			vehicleID := hp.EntityID
			hits = append(hits, core.ProjectileHit{
				VehicleID:     &vehicleID,
				CaptureFrame:  hp.CaptureFrame,
				Position:      hp.Position,
				ComponentsHit: hp.ComponentsHit,
			})
		} else {
			m.deps.LogManager.Logger().Warn("Hit entity not found in cache", "hitEntityID", hp.EntityID)
		}
	}
	return hits
}

// classifyEntity looks up an entity ID in the cache and returns
// a soldier or vehicle pointer accordingly. Both are nil if not found.
func (m *Manager) classifyEntity(id uint16) (soldierID *uint, vehicleID *uint) {
	if _, ok := m.deps.EntityCache.GetSoldier(id); ok {
		v := uint(id)
		return &v, nil
	}
	if _, ok := m.deps.EntityCache.GetVehicle(id); ok {
		v := uint(id)
		return nil, &v
	}
	return nil, nil
}

func (m *Manager) handleGeneralEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseGeneralEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log general event: %w", err)
	}

	if err := m.backend.RecordGeneralEvent(&obj); err != nil {
		return nil, fmt.Errorf("record general event: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleKillEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseKillEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log kill event: %w", err)
	}

	coreEvent := core.KillEvent{
		Time:           parsed.Time,
		CaptureFrame:   parsed.CaptureFrame,
		WeaponVehicle:  parsed.WeaponVehicle,
		WeaponName:     parsed.WeaponName,
		WeaponMagazine: parsed.WeaponMagazine,
		EventText:      parsed.EventText,
		Distance:       parsed.Distance,
	}

	coreEvent.VictimSoldierID, coreEvent.VictimVehicleID = m.classifyEntity(parsed.VictimID)
	coreEvent.KillerSoldierID, coreEvent.KillerVehicleID = m.classifyEntity(parsed.KillerID)

	if err := m.backend.RecordKillEvent(&coreEvent); err != nil {
		return nil, fmt.Errorf("record kill event: %w", err)
	}
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

	if err := m.backend.RecordChatEvent(&obj); err != nil {
		return nil, fmt.Errorf("record chat event: %w", err)
	}
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

	if err := m.backend.RecordRadioEvent(&obj); err != nil {
		return nil, fmt.Errorf("record radio event: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleTelemetryEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseTelemetryEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to parse telemetry event: %w", err)
	}

	if err := m.backend.RecordTelemetryEvent(&obj); err != nil {
		return nil, fmt.Errorf("record telemetry event: %w", err)
	}
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

	if err := m.backend.RecordAce3DeathEvent(&obj); err != nil {
		return nil, fmt.Errorf("record ace3 death event: %w", err)
	}
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

	if err := m.backend.RecordAce3UnconsciousEvent(&obj); err != nil {
		return nil, fmt.Errorf("record ace3 unconscious event: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleMarkerCreate(e dispatcher.Event) (any, error) {
	marker, err := m.deps.ParserService.ParseMarkerCreate(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create marker: %w", err)
	}

	id, err := m.backend.AddMarker(&marker)
	if err != nil {
		return nil, fmt.Errorf("add marker: %w", err)
	}
	// Cache the assigned ID so state updates can find this marker
	m.deps.MarkerCache.Set(marker.MarkerName, id)

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

	coreState := core.MarkerState{
		MarkerID:     markerID,
		CaptureFrame: parsed.CaptureFrame,
		Position:     parsed.Position,
		Direction:    parsed.Direction,
		Alpha:        parsed.Alpha,
		Time:         parsed.Time,
	}

	if err := m.backend.RecordMarkerState(&coreState); err != nil {
		return nil, fmt.Errorf("record marker state: %w", err)
	}
	return nil, nil
}

func (m *Manager) handleMarkerDelete(e dispatcher.Event) (any, error) {
	dm, err := m.deps.ParserService.ParseMarkerDelete(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to delete marker: %w", err)
	}

	if err := m.backend.DeleteMarker(dm); err != nil {
		return nil, fmt.Errorf("delete marker: %w", err)
	}
	return nil, nil
}
