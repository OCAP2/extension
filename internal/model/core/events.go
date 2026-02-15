// internal/model/core/events.go
package core

import "time"

// FiredEvent represents a weapon being fired.
// SoldierID is the ObjectID of the soldier who fired.
type FiredEvent struct {
	MissionID    uint
	SoldierID    uint16 // ObjectID of the soldier who fired
	Time         time.Time
	CaptureFrame uint
	Weapon       string
	Magazine     string
	FiringMode   string
	StartPos     Position3D
	EndPos       Position3D
}

// GeneralEvent is a generic event
type GeneralEvent struct {
	ID           uint
	MissionID    uint
	Time         time.Time
	CaptureFrame uint
	Name         string
	Message      string
	ExtraData    map[string]any
}

// HitEvent represents something being hit
type HitEvent struct {
	ID               uint
	MissionID        uint
	Time             time.Time
	CaptureFrame     uint
	VictimSoldierID  *uint
	VictimVehicleID  *uint
	ShooterSoldierID *uint
	ShooterVehicleID *uint
	WeaponVehicle    string
	WeaponName       string
	WeaponMagazine   string
	EventText        string
	Distance         float32
}

// KillEvent represents something being killed
type KillEvent struct {
	ID              uint
	MissionID       uint
	Time            time.Time
	CaptureFrame    uint
	VictimSoldierID *uint
	VictimVehicleID *uint
	KillerSoldierID *uint
	KillerVehicleID *uint
	WeaponVehicle   string
	WeaponName      string
	WeaponMagazine  string
	EventText       string
	Distance        float32
}

// ChatEvent represents a chat message
type ChatEvent struct {
	ID           uint
	MissionID    uint
	SoldierID    *uint
	Time         time.Time
	CaptureFrame uint
	Channel      string
	FromName     string
	SenderName   string
	Message      string
	PlayerUID    string
}

// RadioEvent represents radio transmission
type RadioEvent struct {
	ID           uint
	MissionID    uint
	SoldierID    *uint
	Time         time.Time
	CaptureFrame uint
	Radio        string
	RadioType    string
	StartEnd     string
	Channel      int8
	IsAdditional bool
	Frequency    float32
	Code         string
}

// ServerFpsEvent represents server performance data
type ServerFpsEvent struct {
	ID           uint
	MissionID    uint
	Time         time.Time
	CaptureFrame uint
	FpsAverage   float32
	FpsMin       float32
}

// Ace3DeathEvent represents ACE3 medical death
type Ace3DeathEvent struct {
	ID                 uint
	MissionID          uint
	SoldierID          uint
	Time               time.Time
	CaptureFrame       uint
	Reason             string
	LastDamageSourceID *uint
}

// Ace3UnconsciousEvent represents ACE3 unconscious state change
type Ace3UnconsciousEvent struct {
	ID            uint
	MissionID     uint
	SoldierID     uint
	Time          time.Time
	CaptureFrame  uint
	IsUnconscious bool // true = went unconscious, false = regained consciousness
}

// TrajectoryPoint represents a single position sample in a projectile trajectory.
type TrajectoryPoint struct {
	Position Position3D
	Frame    uint
}

// ProjectileHit represents a hit from a projectile on a soldier or vehicle.
type ProjectileHit struct {
	CaptureFrame uint
	Position     Position3D
	SoldierID    *uint16 // set if soldier was hit
	VehicleID    *uint16 // set if vehicle was hit
}

// ProjectileEvent represents a raw projectile event from the game.
type ProjectileEvent struct {
	MissionID       uint
	CaptureFrame    uint
	FirerObjectID   uint16
	VehicleObjectID *uint16 // nil if not fired from vehicle

	Weapon          string
	WeaponDisplay   string
	Magazine        string
	MagazineDisplay string
	Muzzle          string
	MuzzleDisplay   string
	Mode            string

	SimulationType string
	MagazineIcon   string

	Trajectory []TrajectoryPoint
	Hits       []ProjectileHit
}

// TimeState represents mission time synchronization data
type TimeState struct {
	ID             uint
	MissionID      uint
	Time           time.Time
	CaptureFrame   uint
	SystemTimeUTC  string
	MissionDate    string
	TimeMultiplier float32
	MissionTime    float32
}
