// Package convert provides functions to convert between GORM models and core models
package convert

import (
	"database/sql"
	"encoding/json"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/pkg/core"
	geom "github.com/peterstace/simplefeatures/geom"
	"gorm.io/datatypes"
)

// position3DToPoint converts a core.Position3D to a PostGIS geom.Point
func position3DToPoint(p core.Position3D) geom.Point {
	coords := geom.Coordinates{XY: geom.XY{X: p.X, Y: p.Y}, Z: p.Z}
	return geom.NewPoint(coords)
}

// polylineToLineString converts a core.Polyline to a geom.LineString
func polylineToLineString(p core.Polyline) geom.LineString {
	if len(p) == 0 {
		return geom.LineString{}
	}
	coords := make([]float64, 0, len(p)*2)
	for _, pt := range p {
		coords = append(coords, pt.X, pt.Y)
	}
	seq := geom.NewSequence(coords, geom.DimXY)
	return geom.NewLineString(seq)
}

// componentsToJSON converts a []string to datatypes.JSON for DB storage.
func componentsToJSON(components []string) datatypes.JSON {
	if len(components) == 0 {
		return datatypes.JSON("[]")
	}
	data, _ := json.Marshal(components)
	return datatypes.JSON(data)
}

// CoreToSoldier converts a core.Soldier to a GORM model.Soldier.
// core.Soldier.ID maps to GORM Soldier.ObjectID.
func CoreToSoldier(s core.Soldier) model.Soldier {
	var squadParams datatypes.JSON
	if len(s.SquadParams) > 0 {
		squadParams, _ = json.Marshal(s.SquadParams)
	} else {
		squadParams = datatypes.JSON("[]")
	}

	return model.Soldier{
		ObjectID:        s.ID,
		JoinTime:        s.JoinTime,
		JoinFrame:       uint(s.JoinFrame),
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

// CoreToVehicle converts a core.Vehicle to a GORM model.Vehicle.
// core.Vehicle.ID maps to GORM Vehicle.ObjectID.
func CoreToVehicle(v core.Vehicle) model.Vehicle {
	return model.Vehicle{
		ObjectID:      v.ID,
		JoinTime:      v.JoinTime,
		JoinFrame:     uint(v.JoinFrame),
		OcapType:      v.OcapType,
		ClassName:     v.ClassName,
		DisplayName:   v.DisplayName,
		Customization: v.Customization,
	}
}

// CoreToMarker converts a core.Marker to a GORM model.Marker.
func CoreToMarker(m core.Marker) model.Marker {
	return model.Marker{
		ID:           m.ID,
		Time:         m.Time,
		CaptureFrame: uint(m.CaptureFrame),
		MarkerName:   m.MarkerName,
		Direction:    m.Direction,
		MarkerType:   m.MarkerType,
		Text:         m.Text,
		OwnerID:      m.OwnerID,
		Color:        m.Color,
		Size:         m.Size,
		Side:         m.Side,
		Position:     position3DToPoint(m.Position),
		Polyline:     polylineToLineString(m.Polyline),
		Shape:        m.Shape,
		Alpha:        m.Alpha,
		Brush:        m.Brush,
		IsDeleted:    m.IsDeleted,
	}
}

// CoreToSoldierState converts a core.SoldierState to a GORM model.SoldierState.
func CoreToSoldierState(s core.SoldierState) model.SoldierState {
	var inVehicleObjID sql.NullInt32
	if s.InVehicleObjectID != nil {
		inVehicleObjID = sql.NullInt32{Int32: int32(*s.InVehicleObjectID), Valid: true}
	}

	return model.SoldierState{
		SoldierObjectID:  s.SoldierID,
		Time:             s.Time,
		CaptureFrame:     uint(s.CaptureFrame),
		Position:         position3DToPoint(s.Position),
		ElevationASL:     float32(s.Position.Z),
		Bearing:          s.Bearing,
		Lifestate:        s.Lifestate,
		InVehicle:        s.InVehicle,
		InVehicleObjectID: inVehicleObjID,
		VehicleRole:      s.VehicleRole,
		UnitName:         s.UnitName,
		IsPlayer:         s.IsPlayer,
		CurrentRole:      s.CurrentRole,
		HasStableVitals:  s.HasStableVitals,
		IsDraggedCarried: s.IsDraggedCarried,
		Stance:           s.Stance,
		GroupID:          s.GroupID,
		Side:             s.Side,
		Scores: model.SoldierScores{
			InfantryKills: s.Scores.InfantryKills,
			VehicleKills:  s.Scores.VehicleKills,
			ArmorKills:    s.Scores.ArmorKills,
			AirKills:      s.Scores.AirKills,
			Deaths:        s.Scores.Deaths,
			TotalScore:    s.Scores.TotalScore,
		},
	}
}

// CoreToVehicleState converts a core.VehicleState to a GORM model.VehicleState.
func CoreToVehicleState(v core.VehicleState) model.VehicleState {
	return model.VehicleState{
		VehicleObjectID: v.VehicleID,
		Time:            v.Time,
		CaptureFrame:    uint(v.CaptureFrame),
		Position:        position3DToPoint(v.Position),
		ElevationASL:    float32(v.Position.Z),
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

// CoreToMarkerState converts a core.MarkerState to a GORM model.MarkerState.
func CoreToMarkerState(s core.MarkerState) model.MarkerState {
	return model.MarkerState{
		ID:           s.ID,
		MarkerID:     s.MarkerID,
		Time:         s.Time,
		CaptureFrame: uint(s.CaptureFrame),
		Position:     position3DToPoint(s.Position),
		Direction:    s.Direction,
		Alpha:        s.Alpha,
	}
}

// CoreToGeneralEvent converts a core.GeneralEvent to a GORM model.GeneralEvent.
func CoreToGeneralEvent(e core.GeneralEvent) model.GeneralEvent {
	var extraData datatypes.JSON
	if len(e.ExtraData) > 0 {
		extraData, _ = json.Marshal(e.ExtraData)
	} else {
		extraData = datatypes.JSON("{}")
	}

	return model.GeneralEvent{
		ID:           e.ID,
		Time:         e.Time,
		CaptureFrame: uint(e.CaptureFrame),
		Name:         e.Name,
		Message:      e.Message,
		ExtraData:    extraData,
	}
}

// CoreToKillEvent converts a core.KillEvent to a GORM model.KillEvent.
func CoreToKillEvent(e core.KillEvent) model.KillEvent {
	result := model.KillEvent{
		ID:             e.ID,
		Time:           e.Time,
		CaptureFrame:   uint(e.CaptureFrame),
		WeaponVehicle:  e.WeaponVehicle,
		WeaponName:     e.WeaponName,
		WeaponMagazine: e.WeaponMagazine,
		EventText:      e.EventText,
		Distance:       e.Distance,
	}

	if e.VictimSoldierID != nil {
		result.VictimSoldierObjectID = sql.NullInt32{Int32: int32(*e.VictimSoldierID), Valid: true}
	}
	if e.VictimVehicleID != nil {
		result.VictimVehicleObjectID = sql.NullInt32{Int32: int32(*e.VictimVehicleID), Valid: true}
	}
	if e.KillerSoldierID != nil {
		result.KillerSoldierObjectID = sql.NullInt32{Int32: int32(*e.KillerSoldierID), Valid: true}
	}
	if e.KillerVehicleID != nil {
		result.KillerVehicleObjectID = sql.NullInt32{Int32: int32(*e.KillerVehicleID), Valid: true}
	}

	return result
}

// CoreToChatEvent converts a core.ChatEvent to a GORM model.ChatEvent.
func CoreToChatEvent(e core.ChatEvent) model.ChatEvent {
	result := model.ChatEvent{
		ID:           e.ID,
		Time:         e.Time,
		CaptureFrame: uint(e.CaptureFrame),
		Channel:      e.Channel,
		FromName:     e.FromName,
		SenderName:   e.SenderName,
		Message:      e.Message,
		PlayerUID:    e.PlayerUID,
	}

	if e.SoldierID != nil {
		result.SoldierObjectID = sql.NullInt32{Int32: int32(*e.SoldierID), Valid: true}
	}

	return result
}

// CoreToRadioEvent converts a core.RadioEvent to a GORM model.RadioEvent.
func CoreToRadioEvent(e core.RadioEvent) model.RadioEvent {
	result := model.RadioEvent{
		ID:           e.ID,
		Time:         e.Time,
		CaptureFrame: uint(e.CaptureFrame),
		Radio:        e.Radio,
		RadioType:    e.RadioType,
		StartEnd:     e.StartEnd,
		Channel:      e.Channel,
		IsAdditional: e.IsAdditional,
		Frequency:    e.Frequency,
		Code:         e.Code,
	}

	if e.SoldierID != nil {
		result.SoldierObjectID = sql.NullInt32{Int32: int32(*e.SoldierID), Valid: true}
	}

	return result
}

// CoreToAce3DeathEvent converts a core.Ace3DeathEvent to a GORM model.Ace3DeathEvent.
func CoreToAce3DeathEvent(e core.Ace3DeathEvent) model.Ace3DeathEvent {
	result := model.Ace3DeathEvent{
		ID:              e.ID,
		SoldierObjectID: uint16(e.SoldierID),
		Time:            e.Time,
		CaptureFrame:    uint(e.CaptureFrame),
		Reason:          e.Reason,
	}

	if e.LastDamageSourceID != nil {
		result.LastDamageSourceObjectID = sql.NullInt32{Int32: int32(*e.LastDamageSourceID), Valid: true}
	}

	return result
}

// CoreToAce3UnconsciousEvent converts a core.Ace3UnconsciousEvent to a GORM model.Ace3UnconsciousEvent.
func CoreToAce3UnconsciousEvent(e core.Ace3UnconsciousEvent) model.Ace3UnconsciousEvent {
	return model.Ace3UnconsciousEvent{
		ID:              e.ID,
		SoldierObjectID: uint16(e.SoldierID),
		Time:            e.Time,
		CaptureFrame:    uint(e.CaptureFrame),
		IsUnconscious:   e.IsUnconscious,
	}
}

// CoreToPlacedObject converts a core.PlacedObject to a GORM model.PlacedObject.
func CoreToPlacedObject(p core.PlacedObject) model.PlacedObject {
	return model.PlacedObject{
		ObjectID:     p.ID,
		JoinTime:     p.JoinTime,
		JoinFrame:    uint(p.JoinFrame),
		ClassName:    p.ClassName,
		DisplayName:  p.DisplayName,
		PositionX:    p.Position.X,
		PositionY:    p.Position.Y,
		PositionZ:    p.Position.Z,
		OwnerID:      p.OwnerID,
		Side:         p.Side,
		Weapon:       p.Weapon,
		MagazineIcon: p.MagazineIcon,
	}
}

// CoreToPlacedObjectEvent converts a core.PlacedObjectEvent to a GORM model.PlacedObjectEvent.
func CoreToPlacedObjectEvent(e core.PlacedObjectEvent) model.PlacedObjectEvent {
	result := model.PlacedObjectEvent{
		PlacedObjectID: e.PlacedID,
		EventType:      e.EventType,
		PositionX:      e.Position.X,
		PositionY:      e.Position.Y,
		PositionZ:      e.Position.Z,
		CaptureFrame:   uint(e.CaptureFrame),
	}
	if e.HitEntityID != nil {
		id := uint(*e.HitEntityID)
		result.HitEntityID = &id
	}
	return result
}

// CoreToProjectileEvent converts a core.ProjectileEvent to a GORM model.ProjectileEvent.
// Converts trajectory points to a LineStringZM geometry and splits unified Hits
// into separate HitSoldiers and HitVehicles slices.
func CoreToProjectileEvent(e core.ProjectileEvent) model.ProjectileEvent {
	result := model.ProjectileEvent{
		CaptureFrame:    uint(e.CaptureFrame),
		FirerObjectID:   e.FirerObjectID,
		WeaponDisplay:   e.WeaponDisplay,
		MagazineDisplay: e.MagazineDisplay,
		MuzzleDisplay:   e.MuzzleDisplay,
		SimulationType:  e.SimulationType,
		MagazineIcon:    e.MagazineIcon,
	}

	// Convert VehicleObjectID
	if e.VehicleObjectID != nil {
		result.VehicleObjectID = sql.NullInt32{Int32: int32(*e.VehicleObjectID), Valid: true}
	}

	// Convert TrajectoryPoints → LineStringZM
	if len(e.Trajectory) >= 2 {
		coords := make([]float64, 0, len(e.Trajectory)*4)
		for _, tp := range e.Trajectory {
			coords = append(coords, tp.Position.X, tp.Position.Y, tp.Position.Z, float64(tp.FrameNum))
		}
		seq := geom.NewSequence(coords, geom.DimXYZM)
		ls := geom.NewLineString(seq)
		result.Positions = ls.AsGeometry()
	}

	// Split unified Hits → HitSoldiers + HitVehicles
	for _, hit := range e.Hits {
		if hit.SoldierID != nil {
			result.HitSoldiers = append(result.HitSoldiers, model.ProjectileHitsSoldier{
				SoldierObjectID: *hit.SoldierID,
				CaptureFrame:    uint(hit.CaptureFrame),
				Position:        position3DToPoint(hit.Position),
				ComponentsHit:   componentsToJSON(hit.ComponentsHit),
			})
		}
		if hit.VehicleID != nil {
			result.HitVehicles = append(result.HitVehicles, model.ProjectileHitsVehicle{
				VehicleObjectID: *hit.VehicleID,
				CaptureFrame:    uint(hit.CaptureFrame),
				Position:        position3DToPoint(hit.Position),
				ComponentsHit:   componentsToJSON(hit.ComponentsHit),
			})
		}
	}

	return result
}

// CoreToMission converts a core.Mission to a GORM model.Mission.
func CoreToMission(m core.Mission) model.Mission {
	addons := make([]model.Addon, 0, len(m.Addons))
	for _, a := range m.Addons {
		addons = append(addons, model.Addon{
			Name:       a.Name,
			WorkshopID: a.WorkshopID,
		})
	}

	return model.Mission{
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
		Addons:                       addons,
		PlayableSlots: model.PlayableSlots{
			West:        m.PlayableSlots.West,
			East:        m.PlayableSlots.East,
			Independent: m.PlayableSlots.Independent,
			Civilian:    m.PlayableSlots.Civilian,
			Logic:       m.PlayableSlots.Logic,
		},
		SideFriendly: model.SideFriendly{
			EastWest:        m.SideFriendly.EastWest,
			EastIndependent: m.SideFriendly.EastIndependent,
			WestIndependent: m.SideFriendly.WestIndependent,
		},
	}
}

// CoreToWorld converts a core.World to a GORM model.World.
func CoreToWorld(w core.World) model.World {
	return model.World{
		Author:            w.Author,
		WorkshopID:        w.WorkshopID,
		DisplayName:       w.DisplayName,
		WorldName:         w.WorldName,
		WorldNameOriginal: w.WorldNameOriginal,
		WorldSize:         w.WorldSize,
		Latitude:          w.Latitude,
		Longitude:         w.Longitude,
		Location:          position3DToPoint(w.Location),
	}
}

// CoreToTimeState converts a core.TimeState to a GORM model.TimeState.
func CoreToTimeState(t core.TimeState) model.TimeState {
	return model.TimeState{
		Time:           t.Time,
		CaptureFrame:   uint(t.CaptureFrame),
		SystemTimeUTC:  t.SystemTimeUTC,
		MissionDate:    t.MissionDate,
		TimeMultiplier: t.TimeMultiplier,
		MissionTime:    t.MissionTime,
	}
}
