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
	geom "github.com/peterstace/simplefeatures/geom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	projectile   model.ProjectileEvent
	generalEvent model.GeneralEvent
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
	require.NoError(t, err, "failed to create dispatcher")

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
		":PROJECTILE:",
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
		assert.True(t, d.HasHandler(cmd), "expected handler for %s to be registered", cmd)
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

	assert.NoError(t, err)
	assert.Nil(t, result, "expected nil result when no database/backend")
}

func TestSetBackend(t *testing.T) {
	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	assert.False(t, manager.hasBackend(), "expected no backend initially")

	backend := &mockBackend{}
	manager.SetBackend(backend)

	assert.True(t, manager.hasBackend(), "expected backend to be set")
}

func TestNewQueues(t *testing.T) {
	queues := NewQueues()

	require.NotNil(t, queues, "expected non-nil queues")
	assert.NotNil(t, queues.Soldiers, "expected Soldiers queue to be initialized")
	assert.NotNil(t, queues.SoldierStates, "expected SoldierStates queue to be initialized")
	assert.NotNil(t, queues.Vehicles, "expected Vehicles queue to be initialized")
	assert.NotNil(t, queues.VehicleStates, "expected VehicleStates queue to be initialized")
	assert.NotNil(t, queues.ProjectileEvents, "expected ProjectileEvents queue to be initialized")
	assert.NotNil(t, queues.GeneralEvents, "expected GeneralEvents queue to be initialized")
	assert.NotNil(t, queues.KillEvents, "expected KillEvents queue to be initialized")
	assert.NotNil(t, queues.ChatEvents, "expected ChatEvents queue to be initialized")
	assert.NotNil(t, queues.RadioEvents, "expected RadioEvents queue to be initialized")
	assert.NotNil(t, queues.FpsEvents, "expected FpsEvents queue to be initialized")
	assert.NotNil(t, queues.Ace3DeathEvents, "expected Ace3DeathEvents queue to be initialized")
	assert.NotNil(t, queues.Ace3UnconsciousEvents, "expected Ace3UnconsciousEvents queue to be initialized")
	assert.NotNil(t, queues.Markers, "expected Markers queue to be initialized")
	assert.NotNil(t, queues.MarkerStates, "expected MarkerStates queue to be initialized")
}

func TestNewManager(t *testing.T) {
	deps := Dependencies{
		IsDatabaseValid: func() bool { return true },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
	}
	queues := NewQueues()

	manager := NewManager(deps, queues)

	require.NotNil(t, manager, "expected non-nil manager")
	assert.False(t, manager.hasBackend(), "expected no backend initially")
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

	require.NoError(t, err)
	assert.Nil(t, result, "expected nil result")

	// Verify soldier is in backend
	assert.Equal(t, 1, len(backend.soldiers), "expected 1 soldier in backend")

	// Verify soldier is also in EntityCache
	cachedSoldier, found := entityCache.GetSoldier(42)
	assert.True(t, found, "expected soldier to be cached in EntityCache")
	assert.Equal(t, "Test Soldier", cachedSoldier.UnitName)
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

	require.NoError(t, err)
	assert.Nil(t, result, "expected nil result")

	// Verify vehicle is in backend
	assert.Equal(t, 1, len(backend.vehicles), "expected 1 vehicle in backend")

	// Verify vehicle is also in EntityCache
	cachedVehicle, found := entityCache.GetVehicle(99)
	assert.True(t, found, "expected vehicle to be cached in EntityCache")
	assert.Equal(t, "car", cachedVehicle.OcapType)
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

	require.NoError(t, err)

	// Verify soldier is in queue
	assert.Equal(t, 1, queues.Soldiers.Len(), "expected 1 soldier in queue")

	// Verify soldier is also in EntityCache
	cachedSoldier, found := entityCache.GetSoldier(55)
	assert.True(t, found, "expected soldier to be cached in EntityCache even when using queue")
	assert.Equal(t, "Queue Soldier", cachedSoldier.UnitName)
}

func TestHandleProjectile_ThrownGrenade_MarkerStatesUseBackendID(t *testing.T) {
	d, _ := newTestDispatcher(t)

	// Create a LineStringZM with 3 positions (thrown, mid-flight, impact)
	// Format: [x, y, z, frameNo] where M = frame number
	coords := []float64{
		6456.5, 5345.7, 10.443, 620.0, // thrown position at frame 620
		6448.0, 5337.0, 15.0, 625.0,   // mid-flight at frame 625
		6441.55, 5328.46, 9.88, 630.0,  // impact position at frame 630
	}
	seq := geom.NewSequence(coords, geom.DimXYZM)
	ls := geom.NewLineString(seq)

	handlerService := &mockHandlerService{
		projectile: model.ProjectileEvent{
			MissionID:       1,
			FirerObjectID:   1,
			CaptureFrame:    620,
			Weapon:          "throw",
			WeaponDisplay:   "Throw",
			MagazineDisplay: "RGO Grenade",
			MagazineIcon:    `\A3\Weapons_F\Data\UI\gear_M67_CA.paa`,
			Mode:            "HandGrenadeMuzzle",
			Positions:       ls.AsGeometry(),
		},
	}

	deps := Dependencies{
		IsDatabaseValid: func() bool { return false },
		ShouldSaveLocal: func() bool { return false },
		DBInsertsPaused: func() bool { return false },
		HandlerService:  handlerService,
		EntityCache:     cache.NewEntityCache(),
		MarkerCache:     cache.NewMarkerCache(),
	}
	queues := NewQueues()
	manager := NewManager(deps, queues)

	backend := &mockBackend{}
	manager.SetBackend(backend)
	manager.RegisterHandlers(d)

	// Dispatch a thrown grenade projectile event
	_, err := d.Dispatch(dispatcher.Event{Command: ":PROJECTILE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for the buffered handler to process
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		numMarkers := len(backend.markers)
		numStates := len(backend.markerStates)
		backend.mu.Unlock()

		if numMarkers > 0 && numStates > 0 {
			break
		}

		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for projectile marker to be processed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	// Verify marker was created
	require.Equal(t, 1, len(backend.markers), "expected 1 marker")
	marker := backend.markers[0]
	assert.Equal(t, "RGO Grenade", marker.Text)
	assert.Equal(t, "magIcons/gear_M67_CA.paa", marker.MarkerType)

	// Verify marker states have the backend-assigned ID (not the pre-computed one)
	// This is the core of the test: AddMarker assigns a new ID, and states must match it
	require.Equal(t, 2, len(backend.markerStates), "expected 2 marker states (positions 2 and 3)")
	for i, state := range backend.markerStates {
		assert.Equal(t, marker.ID, state.MarkerID,
			"marker state %d MarkerID should match backend-assigned marker ID", i)
	}

	// Verify state positions match the trajectory
	assert.Equal(t, 6448.0, backend.markerStates[0].Position.X)
	assert.Equal(t, 6441.55, backend.markerStates[1].Position.X)
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
	require.NoError(t, err, "failed to create marker")

	// Now delete it (buffered handler - processes asynchronously)
	_, err = d.Dispatch(dispatcher.Event{Command: ":DELETE:MARKER:", Args: []string{}})
	require.NoError(t, err)

	// Wait for the buffered handler to process
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		endFrame, ok := backend.deletedMarkers["Projectile#123"]
		backend.mu.Unlock()

		if ok {
			assert.Equal(t, uint(500), endFrame)
			return
		}

		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for marker delete to be processed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

