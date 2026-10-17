package core

import "time"

// PlacedObject represents a placed object (mine, explosive, etc.) in the game world.
// ID is the ObjectID - the game's identifier for this entity.
type PlacedObject struct {
	ID           uint16    // ObjectID - game identifier
	JoinTime     time.Time
	JoinFrame    Frame
	ClassName    string
	DisplayName  string
	Position     Position3D
	OwnerID      uint16 // OCAP ID of soldier who placed it
	Side         string
	Weapon       string
	MagazineIcon string

	// Additive, optional. Absent in recordings made before trench tracking:
	// Direction stays 0 and MarkerIcon stays empty, which reproduces the old
	// export byte-for-byte.
	Direction  float32 // Object heading in degrees, used to rotate the exported marker
	MarkerIcon string  // Explicit marker icon name; overrides the MagazineIcon-derived type
}

// PlacedObjectEvent represents a lifecycle event (detonation or deletion) for a placed object.
type PlacedObjectEvent struct {
	CaptureFrame Frame
	PlacedID     uint16
	EventType    string // "detonated", "deleted", or "hit"
	Position     Position3D
	HitEntityID *uint16 // OCAP ID of hit entity (only for "hit" events)
}
