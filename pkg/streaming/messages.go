package streaming

import (
	"encoding/json"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// Message type constants matching the streaming protocol.
const (
	TypeStartMission    = "start_mission"
	TypeEndMission      = "end_mission"
	TypeAddSoldier      = "add_soldier"
	TypeAddVehicle      = "add_vehicle"
	TypeAddMarker       = "add_marker"
	TypeSoldierState    = "soldier_state"
	TypeVehicleState    = "vehicle_state"
	TypeMarkerState     = "marker_state"
	TypeDeleteMarker    = "delete_marker"
	TypeFiredEvent      = "fired_event"
	TypeProjectileEvent = "projectile_event"
	TypeGeneralEvent    = "general_event"
	TypeHitEvent        = "hit_event"
	TypeKillEvent       = "kill_event"
	TypeChatEvent       = "chat_event"
	TypeRadioEvent      = "radio_event"
	TypeServerFps       = "server_fps"
	TypeTimeState       = "time_state"
	TypeAce3Death       = "ace3_death"
	TypeAce3Unconscious = "ace3_unconscious"
)

// Envelope wraps all messages sent over the WebSocket.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// AckMessage is the server's acknowledgement response.
type AckMessage struct {
	Type string `json:"type"` // always "ack"
	For  string `json:"for"`  // the message type being acknowledged
}

// StartMissionPayload carries mission and world data.
type StartMissionPayload struct {
	Mission *core.Mission `json:"mission"`
	World   *core.World   `json:"world"`
}
