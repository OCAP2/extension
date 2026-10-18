package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

// Config holds WebSocket backend configuration.
type Config struct {
	URL         string
	Secret      string
	SaveTimeout time.Duration
}

// Backend streams mission data over WebSocket to the OCAP2 web server.
// It implements storage.Backend but not storage.Uploadable.
type Backend struct {
	missionMu    sync.Mutex
	mu           sync.Mutex
	restart      bool
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
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn.dial(b.cfg.URL, b.cfg.Secret)
}

// Close disconnects from the WebSocket server.
func (b *Backend) Close() error {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	return conn.close()
}

// marshalEnvelope builds a JSON-encoded Envelope from a message type and payload.
func marshalEnvelope(msgType string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", msgType, err)
	}
	env := streaming.Envelope{Type: msgType, Payload: raw}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal %s envelope: %w", msgType, err)
	}
	return data, nil
}

// sendEnvelope marshals the payload into an Envelope and pushes it
// to the write loop (fire-and-forget).
func (b *Backend) sendEnvelope(msgType string, payload any) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	data, err := marshalEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.restart {
		return fmt.Errorf("mission already ended")
	}
	return b.conn.send(data)
}

// sendEnvelopeAndWait marshals the payload and waits for a server ack.
func (b *Backend) sendEnvelopeAndWait(msgType string, payload any) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	data, err := marshalEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	return conn.sendAndWait(data, msgType, ackTimeout)
}

// StartMission sends mission and world data and waits for server ack.
func (b *Backend) StartMission(mission *core.Mission, world *core.World) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	b.mu.Lock()
	// A completed mission's server closes its socket. Dial a fresh connection
	// before accepting the next mission instead of racing the reconnect loop.
	if b.restart {
		b.conn.close()
		b.conn = newConnection(slog.Default())
		if err := b.conn.dial(b.cfg.URL, b.cfg.Secret); err != nil {
			b.mu.Unlock()
			return err
		}
		b.restart = false
	}
	conn := b.conn
	b.mu.Unlock()
	base := uint8(1)
	data, err := marshalEnvelope(streaming.TypeStartMission, streaming.StartMissionPayload{Mission: mission, World: world, FrameBase: &base})
	if err != nil {
		return err
	}

	return conn.sendAndWait(data, streaming.TypeStartMission, ackTimeout)
}

// EndMission sends end_mission and waits for server ack.
// tag is the save-time override from :MISSION:SAVE:; empty keeps the
// mission-start tag on the web side and sends a null payload for byte
// compatibility with the previous wire format.
func (b *Backend) EndMission(tag string) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()

	var payload any
	if tag != "" {
		payload = streaming.EndMissionPayload{Tag: tag}
	}
	data, err := marshalEnvelope(streaming.TypeEndMission, payload)
	if err != nil {
		return err
	}
	timeout := b.cfg.SaveTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	err = conn.sendAndWait(data, streaming.TypeEndMission, timeout)

	// Clear cached state regardless of error.
	conn.mu.Lock()
	conn.cachedStartMsg = nil
	conn.mu.Unlock()
	b.nextMarkerID.Store(0)
	b.mu.Lock()
	b.restart = true
	b.mu.Unlock()

	return err
}

func (b *Backend) AddSoldier(s *core.Soldier) error {
	return b.sendEnvelope(streaming.TypeAddSoldier, s)
}

func (b *Backend) AddVehicle(v *core.Vehicle) error {
	return b.sendEnvelope(streaming.TypeAddVehicle, v)
}

// AddMarker assigns an auto-increment ID and sends the marker.
// Returns the assigned ID.
func (b *Backend) AddMarker(m *core.Marker) (uint, error) {
	id := uint(b.nextMarkerID.Add(1))
	markerCopy := *m
	markerCopy.ID = id
	return id, b.sendEnvelope(streaming.TypeAddMarker, &markerCopy)
}

func (b *Backend) DeleteSoldier(id uint16, frame core.Frame) error {
	return b.sendEnvelope(streaming.TypeDeleteSoldier, map[string]any{"id": id, "frame": frame})
}

func (b *Backend) DeleteVehicle(id uint16, frame core.Frame) error {
	return b.sendEnvelope(streaming.TypeDeleteVehicle, map[string]any{"id": id, "frame": frame})
}

func (b *Backend) RecordSoldierState(s *core.SoldierState) error {
	return b.sendEnvelope(streaming.TypeSoldierState, s)
}

func (b *Backend) RecordVehicleState(v *core.VehicleState) error {
	return b.sendEnvelope(streaming.TypeVehicleState, v)
}

func (b *Backend) RecordMarkerState(s *core.MarkerState) error {
	return b.sendEnvelope(streaming.TypeMarkerState, s)
}

func (b *Backend) DeleteMarker(dm *core.DeleteMarker) error {
	return b.sendEnvelope(streaming.TypeDeleteMarker, dm)
}

func (b *Backend) RecordFiredEvent(e *core.FiredEvent) error {
	return b.sendEnvelope(streaming.TypeFiredEvent, e)
}

func (b *Backend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	return b.sendEnvelope(streaming.TypeProjectileEvent, e)
}

func (b *Backend) RecordGeneralEvent(e *core.GeneralEvent) error {
	return b.sendEnvelope(streaming.TypeGeneralEvent, e)
}

func (b *Backend) RecordSectorEvent(e *core.SectorEvent) error {
	return b.sendEnvelope(streaming.TypeSectorEvent, e)
}

func (b *Backend) RecordEndMissionEvent(e *core.EndMissionEvent) error {
	return b.sendEnvelope(streaming.TypeEndMissionEvent, e)
}

func (b *Backend) RecordHitEvent(e *core.HitEvent) error {
	return b.sendEnvelope(streaming.TypeHitEvent, e)
}

func (b *Backend) RecordKillEvent(e *core.KillEvent) error {
	return b.sendEnvelope(streaming.TypeKillEvent, e)
}

func (b *Backend) RecordChatEvent(e *core.ChatEvent) error {
	return b.sendEnvelope(streaming.TypeChatEvent, e)
}

func (b *Backend) RecordRadioEvent(e *core.RadioEvent) error {
	return b.sendEnvelope(streaming.TypeRadioEvent, e)
}

func (b *Backend) RecordTelemetryEvent(e *core.TelemetryEvent) error {
	return b.sendEnvelope(streaming.TypeTelemetry, e)
}

func (b *Backend) RecordTimeState(t *core.TimeState) error {
	return b.sendEnvelope(streaming.TypeTimeState, t)
}

func (b *Backend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error {
	return b.sendEnvelope(streaming.TypeAce3Death, e)
}

func (b *Backend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	return b.sendEnvelope(streaming.TypeAce3Unconscious, e)
}

func (b *Backend) SetFocusStart(frame core.Frame) error {
	return b.sendEnvelope(streaming.TypeMissionFocusStart, map[string]any{"frame": frame})
}

func (b *Backend) SetFocusEnd(frame core.Frame) error {
	return b.sendEnvelope(streaming.TypeMissionFocusEnd, map[string]any{"frame": frame})
}

func (b *Backend) AddPlacedObject(p *core.PlacedObject) error {
	return b.sendEnvelope(streaming.TypeAddPlaced, p)
}

func (b *Backend) RecordPlacedObjectEvent(e *core.PlacedObjectEvent) error {
	return b.sendEnvelope(streaming.TypePlacedEvent, e)
}
