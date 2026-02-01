// internal/model/core/marker.go
package core

import "time"

// Marker represents a map marker
type Marker struct {
	ID           uint
	MissionID    uint
	Time         time.Time
	CaptureFrame uint
	MarkerName   string
	Direction    float32
	MarkerType   string
	Text         string
	OwnerID      int
	Color        string
	Size         string
	Side         string
	Position     Position3D
	Shape        string
	Alpha        float32
	Brush        string
	IsDeleted    bool
}

// MarkerState tracks marker position changes over time
type MarkerState struct {
	ID           uint
	MissionID    uint
	MarkerID     uint
	Time         time.Time
	CaptureFrame uint
	Position     Position3D
	Direction    float32
	Alpha        float32
}
