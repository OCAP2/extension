// internal/storage/storage.go
package storage

import "github.com/OCAP2/extension/v5/pkg/core"

// Backend is the interface all storage implementations must satisfy
type Backend interface {
	// Lifecycle
	Init() error
	Close() error

	// Mission management
	StartMission(mission *core.Mission, world *core.World) error
	EndMission() error

	// Entity registration (assigns ID to the passed pointer)
	AddSoldier(s *core.Soldier) error
	AddVehicle(v *core.Vehicle) error
	AddMarker(m *core.Marker) (uint, error)

	// State recording
	RecordSoldierState(s *core.SoldierState) error
	RecordVehicleState(v *core.VehicleState) error
	RecordMarkerState(s *core.MarkerState) error
	DeleteMarker(dm *core.DeleteMarker) error

	// Event recording
	RecordFiredEvent(e *core.FiredEvent) error
	RecordProjectileEvent(e *core.ProjectileEvent) error
	RecordGeneralEvent(e *core.GeneralEvent) error
	RecordHitEvent(e *core.HitEvent) error
	RecordKillEvent(e *core.KillEvent) error
	RecordChatEvent(e *core.ChatEvent) error
	RecordRadioEvent(e *core.RadioEvent) error
	RecordServerFpsEvent(e *core.ServerFpsEvent) error
	RecordTelemetryEvent(e *core.TelemetryEvent) error
	RecordTimeState(t *core.TimeState) error
	RecordAce3DeathEvent(e *core.Ace3DeathEvent) error
	RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error
}

// Uploadable is an optional interface for storage backends that produce
// files suitable for upload to the OCAP web frontend.
type Uploadable interface {
	GetExportedFilePath() string
	GetExportMetadata() core.UploadMetadata
}
