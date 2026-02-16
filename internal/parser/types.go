package parser

import (
	"encoding/json"

	"github.com/OCAP2/extension/v5/internal/model/core"
)

// RawHitPart holds parsed hit data before entity classification (soldier vs vehicle).
// The worker layer uses EntityCache to classify each hit.
type RawHitPart struct {
	EntityID      uint16
	ComponentsHit json.RawMessage
	CaptureFrame  uint
	Position      core.Position3D
}

// ParsedProjectileEvent holds a projectile event with raw hit parts
// that need entity classification by the worker layer.
type ParsedProjectileEvent struct {
	Event    core.ProjectileEvent
	HitParts []RawHitPart
}

// ParsedKillEvent holds a kill event with raw victim/killer IDs
// that need entity classification (soldier vs vehicle) by the worker layer.
type ParsedKillEvent struct {
	Event    core.KillEvent
	VictimID uint16
	KillerID uint16
}

// ParsedMarkerMove holds a marker state with the marker name
// that needs resolution to a MarkerID via MarkerCache by the worker layer.
type ParsedMarkerMove struct {
	State      core.MarkerState
	MarkerName string
}
