package worker

import (
	"errors"
	"sync"
	"testing"

	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"

	"go.opentelemetry.io/otel/metric/noop"
)

// mockLogger implements dispatcher.Logger for testing
type mockLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *mockLogger) Debug(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, msg)
}

func (l *mockLogger) Info(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, msg)
}

func (l *mockLogger) Error(msg string, keysAndValues ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, msg)
}

// mockBackend implements storage.Backend for testing
type mockBackend struct {
	mu sync.Mutex

	soldiers       []*core.Soldier
	vehicles       []*core.Vehicle
	markers        []*core.Marker
	soldierStates  []*core.SoldierState
	vehicleStates  []*core.VehicleState
	markerStates   []*core.MarkerState
	firedEvents    []*core.FiredEvent
	generalEvents  []*core.GeneralEvent
	hitEvents      []*core.HitEvent
	killEvents     []*core.KillEvent
	chatEvents     []*core.ChatEvent
	radioEvents    []*core.RadioEvent
	fpsEvents      []*core.ServerFpsEvent
	ace3Deaths     []*core.Ace3DeathEvent
	ace3Uncon      []*core.Ace3UnconsciousEvent
	initCalled     bool
	closeCalled    bool
	missionStarted bool
	missionEnded   bool
}

func (b *mockBackend) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.initCalled = true
	return nil
}

func (b *mockBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeCalled = true
	return nil
}

func (b *mockBackend) StartMission(mission *core.Mission, world *core.World) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.missionStarted = true
	return nil
}

func (b *mockBackend) EndMission() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.missionEnded = true
	return nil
}

func (b *mockBackend) AddSoldier(s *core.Soldier) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s.ID = uint(len(b.soldiers) + 1)
	b.soldiers = append(b.soldiers, s)
	return nil
}

func (b *mockBackend) AddVehicle(v *core.Vehicle) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	v.ID = uint(len(b.vehicles) + 1)
	b.vehicles = append(b.vehicles, v)
	return nil
}

func (b *mockBackend) AddMarker(m *core.Marker) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	m.ID = uint(len(b.markers) + 1)
	b.markers = append(b.markers, m)
	return nil
}

func (b *mockBackend) RecordSoldierState(s *core.SoldierState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.soldierStates = append(b.soldierStates, s)
	return nil
}

func (b *mockBackend) RecordVehicleState(v *core.VehicleState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.vehicleStates = append(b.vehicleStates, v)
	return nil
}

func (b *mockBackend) RecordMarkerState(s *core.MarkerState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.markerStates = append(b.markerStates, s)
	return nil
}

func (b *mockBackend) RecordFiredEvent(e *core.FiredEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.firedEvents = append(b.firedEvents, e)
	return nil
}

func (b *mockBackend) RecordGeneralEvent(e *core.GeneralEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.generalEvents = append(b.generalEvents, e)
	return nil
}

func (b *mockBackend) RecordHitEvent(e *core.HitEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hitEvents = append(b.hitEvents, e)
	return nil
}

func (b *mockBackend) RecordKillEvent(e *core.KillEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.killEvents = append(b.killEvents, e)
	return nil
}

func (b *mockBackend) RecordChatEvent(e *core.ChatEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chatEvents = append(b.chatEvents, e)
	return nil
}

func (b *mockBackend) RecordRadioEvent(e *core.RadioEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.radioEvents = append(b.radioEvents, e)
	return nil
}

func (b *mockBackend) RecordServerFpsEvent(e *core.ServerFpsEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fpsEvents = append(b.fpsEvents, e)
	return nil
}

func (b *mockBackend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ace3Deaths = append(b.ace3Deaths, e)
	return nil
}

func (b *mockBackend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ace3Uncon = append(b.ace3Uncon, e)
	return nil
}

func (b *mockBackend) GetSoldierByOcapID(ocapID uint16) (*core.Soldier, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.soldiers {
		if s.OcapID == ocapID {
			return s, true
		}
	}
	return nil, false
}

func (b *mockBackend) GetVehicleByOcapID(ocapID uint16) (*core.Vehicle, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, v := range b.vehicles {
		if v.OcapID == ocapID {
			return v, true
		}
	}
	return nil, false
}

func (b *mockBackend) GetMarkerByName(name string) (*core.Marker, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range b.markers {
		if m.MarkerName == name {
			return m, true
		}
	}
	return nil, false
}

// mockHandlerService provides a minimal implementation for testing
type mockHandlerService struct {
	mu sync.Mutex

	// Return values
	soldier       model.Soldier
	vehicle       model.Vehicle
	soldierState  model.SoldierState
	vehicleState  model.VehicleState
	firedEvent    model.FiredEvent
	projectile    model.ProjectileEvent
	generalEvent  model.GeneralEvent
	hitEvent      model.HitEvent
	killEvent     model.KillEvent
	chatEvent     model.ChatEvent
	radioEvent    model.RadioEvent
	fpsEvent      model.ServerFpsEvent
	ace3Death     model.Ace3DeathEvent
	ace3Uncon     model.Ace3UnconsciousEvent
	marker        model.Marker
	markerState   model.MarkerState
	deletedMarker string
	deleteFrame   int

	// Error simulation
	returnError bool
	errorMsg    string

	// Call tracking
	calls []string
}

func (h *mockHandlerService) LogNewSoldier(args []string) (model.Soldier, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogNewSoldier")
	if h.returnError {
		return model.Soldier{}, errors.New(h.errorMsg)
	}
	return h.soldier, nil
}

func (h *mockHandlerService) LogNewVehicle(args []string) (model.Vehicle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogNewVehicle")
	if h.returnError {
		return model.Vehicle{}, errors.New(h.errorMsg)
	}
	return h.vehicle, nil
}

func (h *mockHandlerService) LogSoldierState(args []string) (model.SoldierState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogSoldierState")
	if h.returnError {
		return model.SoldierState{}, errors.New(h.errorMsg)
	}
	return h.soldierState, nil
}

func (h *mockHandlerService) LogVehicleState(args []string) (model.VehicleState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogVehicleState")
	if h.returnError {
		return model.VehicleState{}, errors.New(h.errorMsg)
	}
	return h.vehicleState, nil
}

func (h *mockHandlerService) LogFiredEvent(args []string) (model.FiredEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogFiredEvent")
	if h.returnError {
		return model.FiredEvent{}, errors.New(h.errorMsg)
	}
	return h.firedEvent, nil
}

func (h *mockHandlerService) LogProjectileEvent(args []string) (model.ProjectileEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogProjectileEvent")
	if h.returnError {
		return model.ProjectileEvent{}, errors.New(h.errorMsg)
	}
	return h.projectile, nil
}

func (h *mockHandlerService) LogGeneralEvent(args []string) (model.GeneralEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogGeneralEvent")
	if h.returnError {
		return model.GeneralEvent{}, errors.New(h.errorMsg)
	}
	return h.generalEvent, nil
}

func (h *mockHandlerService) LogHitEvent(args []string) (model.HitEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogHitEvent")
	if h.returnError {
		return model.HitEvent{}, errors.New(h.errorMsg)
	}
	return h.hitEvent, nil
}

func (h *mockHandlerService) LogKillEvent(args []string) (model.KillEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogKillEvent")
	if h.returnError {
		return model.KillEvent{}, errors.New(h.errorMsg)
	}
	return h.killEvent, nil
}

func (h *mockHandlerService) LogChatEvent(args []string) (model.ChatEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogChatEvent")
	if h.returnError {
		return model.ChatEvent{}, errors.New(h.errorMsg)
	}
	return h.chatEvent, nil
}

func (h *mockHandlerService) LogRadioEvent(args []string) (model.RadioEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogRadioEvent")
	if h.returnError {
		return model.RadioEvent{}, errors.New(h.errorMsg)
	}
	return h.radioEvent, nil
}

func (h *mockHandlerService) LogFpsEvent(args []string) (model.ServerFpsEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogFpsEvent")
	if h.returnError {
		return model.ServerFpsEvent{}, errors.New(h.errorMsg)
	}
	return h.fpsEvent, nil
}

func (h *mockHandlerService) LogAce3DeathEvent(args []string) (model.Ace3DeathEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogAce3DeathEvent")
	if h.returnError {
		return model.Ace3DeathEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Death, nil
}

func (h *mockHandlerService) LogAce3UnconsciousEvent(args []string) (model.Ace3UnconsciousEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogAce3UnconsciousEvent")
	if h.returnError {
		return model.Ace3UnconsciousEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Uncon, nil
}

func (h *mockHandlerService) LogMarkerCreate(args []string) (model.Marker, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogMarkerCreate")
	if h.returnError {
		return model.Marker{}, errors.New(h.errorMsg)
	}
	return h.marker, nil
}

func (h *mockHandlerService) LogMarkerMove(args []string) (model.MarkerState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogMarkerMove")
	if h.returnError {
		return model.MarkerState{}, errors.New(h.errorMsg)
	}
	return h.markerState, nil
}

func (h *mockHandlerService) LogMarkerDelete(args []string) (string, int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogMarkerDelete")
	if h.returnError {
		return "", 0, errors.New(h.errorMsg)
	}
	return h.deletedMarker, h.deleteFrame, nil
}

func newTestDispatcher(t *testing.T) (*dispatcher.Dispatcher, *mockLogger) {
	logger := &mockLogger{}
	meter := noop.NewMeterProvider().Meter("test")

	d, err := dispatcher.New(logger, meter)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}

	return d, logger
}

func TestRegisterHandlers_RegistersAllCommands(t *testing.T) {
	d, _ := newTestDispatcher(t)

	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	manager.RegisterHandlers(d)

	expectedCommands := []string{
		":NEW:SOLDIER:",
		":NEW:VEHICLE:",
		":NEW:SOLDIER:STATE:",
		":NEW:VEHICLE:STATE:",
		":FIRED:",
		":PROJECTILE:",
		":HIT:",
		":KILL:",
		":EVENT:",
		":CHAT:",
		":RADIO:",
		":FPS:",
		":ACE3:DEATH:",
		":ACE3:UNCONSCIOUS:",
		":MARKER:CREATE:",
		":MARKER:MOVE:",
		":MARKER:DELETE:",
	}

	for _, cmd := range expectedCommands {
		if !d.HasHandler(cmd) {
			t.Errorf("expected handler for %s to be registered", cmd)
		}
	}
}

func TestHandler_NoDatabaseNoBackend_ReturnsNil(t *testing.T) {
	d, _ := newTestDispatcher(t)

	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)
	manager.RegisterHandlers(d)

	// Test that handlers do nothing when there's no valid database or backend
	result, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:", Args: []string{}})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no database/backend, got %v", result)
	}
}

func TestSetBackend(t *testing.T) {
	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	if manager.hasBackend() {
		t.Error("expected no backend initially")
	}

	backend := &mockBackend{}
	manager.SetBackend(backend)

	if !manager.hasBackend() {
		t.Error("expected backend to be set")
	}
}

