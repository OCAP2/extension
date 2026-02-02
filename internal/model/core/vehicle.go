// internal/model/core/vehicle.go
package core

import "time"

// Vehicle represents a vehicle or static weapon.
// ID is the OcapID - the game's identifier for this entity.
type Vehicle struct {
	ID            uint16 // OcapID - game identifier
	MissionID     uint
	JoinTime      time.Time
	JoinFrame     uint
	OcapType      string
	ClassName     string
	DisplayName   string
	Customization string
}

// VehicleState represents vehicle state at a point in time.
// VehicleID references the Vehicle's ID (OcapID).
type VehicleState struct {
	VehicleID       uint16 // References Vehicle.ID (OcapID)
	MissionID       uint
	Time            time.Time
	CaptureFrame    uint
	Position        Position3D
	Bearing         uint16
	IsAlive         bool
	Crew            string
	Fuel            float32
	Damage          float32
	Locked          bool
	EngineOn        bool
	Side            string
	VectorDir       string
	VectorUp        string
	TurretAzimuth   float32
	TurretElevation float32
}
