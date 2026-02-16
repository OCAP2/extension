package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// Config holds WebSocket backend configuration.
type Config struct {
	URL    string
	Secret string
}

// Backend streams mission data over WebSocket to the OCAP2 web server.
// It implements storage.Backend but not storage.Uploadable.
type Backend struct {
	conn         *connection
	cfg          Config
	nextMarkerID atomic.Uint64
}

// New creates a new WebSocket storage backend.
func New(cfg Config) *Backend {
	return &Backend{
		conn: newConnection(slog.Default()),
		cfg:  cfg,
	}
}

// Init connects to the WebSocket server.
func (b *Backend) Init() error {
	return b.conn.dial(b.cfg.URL, b.cfg.Secret)
}

// Close disconnects from the WebSocket server.
func (b *Backend) Close() error {
	return b.conn.close()
}

// marshalEnvelope builds a JSON-encoded Envelope from a message type and payload.
func marshalEnvelope(msgType string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", msgType, err)
	}
	env := Envelope{Type: msgType, Payload: raw}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal %s envelope: %w", msgType, err)
	}
	return data, nil
}

// sendEnvelope marshals the payload into an Envelope and pushes it
// to the write loop (fire-and-forget).
func (b *Backend) sendEnvelope(msgType string, payload any) error {
	data, err := marshalEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	b.conn.send(data)
	return nil
}

// sendEnvelopeAndWait marshals the payload and waits for a server ack.
func (b *Backend) sendEnvelopeAndWait(msgType string, payload any) error {
	data, err := marshalEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	return b.conn.sendAndWait(data, msgType, ackTimeout)
}

// StartMission sends mission and world data and waits for server ack.
func (b *Backend) StartMission(mission *core.Mission, world *core.World) error {
	data, err := marshalEnvelope(TypeStartMission, StartMissionPayload{Mission: mission, World: world})
	if err != nil {
		return err
	}

	// Cache for reconnect replay.
	b.conn.mu.Lock()
	b.conn.cachedStartMsg = data
	b.conn.mu.Unlock()

	return b.conn.sendAndWait(data, TypeStartMission, ackTimeout)
}

// EndMission sends end_mission and waits for server ack.
func (b *Backend) EndMission() error {
	err := b.sendEnvelopeAndWait(TypeEndMission, nil)

	// Clear cached state regardless of error.
	b.conn.mu.Lock()
	b.conn.cachedStartMsg = nil
	b.conn.mu.Unlock()
	b.nextMarkerID.Store(0)

	return err
}

func (b *Backend) AddSoldier(s *core.Soldier) error {
	return b.sendEnvelope(TypeAddSoldier, s)
}

func (b *Backend) AddVehicle(v *core.Vehicle) error {
	return b.sendEnvelope(TypeAddVehicle, v)
}

// AddMarker assigns an auto-increment ID and sends the marker.
func (b *Backend) AddMarker(m *core.Marker) error {
	m.ID = uint(b.nextMarkerID.Add(1))
	return b.sendEnvelope(TypeAddMarker, m)
}

func (b *Backend) RecordSoldierState(s *core.SoldierState) error {
	return b.sendEnvelope(TypeSoldierState, s)
}

func (b *Backend) RecordVehicleState(v *core.VehicleState) error {
	return b.sendEnvelope(TypeVehicleState, v)
}

func (b *Backend) RecordMarkerState(s *core.MarkerState) error {
	return b.sendEnvelope(TypeMarkerState, s)
}

func (b *Backend) DeleteMarker(dm *core.DeleteMarker) error {
	return b.sendEnvelope(TypeDeleteMarker, dm)
}

func (b *Backend) RecordFiredEvent(e *core.FiredEvent) error {
	return b.sendEnvelope(TypeFiredEvent, e)
}

func (b *Backend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	return b.sendEnvelope(TypeProjectileEvent, e)
}

func (b *Backend) RecordGeneralEvent(e *core.GeneralEvent) error {
	return b.sendEnvelope(TypeGeneralEvent, e)
}

func (b *Backend) RecordHitEvent(e *core.HitEvent) error {
	return b.sendEnvelope(TypeHitEvent, e)
}

func (b *Backend) RecordKillEvent(e *core.KillEvent) error {
	return b.sendEnvelope(TypeKillEvent, e)
}

func (b *Backend) RecordChatEvent(e *core.ChatEvent) error {
	return b.sendEnvelope(TypeChatEvent, e)
}

func (b *Backend) RecordRadioEvent(e *core.RadioEvent) error {
	return b.sendEnvelope(TypeRadioEvent, e)
}

func (b *Backend) RecordServerFpsEvent(e *core.ServerFpsEvent) error {
	return b.sendEnvelope(TypeServerFps, e)
}

func (b *Backend) RecordTimeState(t *core.TimeState) error {
	return b.sendEnvelope(TypeTimeState, t)
}

func (b *Backend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error {
	return b.sendEnvelope(TypeAce3Death, e)
}

func (b *Backend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	return b.sendEnvelope(TypeAce3Unconscious, e)
}
