package streaming

import (
	"encoding/json"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// Message type constants matching the streaming protocol.
const (
	TypeStartMission      = "start_mission"
	TypeEndMission        = "end_mission"
	TypeAddSoldier        = "add_soldier"
	TypeAddVehicle        = "add_vehicle"
	TypeAddMarker         = "add_marker"
	TypeSoldierState      = "soldier_state"
	TypeVehicleState      = "vehicle_state"
	TypeMarkerState       = "marker_state"
	TypeDeleteMarker      = "delete_marker"
	TypeDeleteSoldier     = "delete_soldier"
	TypeDeleteVehicle     = "delete_vehicle"
	TypeFiredEvent        = "fired_event"
	TypeProjectileEvent   = "projectile_event"
	TypeGeneralEvent      = "general_event"
	TypeSectorEvent       = "sector_event"
	TypeEndMissionEvent   = "end_mission_event"
	TypeHitEvent          = "hit_event"
	TypeKillEvent         = "kill_event"
	TypeChatEvent         = "chat_event"
	TypeRadioEvent        = "radio_event"
	TypeTelemetry         = "telemetry"
	TypeTimeState         = "time_state"
	TypeAce3Death         = "ace3_death"
	TypeAce3Unconscious   = "ace3_unconscious"
	TypeAddPlaced         = "add_placed"
	TypePlacedEvent       = "placed_event"
	TypeMissionFocusStart = "mission_focus_start"
	TypeMissionFocusEnd   = "mission_focus_end"
)

// AllMessageTypes lists every streaming message type.
var AllMessageTypes = []string{
	TypeStartMission, TypeEndMission,
	TypeAddSoldier, TypeAddVehicle, TypeAddMarker,
	TypeSoldierState, TypeVehicleState, TypeMarkerState, TypeDeleteMarker, TypeDeleteSoldier, TypeDeleteVehicle,
	TypeFiredEvent, TypeProjectileEvent, TypeGeneralEvent, TypeSectorEvent, TypeEndMissionEvent,
	TypeHitEvent, TypeKillEvent, TypeChatEvent, TypeRadioEvent,
	TypeTelemetry, TypeTimeState, TypeAce3Death, TypeAce3Unconscious,
	TypeAddPlaced, TypePlacedEvent,
	TypeMissionFocusStart, TypeMissionFocusEnd,
}

// Envelope wraps all messages sent over the WebSocket.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	// Seq numbers every envelope of a resumable mission, starting with 1 for
	// start_mission. Zero (omitted) means the sender does not number messages.
	Seq uint64 `json:"seq,omitempty"`
}

// ProtocolVersionResume is the first protocol version with numbered
// envelopes, cumulative acknowledgements and mission resume.
const ProtocolVersionResume = 2

// ProtocolVersionDiscard is the first protocol version whose servers accept
// discard_mission.
const ProtocolVersionDiscard = 3

// TypeDiscardMission ends a mission without saving it, in place of
// end_mission: the addon decided the recording does not meet its save limits,
// for example the minimum mission duration. The server drops the recording
// and acknowledges it like end_mission. On a session it is numbered like any
// mission envelope; without one, it is not numbered and the payload alone
// names the mission. It is sent only to servers that acknowledged
// start_mission, or answered resume, with ProtocolVersionDiscard or later.
// Saved missions never carry it, so it is not part of AllMessageTypes. Its
// payload is DiscardMissionPayload.
const TypeDiscardMission = "discard_mission"

// DiscardMissionPayload names the discarded mission. A server that no longer
// holds the mission's session, for example because it saved it as partial
// after the producer went away, drops what it kept of the mission by id.
type DiscardMissionPayload struct {
	MissionID  string `json:"missionId"`
	ProducerID string `json:"producerId"`
}

// Connection control messages. They are not mission data, so they are not
// part of AllMessageTypes.
const (
	// TypeResume is the first envelope on a replacement connection of a
	// resumable mission. Its payload is ResumePayload.
	TypeResume = "resume"
)

// Server message types.
const (
	// ServerAck acknowledges a lifecycle message (For is set) or, when Seq is
	// set, every envelope up to and including Seq.
	ServerAck = "ack"
	// ServerResumeOK answers resume; Seq is the last envelope the server holds.
	ServerResumeOK = "resume_ok"
	// ServerResumeUnknown answers resume when the server no longer holds the
	// mission, for example after a web server restart.
	ServerResumeUnknown = "resume_unknown"
	// ServerResumeCompleted answers resume for a mission the server has
	// already saved, or discarded. It stands in for the end_mission or
	// discard_mission acknowledgement.
	ServerResumeCompleted = "resume_completed"
)

// AckMessage is a server message: an acknowledgement or a resume answer.
type AckMessage struct {
	Type string `json:"type"`          // one of the Server* types
	For  string `json:"for,omitempty"` // the message type being acknowledged
	// Seq is the highest envelope the server has received (ack, resume_ok).
	Seq uint64 `json:"seq,omitempty"`
	// ProtocolVersion is set on the start_mission acknowledgement by servers
	// that support resume, and on resume replies by servers that support
	// discard. Older servers omit it.
	ProtocolVersion uint8 `json:"protocolVersion,omitempty"`
}

// StartMissionPayload carries mission and world data.
type StartMissionPayload struct {
	Mission *core.Mission `json:"mission"`
	World   *core.World   `json:"world"`
	// Omitted by older zero-based streaming clients. Current core.Frame is one-based.
	FrameBase *uint8 `json:"frameBase,omitempty"`
	// Resume support. Omitted by older clients, which get the original
	// connection-scoped behavior.
	MissionID       string `json:"missionId,omitempty"`  // unique per recording
	ProducerID      string `json:"producerId,omitempty"` // unique per extension process
	ProtocolVersion uint8  `json:"protocolVersion,omitempty"`
	// Recovered marks a start resent from the spool, for a mission the
	// producer no longer records. A server takes any other start as the
	// producer's next mission and saves the producer's unfinished ones as
	// partial; servers at ProtocolVersionDiscard or later leave them alone
	// for a recovered start.
	Recovered bool `json:"recovered,omitempty"`
}

// ResumePayload identifies the mission a replacement connection continues.
type ResumePayload struct {
	MissionID  string `json:"missionId"`
	ProducerID string `json:"producerId"`
}

// EndMissionPayload carries finalization hints for end_mission.
// Tag overrides the mission-start tag when non-empty. Older senders send no
// payload at all (null); the receiver falls back to the start_mission tag.
type EndMissionPayload struct {
	Tag string `json:"tag,omitempty"`
	// Interrupted marks a mission that ended without :MISSION:SAVE:, for
	// example because the game server crashed. A producer recovering such a
	// mission from its spool sends it; the receiver saves the recording as
	// partial. Older receivers ignore it.
	Interrupted bool `json:"interrupted,omitempty"`
}
