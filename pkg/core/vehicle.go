// pkg/core/vehicle.go
package core

import "time"

// Vehicle represents a vehicle or static weapon.
// ID is the ObjectID - the game's identifier for this entity.
type Vehicle struct {
	ID            uint16 // ObjectID - game identifier
	JoinTime      time.Time
	JoinFrame     Frame
	OcapType      string
	ClassName     string
	DisplayName   string
	Customization string
	Side          string // Config side: WEST, EAST, GUER, CIV (from "str side vehicle")
}

// VehicleState represents vehicle state at a point in time.
// VehicleID references the Vehicle's ID (ObjectID).
type VehicleState struct {
	VehicleID       uint16 // References Vehicle.ID (ObjectID)
	Time            time.Time
	CaptureFrame    Frame
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
