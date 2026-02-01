// internal/model/core/vehicle.go
package core

import "time"

// Vehicle represents a vehicle or static weapon
type Vehicle struct {
	ID            uint
	MissionID     uint
	JoinTime      time.Time
	JoinFrame     uint
	OcapID        uint16
	OcapType      string
	ClassName     string
	DisplayName   string
	Customization string
}

// VehicleState represents vehicle state at a point in time
type VehicleState struct {
	ID              uint
	MissionID       uint
	VehicleID       uint
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
