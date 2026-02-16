package worker

import (
	"database/sql"
	"fmt"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/convert"
	"github.com/OCAP2/extension/v5/internal/parser"

	"gorm.io/datatypes"
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

	coreObj := convert.SoldierToCore(obj)
	m.backend.AddSoldier(&coreObj)

	return nil, nil
}

func (m *Manager) handleNewVehicle(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseVehicle(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log new vehicle: %w", err)
	}

	// Always cache for state handler lookups
	m.deps.EntityCache.AddVehicle(obj)

	coreObj := convert.VehicleToCore(obj)
	m.backend.AddVehicle(&coreObj)

	return nil, nil
}

func (m *Manager) handleSoldierState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseSoldierState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log soldier state: %w", err)
	}

	// Validate soldier exists in cache; fill GroupID/Side if empty
	soldier, ok := m.deps.EntityCache.GetSoldier(obj.SoldierObjectID)
	if !ok {
		return nil, ErrTooEarlyForStateAssociation
	}
	if obj.GroupID == "" {
		obj.GroupID = soldier.GroupID
	}
	if obj.Side == "" {
		obj.Side = soldier.Side
	}

	coreObj := convert.SoldierStateToCore(obj)
	m.backend.RecordSoldierState(&coreObj)

	return nil, nil
}

func (m *Manager) handleVehicleState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseVehicleState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log vehicle state: %w", err)
	}

	// Validate vehicle exists in cache
	if _, ok := m.deps.EntityCache.GetVehicle(obj.VehicleObjectID); !ok {
		return nil, ErrTooEarlyForStateAssociation
	}

	coreObj := convert.VehicleStateToCore(obj)
	m.backend.RecordVehicleState(&coreObj)

	return nil, nil
}

func (m *Manager) handleTimeState(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseTimeState(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log time state: %w", err)
	}

	coreObj := convert.TimeStateToCore(obj)
	m.backend.RecordTimeState(&coreObj)

	return nil, nil
}

func (m *Manager) handleProjectileEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseProjectileEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log projectile event: %w", err)
	}
	m.classifyHitParts(&parsed)
	coreObj := convert.ProjectileEventToCore(parsed.Event)
	m.backend.RecordProjectileEvent(&coreObj)
	return nil, nil
}

// classifyHitParts classifies each RawHitPart as soldier or vehicle hit using EntityCache.
func (m *Manager) classifyHitParts(parsed *parser.ParsedProjectileEvent) {
	for _, hp := range parsed.HitParts {
		if _, ok := m.deps.EntityCache.GetSoldier(hp.EntityID); ok {
			parsed.Event.HitSoldiers = append(parsed.Event.HitSoldiers, model.ProjectileHitsSoldier{
				SoldierObjectID: hp.EntityID,
				CaptureFrame:    hp.CaptureFrame,
				Position:        hp.Position,
				ComponentsHit:   datatypes.JSON(hp.ComponentsHit),
			})
		} else if _, ok := m.deps.EntityCache.GetVehicle(hp.EntityID); ok {
			parsed.Event.HitVehicles = append(parsed.Event.HitVehicles, model.ProjectileHitsVehicle{
				VehicleObjectID: hp.EntityID,
				CaptureFrame:    hp.CaptureFrame,
				Position:        hp.Position,
				ComponentsHit:   datatypes.JSON(hp.ComponentsHit),
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

	coreObj := convert.GeneralEventToCore(obj)
	m.backend.RecordGeneralEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleKillEvent(e dispatcher.Event) (any, error) {
	parsed, err := m.deps.ParserService.ParseKillEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log kill event: %w", err)
	}

	// Classify victim as soldier or vehicle
	if _, ok := m.deps.EntityCache.GetSoldier(parsed.VictimID); ok {
		parsed.Event.VictimSoldierObjectID = sql.NullInt32{Int32: int32(parsed.VictimID), Valid: true}
	} else if _, ok := m.deps.EntityCache.GetVehicle(parsed.VictimID); ok {
		parsed.Event.VictimVehicleObjectID = sql.NullInt32{Int32: int32(parsed.VictimID), Valid: true}
	} else {
		m.deps.LogManager.Logger().Warn("Kill event victim not found in cache", "victimID", parsed.VictimID)
	}

	// Classify killer as soldier or vehicle
	if _, ok := m.deps.EntityCache.GetSoldier(parsed.KillerID); ok {
		parsed.Event.KillerSoldierObjectID = sql.NullInt32{Int32: int32(parsed.KillerID), Valid: true}
	} else if _, ok := m.deps.EntityCache.GetVehicle(parsed.KillerID); ok {
		parsed.Event.KillerVehicleObjectID = sql.NullInt32{Int32: int32(parsed.KillerID), Valid: true}
	} else {
		m.deps.LogManager.Logger().Warn("Kill event killer not found in cache", "killerID", parsed.KillerID)
	}

	obj := parsed.Event

	coreObj := convert.KillEventToCore(obj)
	m.backend.RecordKillEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleChatEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseChatEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log chat event: %w", err)
	}

	// Validate sender exists in cache if set
	if obj.SoldierObjectID.Valid {
		if _, ok := m.deps.EntityCache.GetSoldier(uint16(obj.SoldierObjectID.Int32)); !ok {
			return nil, fmt.Errorf("could not find soldier with ocap id %d for chat event", obj.SoldierObjectID.Int32)
		}
	}

	coreObj := convert.ChatEventToCore(obj)
	m.backend.RecordChatEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleRadioEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseRadioEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log radio event: %w", err)
	}

	// Validate sender exists in cache if set
	if obj.SoldierObjectID.Valid {
		if _, ok := m.deps.EntityCache.GetSoldier(uint16(obj.SoldierObjectID.Int32)); !ok {
			return nil, fmt.Errorf("could not find soldier with ocap id %d for radio event", obj.SoldierObjectID.Int32)
		}
	}

	coreObj := convert.RadioEventToCore(obj)
	m.backend.RecordRadioEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleFpsEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseFpsEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log fps event: %w", err)
	}

	coreObj := convert.ServerFpsEventToCore(obj)
	m.backend.RecordServerFpsEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleAce3DeathEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseAce3DeathEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 death event: %w", err)
	}

	// Validate soldier exists in cache
	if _, ok := m.deps.EntityCache.GetSoldier(obj.SoldierObjectID); !ok {
		return nil, fmt.Errorf("could not find soldier with ocap id %d for ace3 death event", obj.SoldierObjectID)
	}

	// Validate lastDamageSource exists in cache if set
	if obj.LastDamageSourceObjectID.Valid {
		sourceID := uint16(obj.LastDamageSourceObjectID.Int32)
		if _, ok := m.deps.EntityCache.GetSoldier(sourceID); !ok {
			if _, ok := m.deps.EntityCache.GetVehicle(sourceID); !ok {
				return nil, fmt.Errorf("could not find entity with ocap id %d for ace3 death last damage source", sourceID)
			}
		}
	}

	coreObj := convert.Ace3DeathEventToCore(obj)
	m.backend.RecordAce3DeathEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleAce3UnconsciousEvent(e dispatcher.Event) (any, error) {
	obj, err := m.deps.ParserService.ParseAce3UnconsciousEvent(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to log ace3 unconscious event: %w", err)
	}

	// Validate soldier exists in cache
	if _, ok := m.deps.EntityCache.GetSoldier(obj.SoldierObjectID); !ok {
		return nil, fmt.Errorf("could not find soldier with ocap id %d for ace3 unconscious event", obj.SoldierObjectID)
	}

	coreObj := convert.Ace3UnconsciousEventToCore(obj)
	m.backend.RecordAce3UnconsciousEvent(&coreObj)

	return nil, nil
}

func (m *Manager) handleMarkerCreate(e dispatcher.Event) (any, error) {
	marker, err := m.deps.ParserService.ParseMarkerCreate(e.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to create marker: %w", err)
	}

	coreObj := convert.MarkerToCore(marker)
	m.backend.AddMarker(&coreObj)
	// Cache the assigned ID so state updates can find this marker
	m.deps.MarkerCache.Set(marker.MarkerName, coreObj.ID)

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

	coreObj := convert.MarkerStateToCore(parsed.State)
	m.backend.RecordMarkerState(&coreObj)

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
