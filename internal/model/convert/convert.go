// Package convert provides functions to convert GORM models to core models
package convert

import (
	"encoding/json"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	geom "github.com/peterstace/simplefeatures/geom"
)

// pointToPosition3D converts a PostGIS geom.Point to a core.Position3D
func pointToPosition3D(p geom.Point) core.Position3D {
	coord, ok := p.Coordinates()
	if !ok {
		return core.Position3D{}
	}
	return core.Position3D{X: coord.XY.X, Y: coord.XY.Y, Z: coord.Z}
}

// lineStringToPolyline converts a geom.LineString to a core.Polyline
func lineStringToPolyline(ls geom.LineString) core.Polyline {
	seq := ls.Coordinates()
	if seq.Length() == 0 {
		return nil
	}
	polyline := make(core.Polyline, seq.Length())
	for i := 0; i < seq.Length(); i++ {
		pt := seq.GetXY(i)
		polyline[i] = core.Position2D{X: pt.X, Y: pt.Y}
	}
	return polyline
}

// SoldierToCore converts a GORM Soldier to a core.Soldier.
// GORM Soldier.ObjectID maps to core Soldier.ID.
func SoldierToCore(s model.Soldier) core.Soldier {
	var squadParams []any
	if len(s.SquadParams) > 0 {
		_ = json.Unmarshal(s.SquadParams, &squadParams)
	}

	return core.Soldier{
		ID:              s.ObjectID, // Core ID = GORM ObjectID
		MissionID:       s.MissionID,
		JoinTime:        s.JoinTime,
		JoinFrame:       s.JoinFrame,
		OcapType:        s.OcapType,
		UnitName:        s.UnitName,
		GroupID:         s.GroupID,
		Side:            s.Side,
		IsPlayer:        s.IsPlayer,
		RoleDescription: s.RoleDescription,
		ClassName:       s.ClassName,
		DisplayName:     s.DisplayName,
		PlayerUID:       s.PlayerUID,
		SquadParams:     squadParams,
	}
}

// SoldierStateToCore converts a GORM SoldierState to a core.SoldierState.
// SoldierObjectID in GORM maps directly to SoldierID in core (both are ObjectID).
func SoldierStateToCore(s model.SoldierState) core.SoldierState {
	var inVehicleObjID *uint16
	if s.InVehicleObjectID.Valid {
		id := uint16(s.InVehicleObjectID.Int32)
		inVehicleObjID = &id
	}

	return core.SoldierState{
		SoldierID:         s.SoldierObjectID, // Direct mapping: GORM SoldierObjectID = core SoldierID
		MissionID:         s.MissionID,
		Time:              s.Time,
		CaptureFrame:      s.CaptureFrame,
		Position:          pointToPosition3D(s.Position),
		Bearing:           s.Bearing,
		Lifestate:         s.Lifestate,
		InVehicle:         s.InVehicle,
		InVehicleObjectID: inVehicleObjID,
		VehicleRole:       s.VehicleRole,
		UnitName:          s.UnitName,
		IsPlayer:          s.IsPlayer,
		CurrentRole:       s.CurrentRole,
		HasStableVitals:   s.HasStableVitals,
		IsDraggedCarried:  s.IsDraggedCarried,
		Stance:            s.Stance,
		Scores: core.SoldierScores{
			InfantryKills: s.Scores.InfantryKills,
			VehicleKills:  s.Scores.VehicleKills,
			ArmorKills:    s.Scores.ArmorKills,
			AirKills:      s.Scores.AirKills,
			Deaths:        s.Scores.Deaths,
			TotalScore:    s.Scores.TotalScore,
		},
	}
}

// VehicleToCore converts a GORM Vehicle to a core.Vehicle.
// GORM Vehicle.ObjectID maps to core Vehicle.ID.
func VehicleToCore(v model.Vehicle) core.Vehicle {
	return core.Vehicle{
		ID:            v.ObjectID, // Core ID = GORM ObjectID
		MissionID:     v.MissionID,
		JoinTime:      v.JoinTime,
		JoinFrame:     v.JoinFrame,
		OcapType:      v.OcapType,
		ClassName:     v.ClassName,
		DisplayName:   v.DisplayName,
		Customization: v.Customization,
	}
}

// VehicleStateToCore converts a GORM VehicleState to a core.VehicleState.
// VehicleObjectID in GORM maps directly to VehicleID in core (both are ObjectID).
func VehicleStateToCore(v model.VehicleState) core.VehicleState {
	return core.VehicleState{
		VehicleID:       v.VehicleObjectID, // Direct mapping: GORM VehicleObjectID = core VehicleID
		MissionID:       v.MissionID,
		Time:            v.Time,
		CaptureFrame:    v.CaptureFrame,
		Position:        pointToPosition3D(v.Position),
		Bearing:         v.Bearing,
		IsAlive:         v.IsAlive,
		Crew:            v.Crew,
		Fuel:            v.Fuel,
		Damage:          v.Damage,
		Locked:          v.Locked,
		EngineOn:        v.EngineOn,
		Side:            v.Side,
		VectorDir:       v.VectorDir,
		VectorUp:        v.VectorUp,
		TurretAzimuth:   v.TurretAzimuth,
		TurretElevation: v.TurretElevation,
	}
}

// FiredEventToCore converts a GORM FiredEvent to a core.FiredEvent.
// SoldierObjectID in GORM maps directly to SoldierID in core (both are ObjectID).
func FiredEventToCore(e model.FiredEvent) core.FiredEvent {
	return core.FiredEvent{
		MissionID:    e.MissionID,
		SoldierID:    e.SoldierObjectID, // Direct mapping: GORM SoldierObjectID = core SoldierID
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		Weapon:       e.Weapon,
		Magazine:     e.Magazine,
		FiringMode:   e.FiringMode,
		StartPos:     pointToPosition3D(e.StartPosition),
		EndPos:       pointToPosition3D(e.EndPosition),
	}
}

// GeneralEventToCore converts a GORM GeneralEvent to a core.GeneralEvent
func GeneralEventToCore(e model.GeneralEvent) core.GeneralEvent {
	var extraData map[string]any
	if len(e.ExtraData) > 0 {
		_ = json.Unmarshal(e.ExtraData, &extraData)
	}

	return core.GeneralEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		Name:         e.Name,
		Message:      e.Message,
		ExtraData:    extraData,
	}
}

// HitEventToCore converts a GORM HitEvent to a core.HitEvent
// ObjectID fields in GORM map to uint IDs in core (cast to uint for compatibility)
func HitEventToCore(e model.HitEvent) core.HitEvent {
	result := core.HitEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		EventText:    e.EventText,
		Distance:     e.Distance,
	}

	if e.VictimSoldierObjectID.Valid {
		id := uint(e.VictimSoldierObjectID.Int32)
		result.VictimSoldierID = &id
	}
	if e.VictimVehicleObjectID.Valid {
		id := uint(e.VictimVehicleObjectID.Int32)
		result.VictimVehicleID = &id
	}
	if e.ShooterSoldierObjectID.Valid {
		id := uint(e.ShooterSoldierObjectID.Int32)
		result.ShooterSoldierID = &id
	}
	if e.ShooterVehicleObjectID.Valid {
		id := uint(e.ShooterVehicleObjectID.Int32)
		result.ShooterVehicleID = &id
	}

	return result
}

// KillEventToCore converts a GORM KillEvent to a core.KillEvent
// ObjectID fields in GORM map to uint IDs in core (cast to uint for compatibility)
func KillEventToCore(e model.KillEvent) core.KillEvent {
	result := core.KillEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		EventText:    e.EventText,
		Distance:     e.Distance,
	}

	if e.VictimSoldierObjectID.Valid {
		id := uint(e.VictimSoldierObjectID.Int32)
		result.VictimSoldierID = &id
	}
	if e.VictimVehicleObjectID.Valid {
		id := uint(e.VictimVehicleObjectID.Int32)
		result.VictimVehicleID = &id
	}
	if e.KillerSoldierObjectID.Valid {
		id := uint(e.KillerSoldierObjectID.Int32)
		result.KillerSoldierID = &id
	}
	if e.KillerVehicleObjectID.Valid {
		id := uint(e.KillerVehicleObjectID.Int32)
		result.KillerVehicleID = &id
	}

	return result
}

// ChatEventToCore converts a GORM ChatEvent to a core.ChatEvent
func ChatEventToCore(e model.ChatEvent) core.ChatEvent {
	result := core.ChatEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		Channel:      e.Channel,
		FromName:     e.FromName,
		SenderName:   e.SenderName,
		Message:      e.Message,
		PlayerUID:    e.PlayerUID,
	}

	if e.SoldierObjectID.Valid {
		id := uint(e.SoldierObjectID.Int32)
		result.SoldierID = &id
	}

	return result
}

// RadioEventToCore converts a GORM RadioEvent to a core.RadioEvent
func RadioEventToCore(e model.RadioEvent) core.RadioEvent {
	result := core.RadioEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		Radio:        e.Radio,
		RadioType:    e.RadioType,
		StartEnd:     e.StartEnd,
		Channel:      e.Channel,
		IsAdditional: e.IsAdditional,
		Frequency:    e.Frequency,
		Code:         e.Code,
	}

	if e.SoldierObjectID.Valid {
		id := uint(e.SoldierObjectID.Int32)
		result.SoldierID = &id
	}

	return result
}

// ServerFpsEventToCore converts a GORM ServerFpsEvent to a core.ServerFpsEvent
func ServerFpsEventToCore(e model.ServerFpsEvent) core.ServerFpsEvent {
	return core.ServerFpsEvent{
		MissionID:    e.MissionID,
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		FpsAverage:   e.FpsAverage,
		FpsMin:       e.FpsMin,
	}
}

// TimeStateToCore converts a GORM TimeState to a core.TimeState
func TimeStateToCore(t model.TimeState) core.TimeState {
	return core.TimeState{
		MissionID:      t.MissionID,
		Time:           t.Time,
		CaptureFrame:   t.CaptureFrame,
		SystemTimeUTC:  t.SystemTimeUTC,
		MissionDate:    t.MissionDate,
		TimeMultiplier: t.TimeMultiplier,
		MissionTime:    t.MissionTime,
	}
}

// Ace3DeathEventToCore converts a GORM Ace3DeathEvent to a core.Ace3DeathEvent
func Ace3DeathEventToCore(e model.Ace3DeathEvent) core.Ace3DeathEvent {
	result := core.Ace3DeathEvent{
		ID:           e.ID,
		MissionID:    e.MissionID,
		SoldierID:    uint(e.SoldierObjectID), // ObjectID -> uint for core model
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		Reason:       e.Reason,
	}

	if e.LastDamageSourceObjectID.Valid {
		id := uint(e.LastDamageSourceObjectID.Int32)
		result.LastDamageSourceID = &id
	}

	return result
}

// Ace3UnconsciousEventToCore converts a GORM Ace3UnconsciousEvent to a core.Ace3UnconsciousEvent
func Ace3UnconsciousEventToCore(e model.Ace3UnconsciousEvent) core.Ace3UnconsciousEvent {
	return core.Ace3UnconsciousEvent{
		ID:            e.ID,
		MissionID:    e.MissionID,
		SoldierID:    uint(e.SoldierObjectID), // ObjectID -> uint for core model
		Time:         e.Time,
		CaptureFrame: e.CaptureFrame,
		IsUnconscious: e.IsUnconscious,
	}
}

// MarkerToCore converts a GORM Marker to a core.Marker
func MarkerToCore(m model.Marker) core.Marker {
	return core.Marker{
		ID:           m.ID,
		MissionID:    m.MissionID,
		Time:         m.Time,
		CaptureFrame: m.CaptureFrame,
		MarkerName:   m.MarkerName,
		Direction:    m.Direction,
		MarkerType:   m.MarkerType,
		Text:         m.Text,
		OwnerID:      m.OwnerID,
		Color:        m.Color,
		Size:         m.Size,
		Side:         m.Side,
		Position:     pointToPosition3D(m.Position),
		Polyline:     lineStringToPolyline(m.Polyline),
		Shape:        m.Shape,
		Alpha:        m.Alpha,
		Brush:        m.Brush,
		IsDeleted:    m.IsDeleted,
	}
}

// MarkerStateToCore converts a GORM MarkerState to a core.MarkerState
func MarkerStateToCore(m model.MarkerState) core.MarkerState {
	return core.MarkerState{
		ID:           m.ID,
		MissionID:    m.MissionID,
		MarkerID:     m.MarkerID,
		Time:         m.Time,
		CaptureFrame: m.CaptureFrame,
		Position:     pointToPosition3D(m.Position),
		Direction:    m.Direction,
		Alpha:        m.Alpha,
	}
}

// ProjectileEventToFiredEvent converts a ProjectileEvent to a FiredEvent for the memory backend.
// This extracts start/end positions from the projectile trajectory for fireline rendering.
func ProjectileEventToFiredEvent(p model.ProjectileEvent) core.FiredEvent {
	var startPos, endPos core.Position3D

	// Extract positions from the LineStringZM geometry
	if !p.Positions.IsEmpty() {
		if ls, ok := p.Positions.AsLineString(); ok {
			seq := ls.Coordinates()
			if seq.Length() > 0 {
				// First point is start position
				start := seq.Get(0)
				startPos = core.Position3D{X: start.X, Y: start.Y, Z: start.Z}

				// Last point is end position
				end := seq.Get(seq.Length() - 1)
				endPos = core.Position3D{X: end.X, Y: end.Y, Z: end.Z}
			}
		}
	}

	return core.FiredEvent{
		MissionID:    p.MissionID,
		SoldierID:    p.FirerObjectID,
		Time:         p.Time,
		CaptureFrame: p.CaptureFrame,
		Weapon:       p.Weapon,
		Magazine:     p.Magazine,
		FiringMode:   p.Mode,
		StartPos:     startPos,
		EndPos:       endPos,
	}
}

// MissionToCore converts a GORM Mission to a core.Mission
func MissionToCore(m *model.Mission) core.Mission {
	addons := make([]core.Addon, 0, len(m.Addons))
	for _, a := range m.Addons {
		addons = append(addons, core.Addon{
			ID:         a.ID,
			Name:       a.Name,
			WorkshopID: a.WorkshopID,
		})
	}

	return core.Mission{
		ID:                           m.ID,
		MissionName:                  m.MissionName,
		BriefingName:                 m.BriefingName,
		MissionNameSource:            m.MissionNameSource,
		OnLoadName:                   m.OnLoadName,
		Author:                       m.Author,
		ServerName:                   m.ServerName,
		ServerProfile:                m.ServerProfile,
		StartTime:                    m.StartTime,
		WorldID:                      m.WorldID,
		CaptureDelay:                 m.CaptureDelay,
		AddonVersion:                 m.AddonVersion,
		ExtensionVersion:             m.ExtensionVersion,
		ExtensionBuild:               m.ExtensionBuild,
		OcapRecorderExtensionVersion: m.OcapRecorderExtensionVersion,
		Tag:                          m.Tag,
		PlayableSlots: core.PlayableSlots{
			West:        m.PlayableSlots.West,
			East:        m.PlayableSlots.East,
			Independent: m.PlayableSlots.Independent,
			Civilian:    m.PlayableSlots.Civilian,
			Logic:       m.PlayableSlots.Logic,
		},
		SideFriendly: core.SideFriendly{
			EastWest:        m.SideFriendly.EastWest,
			EastIndependent: m.SideFriendly.EastIndependent,
			WestIndependent: m.SideFriendly.WestIndependent,
		},
		Addons: addons,
	}
}

// WorldToCore converts a GORM World to a core.World
func WorldToCore(w *model.World) core.World {
	return core.World{
		ID:                w.ID,
		Author:            w.Author,
		WorkshopID:        w.WorkshopID,
		DisplayName:       w.DisplayName,
		WorldName:         w.WorldName,
		WorldNameOriginal: w.WorldNameOriginal,
		WorldSize:         w.WorldSize,
		Latitude:          w.Latitude,
		Longitude:         w.Longitude,
		Location:          pointToPosition3D(w.Location),
	}
}
