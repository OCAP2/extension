// internal/model/core/soldier.go
package core

import "time"

// Soldier represents a player or AI unit.
// ID is the ObjectID - the game's identifier for this entity.
type Soldier struct {
	ID              uint16 // ObjectID - game identifier
	MissionID       uint
	JoinTime        time.Time
	JoinFrame       uint
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

// SoldierState represents soldier state at a point in time.
// SoldierID references the Soldier's ID (ObjectID).
type SoldierState struct {
	SoldierID         uint16 // References Soldier.ID (ObjectID)
	MissionID         uint
	Time              time.Time
	CaptureFrame      uint
	Position          Position3D
	Bearing           uint16
	Lifestate         uint8
	InVehicle         bool
	InVehicleObjectID *uint16 // ObjectID of vehicle, if in one
	VehicleRole       string
	UnitName          string
	IsPlayer          bool
	CurrentRole       string
	HasStableVitals   bool
	IsDraggedCarried  bool
	Stance            string
	Scores            SoldierScores
}
