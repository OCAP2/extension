// internal/model/core/events.go
package core

import "time"

// FiredEvent represents a weapon being fired
type FiredEvent struct {
	ID           uint
	MissionID    uint
	SoldierID    uint
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
	ID           uint
	MissionID    uint
	SoldierID    uint
	Time         time.Time
	CaptureFrame uint
	IsAwake      bool
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
