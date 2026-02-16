package parser

import (
	"time"

	"github.com/OCAP2/extension/v5/internal/model/core"
)

// HitPart represents a single hit from a projectile as parsed from ArmA data.
// EntityID is a raw ArmA object ID that the worker classifies as soldier or vehicle.
type HitPart struct {
	EntityID      uint16
	ComponentsHit []string
	CaptureFrame  uint
	Position      core.Position3D
}

// ProjectileEvent represents a projectile event as parsed from ArmA data.
// HitParts contain raw entity IDs that need classification by the worker.
type ProjectileEvent struct {
	CaptureFrame    uint
	FirerObjectID   uint16
	VehicleObjectID *uint16

	WeaponDisplay   string
	MagazineDisplay string
	MuzzleDisplay   string

	SimulationType string
	MagazineIcon   string

	Trajectory []core.TrajectoryPoint
	HitParts   []HitPart
}

// KillEvent represents a kill event as parsed from ArmA data.
// VictimID and KillerID are raw ArmA object IDs that the worker classifies
// as soldier or vehicle.
type KillEvent struct {
	Time           time.Time
	CaptureFrame   uint
	VictimID       uint16
	KillerID       uint16
	WeaponVehicle  string
	WeaponName     string
	WeaponMagazine string
	EventText      string
	Distance       float32
}

// MarkerMove represents a marker position update as parsed from ArmA data.
// MarkerName needs resolution to a MarkerID via MarkerCache by the worker.
type MarkerMove struct {
	MarkerName   string
	CaptureFrame uint
	Position     core.Position3D
	Direction    float32
	Alpha        float32
	Time         time.Time
}
