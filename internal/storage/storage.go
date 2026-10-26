// internal/storage/storage.go
package storage

import (
	"errors"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// Backend is the interface all storage implementations must satisfy
type Backend interface {
	// Lifecycle
	Init() error
	Close() error

	// Mission management
	StartMission(mission *core.Mission, world *core.World) error
	// EndMission finalizes the current mission. tag is the optional save-time
	// tag override supplied with :MISSION:SAVE: (from the addon's exportData);
	// empty means "keep the mission-start tag".
	EndMission(tag string) error

	// Entity registration (assigns ID to the passed pointer)
	AddSoldier(s *core.Soldier) error
	AddVehicle(v *core.Vehicle) error
	AddMarker(m *core.Marker) (uint, error)

	// State recording
	DeleteSoldier(id uint16, frame core.Frame) error
	DeleteVehicle(id uint16, frame core.Frame) error
	RecordSoldierState(s *core.SoldierState) error
	RecordVehicleState(v *core.VehicleState) error
	RecordMarkerState(s *core.MarkerState) error
	DeleteMarker(dm *core.DeleteMarker) error

	// Event recording
	RecordFiredEvent(e *core.FiredEvent) error
	RecordProjectileEvent(e *core.ProjectileEvent) error
	RecordGeneralEvent(e *core.GeneralEvent) error
	RecordSectorEvent(e *core.SectorEvent) error
	RecordEndMissionEvent(e *core.EndMissionEvent) error
	RecordHitEvent(e *core.HitEvent) error
	RecordKillEvent(e *core.KillEvent) error
	RecordChatEvent(e *core.ChatEvent) error
	RecordRadioEvent(e *core.RadioEvent) error
	RecordTelemetryEvent(e *core.TelemetryEvent) error
	RecordTimeState(t *core.TimeState) error
	RecordAce3DeathEvent(e *core.Ace3DeathEvent) error
	RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error

	// Placed objects
	AddPlacedObject(p *core.PlacedObject) error
	RecordPlacedObjectEvent(e *core.PlacedObjectEvent) error

	// Mission focus
	SetFocusStart(frame core.Frame) error
	SetFocusEnd(frame core.Frame) error
}

// Discardable is an optional interface for storage backends that can drop
// the current mission instead of saving it (:MISSION:DISCARD:). Backends
// without it keep what they recorded.
type Discardable interface {
	// DiscardMission ends the current mission without saving it. It is a
	// no-op when no mission is being recorded. It returns ErrMissionKept
	// when the mission ended but its data could not be dropped.
	DiscardMission() error
}

// ErrMissionKept means a discarded mission ended, but its storage keeps what
// was recorded, as a backend without Discardable does.
var ErrMissionKept = errors.New("storage keeps the discarded mission")

// Uploadable is an optional interface for storage backends that produce
// files suitable for upload to the OCAP web frontend.
type Uploadable interface {
	GetExportedFilePath() string
	GetExportMetadata() core.UploadMetadata
}
