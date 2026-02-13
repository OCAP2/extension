package worker

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/handlers"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
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
	timeStates     []*core.TimeState
	ace3Deaths     []*core.Ace3DeathEvent
	ace3Uncon      []*core.Ace3UnconsciousEvent
	deletedMarkers map[string]uint // name -> endFrame
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
	// ID is the ObjectID, already set by caller
	b.soldiers = append(b.soldiers, s)
	return nil
}

func (b *mockBackend) AddVehicle(v *core.Vehicle) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	// ID is the ObjectID, already set by caller
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

func (b *mockBackend) RecordTimeState(t *core.TimeState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.timeStates = append(b.timeStates, t)
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

func (b *mockBackend) DeleteMarker(name string, endFrame uint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.deletedMarkers == nil {
		b.deletedMarkers = make(map[string]uint)
	}
	b.deletedMarkers[name] = endFrame
}

func (b *mockBackend) GetSoldierByObjectID(ocapID uint16) (*core.Soldier, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.soldiers {
		if s.ID == ocapID { // ID is the ObjectID
			return s, true
		}
	}
	return nil, false
}

func (b *mockBackend) GetVehicleByObjectID(ocapID uint16) (*core.Vehicle, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, v := range b.vehicles {
		if v.ID == ocapID { // ID is the ObjectID
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
	timeState     model.TimeState
	ace3Death     model.Ace3DeathEvent
	ace3Uncon     model.Ace3UnconsciousEvent
	marker        model.Marker
	markerState   model.MarkerState
	deletedMarker string
	deleteFrame   uint

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

func (h *mockHandlerService) LogTimeState(args []string) (model.TimeState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogTimeState")
	if h.returnError {
		return model.TimeState{}, errors.New(h.errorMsg)
	}
	return h.timeState, nil
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

func (h *mockHandlerService) LogMarkerDelete(args []string) (string, uint, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "LogMarkerDelete")
	if h.returnError {
		return "", 0, errors.New(h.errorMsg)
	}
	return h.deletedMarker, h.deleteFrame, nil
}

func (h *mockHandlerService) GetMissionContext() *handlers.MissionContext {
	return nil
}

func newTestDispatcher(t *testing.T) (*dispatcher.Dispatcher, *mockLogger) {
	logger := &mockLogger{}

	d, err := dispatcher.New(logger)
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
		":NEW:MARKER:",
		":NEW:MARKER:STATE:",
		":DELETE:MARKER:",
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

func TestNewQueues(t *testing.T) {
	queues := NewQueues()

	if queues == nil {
		t.Fatal("expected non-nil queues")
	}
	if queues.Soldiers == nil {
		t.Error("expected Soldiers queue to be initialized")
	}
	if queues.SoldierStates == nil {
		t.Error("expected SoldierStates queue to be initialized")
	}
	if queues.Vehicles == nil {
		t.Error("expected Vehicles queue to be initialized")
	}
	if queues.VehicleStates == nil {
		t.Error("expected VehicleStates queue to be initialized")
	}
	if queues.FiredEvents == nil {
		t.Error("expected FiredEvents queue to be initialized")
	}
	if queues.ProjectileEvents == nil {
		t.Error("expected ProjectileEvents queue to be initialized")
	}
	if queues.GeneralEvents == nil {
		t.Error("expected GeneralEvents queue to be initialized")
	}
	if queues.HitEvents == nil {
		t.Error("expected HitEvents queue to be initialized")
	}
	if queues.KillEvents == nil {
		t.Error("expected KillEvents queue to be initialized")
	}
	if queues.ChatEvents == nil {
		t.Error("expected ChatEvents queue to be initialized")
	}
	if queues.RadioEvents == nil {
		t.Error("expected RadioEvents queue to be initialized")
	}
	if queues.FpsEvents == nil {
		t.Error("expected FpsEvents queue to be initialized")
	}
	if queues.Ace3DeathEvents == nil {
		t.Error("expected Ace3DeathEvents queue to be initialized")
	}
	if queues.Ace3UnconsciousEvents == nil {
		t.Error("expected Ace3UnconsciousEvents queue to be initialized")
	}
	if queues.Markers == nil {
		t.Error("expected Markers queue to be initialized")
	}
	if queues.MarkerStates == nil {
		t.Error("expected MarkerStates queue to be initialized")
	}
}

func TestNewManager(t *testing.T) {
	deps := Dependencies{
		IsDatabaseValid: func() bool { return true },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()

	manager := NewManager(deps, queues)

	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.hasBackend() {
		t.Error("expected no backend initially")
	}
}

func TestHandleNewSoldier_CachesEntityWithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	handlerService := &mockHandlerService{
		soldier: model.Soldier{ObjectID: 42, UnitName: "Test Soldier"},
	}

	deps := Dependencies{
		IsDatabaseValid:  func() bool { return false },
		ShouldSaveLocal:  func() bool { return false },
		DBInsertsPaused:  func() bool { return false },
		HandlerService:   handlerService,
		EntityCache:      entityCache,
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	backend := &mockBackend{}
	manager.SetBackend(backend)
	manager.RegisterHandlers(d)

	// Dispatch new soldier event
	result, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:", Args: []string{}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	// Verify soldier is in backend
	if len(backend.soldiers) != 1 {
		t.Errorf("expected 1 soldier in backend, got %d", len(backend.soldiers))
	}

	// Verify soldier is also in EntityCache
	cachedSoldier, found := entityCache.GetSoldier(42)
	if !found {
		t.Error("expected soldier to be cached in EntityCache")
	}
	if cachedSoldier.UnitName != "Test Soldier" {
		t.Errorf("expected cached soldier name 'Test Soldier', got '%s'", cachedSoldier.UnitName)
	}
}

func TestHandleNewVehicle_CachesEntityWithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	handlerService := &mockHandlerService{
		vehicle: model.Vehicle{ObjectID: 99, OcapType: "car"},
	}

	deps := Dependencies{
		IsDatabaseValid:  func() bool { return false },
		ShouldSaveLocal:  func() bool { return false },
		DBInsertsPaused:  func() bool { return false },
		HandlerService:   handlerService,
		EntityCache:      entityCache,
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	backend := &mockBackend{}
	manager.SetBackend(backend)
	manager.RegisterHandlers(d)

	// Dispatch new vehicle event
	result, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:", Args: []string{}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	// Verify vehicle is in backend
	if len(backend.vehicles) != 1 {
		t.Errorf("expected 1 vehicle in backend, got %d", len(backend.vehicles))
	}

	// Verify vehicle is also in EntityCache
	cachedVehicle, found := entityCache.GetVehicle(99)
	if !found {
		t.Error("expected vehicle to be cached in EntityCache")
	}
	if cachedVehicle.OcapType != "car" {
		t.Errorf("expected cached vehicle type 'car', got '%s'", cachedVehicle.OcapType)
	}
}

func TestHandleNewSoldier_CachesEntityWithoutBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	handlerService := &mockHandlerService{
		soldier: model.Soldier{ObjectID: 55, UnitName: "Queue Soldier"},
	}

	deps := Dependencies{
		IsDatabaseValid:  func() bool { return true }, // DB valid, no backend
		ShouldSaveLocal:  func() bool { return false },
		DBInsertsPaused:  func() bool { return false },
		HandlerService:   handlerService,
		EntityCache:      entityCache,
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)
	// No backend set - should use queues
	manager.RegisterHandlers(d)

	// Dispatch new soldier event
	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:", Args: []string{}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify soldier is in queue
	if queues.Soldiers.Len() != 1 {
		t.Errorf("expected 1 soldier in queue, got %d", queues.Soldiers.Len())
	}

	// Verify soldier is also in EntityCache
	cachedSoldier, found := entityCache.GetSoldier(55)
	if !found {
		t.Error("expected soldier to be cached in EntityCache even when using queue")
	}
	if cachedSoldier.UnitName != "Queue Soldier" {
		t.Errorf("expected cached soldier name 'Queue Soldier', got '%s'", cachedSoldier.UnitName)
	}
}

func TestHandleMarkerDelete_WithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()

	handlerService := &mockHandlerService{
		marker:        model.Marker{MarkerName: "Projectile#123"},
		deletedMarker: "Projectile#123",
		deleteFrame:   500,
	}

	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
		HandlerService:  handlerService,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     markerCache,
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	backend := &mockBackend{}
	manager.SetBackend(backend)
	manager.RegisterHandlers(d)

	// First create a marker so it exists in cache (sync handler)
	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:", Args: []string{}})
	if err != nil {
		t.Fatalf("failed to create marker: %v", err)
	}

	// Now delete it (buffered handler - processes asynchronously)
	_, err = d.Dispatch(dispatcher.Event{Command: ":DELETE:MARKER:", Args: []string{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the buffered handler to process
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		endFrame, ok := backend.deletedMarkers["Projectile#123"]
		backend.mu.Unlock()

		if ok {
			if endFrame != 500 {
				t.Errorf("expected endFrame=500, got %d", endFrame)
			}
			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for marker delete to be processed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

