// internal/model/core/soldier.go
package core

import "time"

// Soldier represents a player or AI unit
type Soldier struct {
	ID              uint
	MissionID       uint
	JoinTime        time.Time
	JoinFrame       uint
	OcapID          uint16
	OcapType        string
	UnitName        string
	GroupID         string
	Side            string
	IsPlayer        bool
	RoleDescription string
	ClassName       string
	DisplayName     string
	PlayerUID       string
	SquadParams     []any
}

// SoldierState represents soldier state at a point in time
type SoldierState struct {
	ID                uint
	MissionID         uint
	SoldierID         uint
	Time              time.Time
	CaptureFrame      uint
	Position          Position3D
	Bearing           uint16
	Lifestate         uint8
	InVehicle         bool
	InVehicleObjectID *uint
	VehicleRole       string
	UnitName          string
	IsPlayer          bool
	CurrentRole       string
	HasStableVitals   bool
	IsDraggedCarried  bool
	Stance            string
	Scores            SoldierScores
}
