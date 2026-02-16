// pkg/core/events.go
package core

import (
	"time"
)

// FiredEvent represents a weapon being fired.
// SoldierID is the ObjectID of the soldier who fired.
type FiredEvent struct {
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
	Time         time.Time
	CaptureFrame uint
	Name         string
	Message      string
	ExtraData    map[string]any
}

// HitEvent represents something being hit
type HitEvent struct {
	ID               uint
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
	Time         time.Time
	CaptureFrame uint
	FpsAverage   float32
	FpsMin       float32
}

// Ace3DeathEvent represents ACE3 medical death
type Ace3DeathEvent struct {
	ID                 uint
	SoldierID          uint
	Time               time.Time
	CaptureFrame       uint
	Reason             string
	LastDamageSourceID *uint
}

// Ace3UnconsciousEvent represents ACE3 unconscious state change
type Ace3UnconsciousEvent struct {
	ID            uint
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
	CaptureFrame  uint
	Position      Position3D
	SoldierID     *uint16  // set if soldier was hit
	VehicleID     *uint16  // set if vehicle was hit
	ComponentsHit []string // body/vehicle parts hit
}

// ProjectileEvent represents a raw projectile event from the game.
type ProjectileEvent struct {
	CaptureFrame    uint
	FirerObjectID   uint16
	VehicleObjectID *uint16 // nil if not fired from vehicle

	WeaponDisplay   string
	MagazineDisplay string
	MuzzleDisplay   string

	SimulationType string
	MagazineIcon   string

	Trajectory []TrajectoryPoint
	Hits       []ProjectileHit
}

// TimeState represents mission time synchronization data
type TimeState struct {
	ID             uint
	Time           time.Time
	CaptureFrame   uint
	SystemTimeUTC  string
	MissionDate    string
	TimeMultiplier float32
	MissionTime    float32
}

// TelemetryEvent represents a unified telemetry snapshot from the game server.
// Replaces the old :FPS: and :METRIC: commands with a single :TELEMETRY: call.
type TelemetryEvent struct {
	Time         time.Time
	CaptureFrame uint

	// FPS data (also written to mission recording via ServerFpsEvent)
	FpsAverage float32
	FpsMin     float32

	// Per-side entity counts: [east, west, independent, civilian]
	SideEntityCounts [4]SideEntityCount

	// Global entity counts (all sides combined)
	GlobalCounts GlobalEntityCount

	// Running script counts
	Scripts ScriptCounts

	// Weather snapshot
	Weather WeatherData

	// Per-player network stats (variable length)
	Players []PlayerNetworkData
}

// SideEntityCount holds entity counts for a single side, split by locality.
type SideEntityCount struct {
	Local  EntityLocality
	Remote EntityLocality
}

// EntityLocality holds entity counts for one locality (server-local or remote).
type EntityLocality struct {
	UnitsTotal    uint
	UnitsAlive    uint
	UnitsDead     uint
	Groups        uint
	Vehicles      uint
	WeaponHolders uint
}

// GlobalEntityCount holds global entity counts across all sides.
type GlobalEntityCount struct {
	UnitsAlive        uint
	UnitsDead         uint
	Groups            uint
	Vehicles          uint
	WeaponHolders     uint
	PlayersAlive      uint
	PlayersDead       uint
	PlayersConnected  uint
}

// ScriptCounts holds the number of running scripts by type.
type ScriptCounts struct {
	Spawn  uint
	ExecVM uint
	Exec   uint
	ExecFSM uint
	PFH    uint
}

// WeatherData holds a snapshot of the weather state.
type WeatherData struct {
	Fog           float32
	Overcast      float32
	Rain          float32
	Humidity      float32
	Waves         float32
	WindDir       float32
	WindStr       float32
	Gusts         float32
	Lightnings    float32
	MoonIntensity float32
	MoonPhase     float32
	SunOrMoon     float32
}

// PlayerNetworkData holds network stats for a single player.
type PlayerNetworkData struct {
	UID     string
	Name    string
	Ping    float32
	BW      float32
	Desync  float32
}
