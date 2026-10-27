package websocket

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/pkg/streaming"
)

// Config holds WebSocket backend configuration.
type Config struct {
	URL         string
	Secret      string
	SaveTimeout time.Duration

	// Recovery tuning. Zero values select the defaults.
	BufferBytes     int           // unacknowledged envelopes held in memory (256 MiB)
	AckStallTimeout time.Duration // written envelopes unacknowledged this long mean a dead socket (20s)
	ReconnectMin    time.Duration // first reconnect delay (1s)
	ReconnectMax    time.Duration // reconnect delay ceiling (30s)

	// SpoolDir keeps each mission on disk under SpoolDir/<missionId> until
	// the server saves it, so the mission can be resent in full when the
	// server lost it. Empty disables spooling.
	SpoolDir      string
	SpoolMaxBytes int64 // disk limit per mission; <= 0 means unlimited
	// RecoveryInterval is how often SpoolDir is checked for missions left
	// behind by a crash or an unconfirmed save (1m).
	RecoveryInterval time.Duration
	// StatsInterval is how often the stream's state is logged while a
	// mission records (1m).
	StatsInterval time.Duration
	// ResendBytesPerSec paces catch-up traffic: the backlog resent after a
	// reconnect, a mission resent in full from the spool, and missions
	// recovered after a crash. Live traffic is never paced. 0 selects the
	// default (8 MiB/s); negative disables pacing.
	ResendBytesPerSec int
}

func (cfg Config) connOptions() connOptions {
	return connOptions{
		bufferBytes:  cfg.BufferBytes,
		ackStall:     cfg.AckStallTimeout,
		reconnectMin: cfg.ReconnectMin,
		reconnectMax: cfg.ReconnectMax,
		resendRate:   cfg.ResendBytesPerSec,
	}
}

// Backend streams mission data over WebSocket to the OCAP2 web server.
// It implements storage.Backend but not storage.Uploadable.
type Backend struct {
	missionMu    sync.Mutex
	mu           sync.Mutex
	restart      bool
	active       bool // a mission has started and not ended
	conn         *connection
	cfg          Config
	producerID   string // identifies this extension process to the server
	missionID    string // identifies the current recording
	nextMarkerID atomic.Uint64
	done         chan struct{} // closed by Close; stops spool recovery
	closeOnce    sync.Once
	loops        sync.WaitGroup // recovery and stats loops

	// Totals of connections replaced between missions; see Stats.
	reconnects, resumes, restarts atomic.Uint64
	recovered                     atomic.Uint64 // missions recovered from the spool

	// recoveryPausedUntil (unix nanos, 0 = not paused) holds off recovery
	// after a server without resume support; recoveryWake ends it early.
	recoveryPausedUntil atomic.Int64
	recoveryWake        chan struct{}
}

// New creates a new WebSocket storage backend.
func New(cfg Config) *Backend {
	return &Backend{
		conn:       newConnection(slog.Default(), cfg.connOptions()),
		cfg:        cfg,
		producerID: newID(),
		done:       make(chan struct{}),

		recoveryWake: make(chan struct{}, 1),
	}
}

func (b *Backend) logger() *slog.Logger { return slog.Default() }

// newID returns a random RFC 4122 version 4 UUID.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Init connects to the WebSocket server.
func (b *Backend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.conn.dial(b.cfg.URL, b.cfg.Secret); err != nil {
		return err
	}
	if b.cfg.SpoolDir != "" {
		b.loops.Add(1)
		go func() {
			defer b.loops.Done()
			b.recoveryLoop(b.done)
		}()
	}
	b.loops.Add(1)
	go func() {
		defer b.loops.Done()
		b.statsLoop(b.done)
	}()
	return nil
}

// Close disconnects from the WebSocket server.
func (b *Backend) Close() error {
	b.closeOnce.Do(func() { close(b.done) })
	b.loops.Wait()
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if sp := conn.log.currentSpool(); sp != nil {
		_ = sp.close() // an unfinished mission stays on disk for recovery
	}
	return conn.close()
}

// marshalPayload encodes a payload for an envelope.
func marshalPayload(msgType string, payload any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", msgType, err)
	}
	return raw, nil
}

// marshalEnvelope builds a JSON-encoded Envelope from a message type,
// payload and sequence number (0 omits it).
func marshalEnvelope(msgType string, payload any, seq uint64) ([]byte, error) {
	raw, err := marshalPayload(msgType, payload)
	if err != nil {
		return nil, err
	}
	return encodeEnvelope(msgType, raw, seq)
}

func encodeEnvelope(msgType string, raw json.RawMessage, seq uint64) ([]byte, error) {
	data, err := json.Marshal(streaming.Envelope{Type: msgType, Payload: raw, Seq: seq})
	if err != nil {
		return nil, fmt.Errorf("marshal %s envelope: %w", msgType, err)
	}
	return data, nil
}

// sendEnvelope logs one mission envelope for delivery (fire-and-forget).
// Envelopes recorded before any mission starts are discarded, as the server
// ignores them.
func (b *Backend) sendEnvelope(msgType string, payload any) error {
	raw, err := marshalPayload(msgType, payload)
	if err != nil {
		return err
	}
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	b.mu.Lock()
	restart, active, conn := b.restart, b.active, b.conn
	b.mu.Unlock()
	if restart {
		return fmt.Errorf("mission already ended")
	}
	if !active {
		return nil
	}
	return conn.send(func(seq uint64) ([]byte, error) { return encodeEnvelope(msgType, raw, seq) })
}

// sendEnvelopeAndWait logs an envelope and waits for its server ack.
func (b *Backend) sendEnvelopeAndWait(msgType string, payload any, timeout time.Duration) error {
	raw, err := marshalPayload(msgType, payload)
	if err != nil {
		return err
	}
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	conn.drainAcks()
	if err := conn.send(func(seq uint64) ([]byte, error) { return encodeEnvelope(msgType, raw, seq) }); err != nil {
		return err
	}
	_, err = conn.waitAck(msgType, timeout)
	return err
}

// StartMission sends mission and world data and waits for server ack.
// The ack tells whether the server supports resume; older servers keep the
// original connection-scoped behavior.
func (b *Backend) StartMission(mission *core.Mission, world *core.World) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()

	// Normally done when the next mission loads (AbandonMission); this
	// covers a start without one.
	b.abandonMission()

	b.mu.Lock()
	// A completed mission's server closes its socket. Dial a fresh connection
	// before accepting the next mission instead of racing the reconnect loop.
	if b.restart {
		b.conn.close()
		b.reconnects.Add(b.conn.reconnects.Load())
		b.resumes.Add(b.conn.resumes.Load())
		b.restarts.Add(b.conn.restarts.Load())
		b.conn = newConnection(slog.Default(), b.cfg.connOptions())
		if err := b.conn.dial(b.cfg.URL, b.cfg.Secret); err != nil {
			b.mu.Unlock()
			return err
		}
		b.restart = false
	}
	conn := b.conn
	b.missionID = newID()
	missionID := b.missionID
	b.mu.Unlock()
	conn.protocol.Store(0)

	base := uint8(1)
	raw, err := marshalPayload(streaming.TypeStartMission, streaming.StartMissionPayload{
		Mission: mission, World: world, FrameBase: &base,
		MissionID: missionID, ProducerID: b.producerID, ProtocolVersion: streaming.ProtocolVersionDiscard,
	})
	if err != nil {
		return err
	}
	resume, err := marshalEnvelope(streaming.TypeResume, streaming.ResumePayload{MissionID: missionID, ProducerID: b.producerID}, 0)
	if err != nil {
		return err
	}

	var sp *spool
	if b.cfg.SpoolDir != "" {
		sp, err = openSpool(filepath.Join(b.cfg.SpoolDir, missionID), b.cfg.SpoolMaxBytes, slog.Default())
		if err != nil {
			slog.Default().Warn("WebSocket spool unavailable; recording without it", "error", err)
		}
	}

	conn.drainAcks()
	if err := conn.beginMission(func(seq uint64) ([]byte, error) {
		return encodeEnvelope(streaming.TypeStartMission, raw, seq)
	}, resume, sp); err != nil {
		if sp != nil {
			_ = sp.remove()
		}
		return err
	}
	b.mu.Lock()
	b.active = true
	b.mu.Unlock()

	ack, err := conn.waitAck(streaming.TypeStartMission, ackTimeout)
	if err != nil {
		// Nothing records a mission that did not start: forget it, so the
		// next start does not take it for an abandoned one. A server that
		// started it late loses its producer and drops the empty session.
		if sp := b.clearMission(); sp != nil {
			removeSpool(sp)
		}
		_ = conn.close()
		return err
	}
	// Stored first: a reconnect from here on records what it finds.
	conn.protocol.Store(uint32(ack.ProtocolVersion))
	resumable := ack.ProtocolVersion >= streaming.ProtocolVersionResume
	conn.setResumable(resumable)
	if resumable {
		conn.ackUpTo(max(ack.Seq, 1))
		b.resumeRecovery()
	}
	slog.Default().Info("WebSocket mission started", "missionId", missionID, "resumable", resumable, "discardable", b.discardable())
	return nil
}

// AbandonMission ends a mission the game moved on from without saving or
// discarding it, for example after an admin changed the mission. It ends as
// interrupted, so the server saves what it got as partial and nothing is left
// to recover. One that recorded nothing after its start is dropped instead,
// when the server can. A server without resume support saves a mission as
// partial when its producer disconnects, so the connection is closed instead.
// It is a no-op when no mission is recording.
//
// A save or discard that holds the mission for longer than a start does is
// ending it anyway, and the next mission load is not held up waiting for its
// server: AbandonMission then leaves the mission to it.
func (b *Backend) AbandonMission() error {
	deadline := time.Now().Add(ackTimeout + time.Second)
	for !b.missionMu.TryLock() {
		if time.Now().After(deadline) {
			slog.Default().Info("WebSocket mission is being ended; not abandoning it")
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer b.missionMu.Unlock()
	return b.abandonMission()
}

// abandonMission is AbandonMission with missionMu held.
func (b *Backend) abandonMission() error {
	if !b.recording() {
		return nil
	}
	b.mu.Lock()
	conn, missionID := b.conn, b.missionID
	b.mu.Unlock()
	switch {
	case !conn.isResumable():
		slog.Default().Warn("WebSocket mission abandoned without save or discard; disconnecting, the server saves it as partial", "missionId", missionID)
		if sp := b.clearMission(); sp != nil {
			removeSpool(sp)
		}
		return conn.close()
	case conn.log.lastSeq() <= 1 && b.discardable():
		slog.Default().Warn("WebSocket mission abandoned before it recorded anything; discarding it", "missionId", missionID)
		return b.discardMission(abandonTimeout)
	default:
		slog.Default().Warn("WebSocket mission abandoned without save or discard; ending it as interrupted", "missionId", missionID)
		return b.endMission(streaming.EndMissionPayload{Interrupted: true}, abandonTimeout)
	}
}

// discardable reports whether the server serving the mission accepts
// discard_mission, as it last announced.
func (b *Backend) discardable() bool {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	return conn.protocol.Load() >= streaming.ProtocolVersionDiscard
}

// EndMission sends end_mission and waits for server ack.
// tag is the save-time override from :MISSION:SAVE:; empty keeps the
// mission-start tag on the web side and sends a null payload for byte
// compatibility with the previous wire format.
func (b *Backend) EndMission(tag string) error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()

	var payload any
	if tag != "" {
		payload = streaming.EndMissionPayload{Tag: tag}
	}
	return b.endMission(payload, b.saveTimeout())
}

// abandonTimeout bounds the wait for the server to save an abandoned mission
// before the next one starts; past it, the spool is kept for recovery. Tests
// lower it.
var abandonTimeout = 30 * time.Second

// endMission sends end_mission, waits for its ack and forgets the mission.
// Caller holds missionMu.
func (b *Backend) endMission(payload any, timeout time.Duration) error {
	err := b.sendEnvelopeAndWait(streaming.TypeEndMission, payload, timeout)

	// Clear mission state regardless of error.
	sp := b.clearMission()

	// The spool is needed until the server has saved the mission.
	if sp != nil {
		if err == nil {
			removeSpool(sp)
		} else {
			_ = sp.close()
			slog.Default().Warn("WebSocket mission not confirmed by the server; spool kept", "dir", sp.dir, "error", err)
		}
	}

	return err
}

// errDiscardUnsupported means the server serving the mission predates
// discard_mission.
var errDiscardUnsupported = fmt.Errorf("websocket server cannot discard missions: %w", storage.ErrMissionKept)

// DiscardMission ends the mission without saving it: it sends
// discard_mission, and the server drops the recording. A server older than
// ProtocolVersionDiscard cannot drop it. The mission is then abandoned: the
// connection is closed and nothing is kept for recovery, but that server
// still saves what it received as partial, as it did before discard existed,
// and DiscardMission returns storage.ErrMissionKept.
func (b *Backend) DiscardMission() error {
	b.missionMu.Lock()
	defer b.missionMu.Unlock()
	return b.discardMission(b.saveTimeout())
}

// discardMission sends discard_mission, waits for its ack and forgets the
// mission. Caller holds missionMu.
func (b *Backend) discardMission(timeout time.Duration) error {
	b.mu.Lock()
	active, missionID := b.active, b.missionID
	b.mu.Unlock()
	if !active {
		return nil
	}

	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	conn.drainAcks()
	// A reconnect that finds the server replaced by an older one lowers the
	// version, and refuses a discard already waiting for its ack.
	discardable := b.discardable()
	err := errDiscardUnsupported
	logged := false // discard_mission is in the log, and so in a healthy spool
	if discardable {
		payload := streaming.DiscardMissionPayload{MissionID: missionID, ProducerID: b.producerID}
		err = conn.send(func(seq uint64) ([]byte, error) {
			return marshalEnvelope(streaming.TypeDiscardMission, payload, seq)
		})
		if err == nil {
			logged = true
			_, err = conn.waitAck(streaming.TypeDiscardMission, timeout)
		}
		if errors.Is(err, errDiscardUnsupported) {
			discardable = false
		}
	}

	// A spool that holds discard_mission lets recovery finish the discard.
	// Without it, recovery would resend the mission and the server save it.
	sp := b.clearMission()
	keep := err != nil && discardable && logged && sp != nil && sp.healthy()
	if sp != nil {
		if keep {
			_ = sp.close()
		} else {
			removeSpool(sp)
		}
	}

	switch {
	case err == nil:
		slog.Default().Info("WebSocket mission discarded", "missionId", missionID)
	case !discardable:
		_ = conn.close() // the server stops waiting for the mission; StartMission dials anew
		slog.Default().Warn("WebSocket server cannot discard missions; it saves what it received as partial", "missionId", missionID)
	case keep:
		slog.Default().Warn("WebSocket mission discard not confirmed by the server; spool kept to finish it", "missionId", missionID, "dir", sp.dir, "error", err)
	default:
		slog.Default().Warn("WebSocket mission discard not confirmed by the server", "missionId", missionID, "error", err)
	}
	return err
}

// clearMission forgets the current mission after end_mission or
// discard_mission and returns its spool, if any, for the caller to keep or
// remove.
func (b *Backend) clearMission() *spool {
	b.mu.Lock()
	conn := b.conn
	b.restart = true
	b.active = false
	b.missionID = ""
	b.mu.Unlock()
	sp := conn.log.currentSpool()
	conn.endMission()
	b.nextMarkerID.Store(0)
	return sp
}

// recording reports whether a mission has started and not ended.
func (b *Backend) recording() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

func (b *Backend) saveTimeout() time.Duration {
	if b.cfg.SaveTimeout <= 0 {
		return 10 * time.Minute
	}
	return b.cfg.SaveTimeout
}

func removeSpool(sp *spool) {
	if err := sp.remove(); err != nil {
		slog.Default().Warn("WebSocket spool not removed", "dir", sp.dir, "error", err)
	}
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
