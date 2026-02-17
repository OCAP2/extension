package worker

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/parser"
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
	chatEvents        []*core.ChatEvent
	radioEvents       []*core.RadioEvent
	telemetryEvents   []*core.TelemetryEvent
	timeStates        []*core.TimeState
	ace3Deaths        []*core.Ace3DeathEvent
	ace3Uncon         []*core.Ace3UnconsciousEvent
	projectileEvents  []*core.ProjectileEvent
	deletedMarkers    []*core.DeleteMarker
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
	b.soldiers = append(b.soldiers, s)
	return nil
}

func (b *mockBackend) AddVehicle(v *core.Vehicle) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.vehicles = append(b.vehicles, v)
	return nil
}

func (b *mockBackend) AddMarker(m *core.Marker) (uint, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := uint(len(b.markers) + 1)
	b.markers = append(b.markers, m)
	return id, nil
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

func (b *mockBackend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.projectileEvents = append(b.projectileEvents, e)
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

func (b *mockBackend) RecordTelemetryEvent(e *core.TelemetryEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.telemetryEvents = append(b.telemetryEvents, e)
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

func (b *mockBackend) DeleteMarker(dm *core.DeleteMarker) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletedMarkers = append(b.deletedMarkers, dm)
	return nil
}

// errorBackend is a mockBackend that returns an error from AddMarker.
type errorBackend struct {
	mockBackend
	err error
}

func (b *errorBackend) AddMarker(_ *core.Marker) (uint, error) {
	return 0, b.err
}

// addSoldierErrorBackend returns an error from AddSoldier.
type addSoldierErrorBackend struct {
	mockBackend
	err error
}

func (b *addSoldierErrorBackend) AddSoldier(_ *core.Soldier) error {
	return b.err
}

// addVehicleErrorBackend returns an error from AddVehicle.
type addVehicleErrorBackend struct {
	mockBackend
	err error
}

func (b *addVehicleErrorBackend) AddVehicle(_ *core.Vehicle) error {
	return b.err
}

// mockParserService provides a minimal implementation for testing
type mockParserService struct {
	mu sync.Mutex

	// Return values
	mission       core.Mission
	world         core.World
	soldier       core.Soldier
	vehicle       core.Vehicle
	soldierState  core.SoldierState
	vehicleState  core.VehicleState
	projectile    parser.ProjectileEvent
	generalEvent  core.GeneralEvent
	killEvent     parser.KillEvent
	chatEvent     core.ChatEvent
	radioEvent    core.RadioEvent
	telemetryEvent core.TelemetryEvent
	timeState      core.TimeState
	ace3Death     core.Ace3DeathEvent
	ace3Uncon     core.Ace3UnconsciousEvent
	marker        core.Marker
	markerMove    parser.MarkerMove
	deleteMarker *core.DeleteMarker

	// Error simulation
	returnError bool
	errorMsg    string

	// Call tracking
	calls []string
}

func (h *mockParserService) ParseMission(args []string) (core.Mission, core.World, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMission")
	if h.returnError {
		return core.Mission{}, core.World{}, errors.New(h.errorMsg)
	}
	return h.mission, h.world, nil
}

func (h *mockParserService) ParseSoldier(args []string) (core.Soldier, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseSoldier")
	if h.returnError {
		return core.Soldier{}, errors.New(h.errorMsg)
	}
	return h.soldier, nil
}

func (h *mockParserService) ParseVehicle(args []string) (core.Vehicle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseVehicle")
	if h.returnError {
		return core.Vehicle{}, errors.New(h.errorMsg)
	}
	return h.vehicle, nil
}

func (h *mockParserService) ParseSoldierState(args []string) (core.SoldierState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseSoldierState")
	if h.returnError {
		return core.SoldierState{}, errors.New(h.errorMsg)
	}
	return h.soldierState, nil
}

func (h *mockParserService) ParseVehicleState(args []string) (core.VehicleState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseVehicleState")
	if h.returnError {
		return core.VehicleState{}, errors.New(h.errorMsg)
	}
	return h.vehicleState, nil
}

func (h *mockParserService) ParseProjectileEvent(args []string) (parser.ProjectileEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseProjectileEvent")
	if h.returnError {
		return parser.ProjectileEvent{}, errors.New(h.errorMsg)
	}
	return h.projectile, nil
}

func (h *mockParserService) ParseGeneralEvent(args []string) (core.GeneralEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseGeneralEvent")
	if h.returnError {
		return core.GeneralEvent{}, errors.New(h.errorMsg)
	}
	return h.generalEvent, nil
}

func (h *mockParserService) ParseKillEvent(args []string) (parser.KillEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseKillEvent")
	if h.returnError {
		return parser.KillEvent{}, errors.New(h.errorMsg)
	}
	return h.killEvent, nil
}

func (h *mockParserService) ParseChatEvent(args []string) (core.ChatEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseChatEvent")
	if h.returnError {
		return core.ChatEvent{}, errors.New(h.errorMsg)
	}
	return h.chatEvent, nil
}

func (h *mockParserService) ParseRadioEvent(args []string) (core.RadioEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseRadioEvent")
	if h.returnError {
		return core.RadioEvent{}, errors.New(h.errorMsg)
	}
	return h.radioEvent, nil
}

func (h *mockParserService) ParseTelemetryEvent(args []string) (core.TelemetryEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseTelemetryEvent")
	if h.returnError {
		return core.TelemetryEvent{}, errors.New(h.errorMsg)
	}
	return h.telemetryEvent, nil
}

func (h *mockParserService) ParseTimeState(args []string) (core.TimeState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseTimeState")
	if h.returnError {
		return core.TimeState{}, errors.New(h.errorMsg)
	}
	return h.timeState, nil
}

func (h *mockParserService) ParseAce3DeathEvent(args []string) (core.Ace3DeathEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseAce3DeathEvent")
	if h.returnError {
		return core.Ace3DeathEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Death, nil
}

func (h *mockParserService) ParseAce3UnconsciousEvent(args []string) (core.Ace3UnconsciousEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseAce3UnconsciousEvent")
	if h.returnError {
		return core.Ace3UnconsciousEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Uncon, nil
}

func (h *mockParserService) ParseMarkerCreate(args []string) (core.Marker, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerCreate")
	if h.returnError {
		return core.Marker{}, errors.New(h.errorMsg)
	}
	return h.marker, nil
}

func (h *mockParserService) ParseMarkerMove(args []string) (parser.MarkerMove, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerMove")
	if h.returnError {
		return parser.MarkerMove{}, errors.New(h.errorMsg)
	}
	return h.markerMove, nil
}

func (h *mockParserService) ParseMarkerDelete(args []string) (*core.DeleteMarker, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerDelete")
	if h.returnError {
		return nil, errors.New(h.errorMsg)
	}
	return h.deleteMarker, nil
}

// waitFor polls until check returns true or times out after 2s.
func waitFor(t *testing.T, check func() bool, msg string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if check() {
			return
		}
		select {
		case <-deadline:
			require.Fail(t, msg)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// waitForLogMsg waits until a message containing substr appears in the mock logger.
func waitForLogMsg(t *testing.T, logger *mockLogger, substr string) {
	t.Helper()
	waitFor(t, func() bool {
		logger.mu.Lock()
		defer logger.mu.Unlock()
		for _, msg := range logger.messages {
			if msg == substr {
				return true
			}
		}
		return false
	}, "timed out waiting for log message: "+substr)
}

func newTestDispatcher(t *testing.T) (*dispatcher.Dispatcher, *mockLogger) {
	logger := &mockLogger{}

	d, err := dispatcher.New(logger)
	require.NoError(t, err, "failed to create dispatcher")

	return d, logger
}

func TestRegisterHandlers_RegistersAllCommands(t *testing.T) {
	d, _ := newTestDispatcher(t)

	backend := &mockBackend{}
	manager := NewManager(Dependencies{}, backend)

	manager.RegisterHandlers(d)

	expectedCommands := []string{
		":NEW:SOLDIER:",
		":NEW:VEHICLE:",
		":NEW:SOLDIER:STATE:",
		":NEW:VEHICLE:STATE:",
		":NEW:TIME:STATE:",
		":PROJECTILE:",
		":KILL:",
		":EVENT:",
		":CHAT:",
		":RADIO:",
		":TELEMETRY:",
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

func TestNewManager(t *testing.T) {
	backend := &mockBackend{}
	manager := NewManager(Dependencies{}, backend)

	require.NotNil(t, manager, "expected non-nil manager")
}

func TestHandleNewSoldier_CachesEntityWithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		soldier: core.Soldier{ID: 42, UnitName: "Test Soldier"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
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

	parserService := &mockParserService{
		vehicle: core.Vehicle{ID: 99, OcapType: "car"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
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

func TestHandleSoldierState_ValidatesAndFillsGroupSide(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	// Pre-cache a soldier with GroupID and Side
	entityCache.AddSoldier(core.Soldier{ID: 10, GroupID: "Alpha 1", Side: "WEST"})

	parserService := &mockParserService{
		soldierState: core.SoldierState{
			SoldierID: 10,
			GroupID:   "", // empty - should be filled from cache
			Side:      "", // empty - should be filled from cache
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for buffered handler
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		n := len(backend.soldierStates)
		backend.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for soldier state")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.soldierStates))
	assert.Equal(t, "Alpha 1", backend.soldierStates[0].GroupID)
	assert.Equal(t, "WEST", backend.soldierStates[0].Side)
}

func TestHandleSoldierState_ReturnsErrorWhenNotCached(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// No soldier cached

	parserService := &mockParserService{
		soldierState: core.SoldierState{SoldierID: 999},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err) // dispatch itself doesn't error for buffered handlers

	waitForLogMsg(t, logger, "buffered handler failed")

	// Backend should have no states since the soldier wasn't cached
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.soldierStates), "state should not be recorded when soldier not cached")
}

func TestHandleVehicleState_ReturnsErrorWhenNotCached(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		vehicleState: core.VehicleState{VehicleID: 888},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "buffered handler failed")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.vehicleStates), "state should not be recorded when vehicle not cached")
}

func TestHandleKillEvent_ClassifiesVictimAndKiller(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	// Soldier victim, vehicle killer
	entityCache.AddSoldier(core.Soldier{ID: 5})
	entityCache.AddVehicle(core.Vehicle{ID: 20})

	parserService := &mockParserService{
		killEvent: parser.KillEvent{
			CaptureFrame: 100,
			VictimID:     5,
			KillerID:     20,
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":KILL:", Args: []string{}})
	require.NoError(t, err)

	// Wait for buffered handler
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		n := len(backend.killEvents)
		backend.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for kill event")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.killEvents))
	ke := backend.killEvents[0]

	// Victim is a soldier
	require.NotNil(t, ke.VictimSoldierID)
	assert.Equal(t, uint(5), *ke.VictimSoldierID)
	assert.Nil(t, ke.VictimVehicleID)

	// Killer is a vehicle
	assert.Nil(t, ke.KillerSoldierID)
	require.NotNil(t, ke.KillerVehicleID)
	assert.Equal(t, uint(20), *ke.KillerVehicleID)
}

func TestHandleProjectile_ClassifiesHitParts(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	entityCache.AddSoldier(core.Soldier{ID: 7})
	entityCache.AddVehicle(core.Vehicle{ID: 30})

	parserService := &mockParserService{
		projectile: parser.ProjectileEvent{
			FirerObjectID: 1,
			CaptureFrame:  620,
			Trajectory: []core.TrajectoryPoint{
				{Position: core.Position3D{X: 6456.5, Y: 5345.7, Z: 10.0}, Frame: 620},
				{Position: core.Position3D{X: 6448.0, Y: 5337.0, Z: 15.0}, Frame: 625},
			},
			HitParts: []parser.HitPart{
				{EntityID: 7, ComponentsHit: []string{"head"}, CaptureFrame: 625},
				{EntityID: 30, ComponentsHit: []string{"hull"}, CaptureFrame: 626},
			},
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		MarkerCache:   cache.NewMarkerCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":PROJECTILE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for buffered handler
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		n := len(backend.projectileEvents)
		backend.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for projectile event")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.projectileEvents))
	pe := backend.projectileEvents[0]

	// Verify hit classification via core Hits (merged from HitSoldiers + HitVehicles)
	require.Len(t, pe.Hits, 2, "expected 2 hits")

	// Find the soldier hit and vehicle hit
	var soldierHit, vehicleHit *core.ProjectileHit
	for i := range pe.Hits {
		if pe.Hits[i].SoldierID != nil {
			soldierHit = &pe.Hits[i]
		}
		if pe.Hits[i].VehicleID != nil {
			vehicleHit = &pe.Hits[i]
		}
	}
	require.NotNil(t, soldierHit, "expected a soldier hit")
	require.NotNil(t, vehicleHit, "expected a vehicle hit")
	assert.Equal(t, uint16(7), *soldierHit.SoldierID)
	assert.Equal(t, uint16(30), *vehicleHit.VehicleID)
}

func TestHandleProjectile_RecordsProjectileEvent(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{
		projectile: parser.ProjectileEvent{
			FirerObjectID:   1,
			CaptureFrame:    620,
			WeaponDisplay:   "Throw",
			MagazineDisplay: "RGO Grenade",
			MagazineIcon:    `\A3\Weapons_F\Data\UI\gear_M67_CA.paa`,
			SimulationType:  "shotShell",
			Trajectory: []core.TrajectoryPoint{
				{Position: core.Position3D{X: 6456.5, Y: 5345.7, Z: 10.443}, Frame: 620},
				{Position: core.Position3D{X: 6448.0, Y: 5337.0, Z: 15.0}, Frame: 625},
				{Position: core.Position3D{X: 6441.55, Y: 5328.46, Z: 9.88}, Frame: 630},
			},
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   cache.NewMarkerCache(),
	}, backend)
	manager.RegisterHandlers(d)

	// Dispatch a projectile event
	_, err := d.Dispatch(dispatcher.Event{Command: ":PROJECTILE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for the buffered handler to process
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		numEvents := len(backend.projectileEvents)
		backend.mu.Unlock()

		if numEvents > 0 {
			break
		}

		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for projectile event to be processed")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()

	// Verify a core.ProjectileEvent was recorded
	require.Equal(t, 1, len(backend.projectileEvents), "expected 1 projectile event")
	pe := backend.projectileEvents[0]
	assert.Equal(t, uint16(1), pe.FirerObjectID)
	assert.Equal(t, uint(620), pe.CaptureFrame)
	assert.Equal(t, "RGO Grenade", pe.MagazineDisplay)

	// Trajectory should have 3 points
	require.Len(t, pe.Trajectory, 3)
	assert.Equal(t, 6456.5, pe.Trajectory[0].Position.X)
	assert.Equal(t, uint(620), pe.Trajectory[0].Frame)
	assert.Equal(t, 6441.55, pe.Trajectory[2].Position.X)
	assert.Equal(t, uint(630), pe.Trajectory[2].Frame)

	// No markers or fired events should be created
	assert.Empty(t, backend.markers, "dispatch should not create markers")
	assert.Empty(t, backend.firedEvents, "dispatch should not create fired events")
	assert.Empty(t, backend.hitEvents, "dispatch should not create hit events")
}

func TestHandleMarkerMove_ResolvesMarkerName(t *testing.T) {
	d, _ := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()
	markerCache.Set("marker_alpha", 42)

	parserService := &mockParserService{
		markerMove: parser.MarkerMove{
			CaptureFrame: 100,
			MarkerName:   "marker_alpha",
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   markerCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:STATE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for buffered handler
	deadline := time.After(2 * time.Second)
	for {
		backend.mu.Lock()
		n := len(backend.markerStates)
		backend.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			require.Fail(t, "timed out waiting for marker state")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.markerStates))
	assert.Equal(t, uint(42), backend.markerStates[0].MarkerID)
}

func TestHandleChatEvent_ValidatesSender(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// No soldier cached — sender validation should fail

	senderID := uint(5)
	parserService := &mockParserService{
		chatEvent: core.ChatEvent{
			SoldierID: &senderID,
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":CHAT:", Args: []string{}})
	require.NoError(t, err) // buffered dispatch doesn't return handler errors

	waitForLogMsg(t, logger, "event failed")

	// Chat event should not be recorded since sender doesn't exist
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.chatEvents), "chat event should not be recorded when sender not cached")
}

func TestHandleMarkerDelete_WithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()

	parserService := &mockParserService{
		marker:       core.Marker{MarkerName: "Projectile#123"},
		deleteMarker: &core.DeleteMarker{Name: "Projectile#123", EndFrame: 500},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   markerCache,
	}, backend)
	manager.RegisterHandlers(d)

	// First create a marker so it exists in cache (sync handler)
	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:", Args: []string{}})
	require.NoError(t, err, "failed to create marker")

	// Now delete it (buffered handler - processes asynchronously)
	_, err = d.Dispatch(dispatcher.Event{Command: ":DELETE:MARKER:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.deletedMarkers)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for marker delete")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Len(t, backend.deletedMarkers, 1)
	assert.Equal(t, "Projectile#123", backend.deletedMarkers[0].Name)
	assert.Equal(t, uint(500), backend.deletedMarkers[0].EndFrame)
}

func TestHandleVehicleState_HappyPath(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddVehicle(core.Vehicle{ID: 50, OcapType: "car"})

	parserService := &mockParserService{
		vehicleState: core.VehicleState{VehicleID: 50, CaptureFrame: 10},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.vehicleStates)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for vehicle state")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.vehicleStates))
	assert.Equal(t, uint16(50), backend.vehicleStates[0].VehicleID)
}

func TestHandleTimeState(t *testing.T) {
	d, _ := newTestDispatcher(t)

	now := time.Now()
	parserService := &mockParserService{
		timeState: core.TimeState{CaptureFrame: 200, Time: now, MissionTime: 600.0},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:TIME:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.timeStates)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for time state")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.timeStates))
	assert.Equal(t, uint(200), backend.timeStates[0].CaptureFrame)
}

func TestHandleGeneralEvent(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{
		generalEvent: core.GeneralEvent{CaptureFrame: 300, Name: "connected", Message: "Player joined"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":EVENT:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.generalEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for general event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.generalEvents))
	assert.Equal(t, "connected", backend.generalEvents[0].Name)
}

func TestHandleRadioEvent_ValidSender(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 3})

	senderID := uint(3)
	parserService := &mockParserService{
		radioEvent: core.RadioEvent{SoldierID: &senderID, CaptureFrame: 400, Radio: "ACRE_PRC152"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":RADIO:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.radioEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for radio event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.radioEvents))
	assert.Equal(t, "ACRE_PRC152", backend.radioEvents[0].Radio)
}

func TestHandleTelemetryEvent(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{
		telemetryEvent: core.TelemetryEvent{
			CaptureFrame: 500,
			FpsAverage:   45.5,
			FpsMin:       30.0,
			GlobalCounts: core.GlobalEntityCount{
				UnitsAlive: 20, PlayersConnected: 8,
			},
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":TELEMETRY:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.telemetryEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for telemetry event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.telemetryEvents))
	assert.Equal(t, float32(45.5), backend.telemetryEvents[0].FpsAverage)
	assert.Equal(t, uint(20), backend.telemetryEvents[0].GlobalCounts.UnitsAlive)
}

func TestHandleAce3DeathEvent(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 8})
	entityCache.AddSoldier(core.Soldier{ID: 9})

	sourceID := uint(9)
	parserService := &mockParserService{
		ace3Death: core.Ace3DeathEvent{
			SoldierID:          8,
			CaptureFrame:       600,
			Reason:             "cardiac_arrest",
			LastDamageSourceID: &sourceID,
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		LogManager:    logging.NewSlogManager(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.ace3Deaths)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for ace3 death event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.ace3Deaths))
	assert.Equal(t, uint(8), backend.ace3Deaths[0].SoldierID)
	assert.Equal(t, "cardiac_arrest", backend.ace3Deaths[0].Reason)
}

func TestHandleAce3DeathEvent_VehicleDamageSource(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 8})
	entityCache.AddVehicle(core.Vehicle{ID: 15}) // damage source is a vehicle

	sourceID := uint(15)
	parserService := &mockParserService{
		ace3Death: core.Ace3DeathEvent{
			SoldierID:          8,
			CaptureFrame:       601,
			Reason:             "bleedout",
			LastDamageSourceID: &sourceID,
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		LogManager:    logging.NewSlogManager(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.ace3Deaths)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for ace3 death event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.ace3Deaths))
}

func TestHandleAce3UnconsciousEvent(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 12})

	parserService := &mockParserService{
		ace3Uncon: core.Ace3UnconsciousEvent{SoldierID: 12, CaptureFrame: 700, IsUnconscious: true},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:UNCONSCIOUS:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.ace3Uncon)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for ace3 unconscious event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.ace3Uncon))
	assert.Equal(t, true, backend.ace3Uncon[0].IsUnconscious)
}

func TestHandleChatEvent_ValidSender(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 5})

	senderID := uint(5)
	parserService := &mockParserService{
		chatEvent: core.ChatEvent{SoldierID: &senderID, CaptureFrame: 800, Channel: "side", Message: "Hello"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":CHAT:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.chatEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for chat event")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.chatEvents))
	assert.Equal(t, "Hello", backend.chatEvents[0].Message)
}

func TestHandleMarkerCreate_BackendError(t *testing.T) {
	d, _ := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()

	parserService := &mockParserService{
		marker: core.Marker{MarkerName: "test_marker"},
	}

	backend := &errorBackend{err: errors.New("db failure")}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   markerCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add marker")

	// Marker should NOT be in cache since backend failed
	_, found := markerCache.Get("test_marker")
	assert.False(t, found, "marker should not be cached when backend fails")
}

func TestHandleSoldierState_UpdatesCacheWhenPlayerTakesOver(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	// Pre-cache an AI soldier (IsPlayer=false)
	entityCache.AddSoldier(core.Soldier{ID: 10, UnitName: "Habibzai", GroupID: "Alpha 1", Side: "EAST", IsPlayer: false})

	parserService := &mockParserService{
		soldierState: core.SoldierState{
			SoldierID: 10,
			UnitName:  "zigster",
			IsPlayer:  true,
			GroupID:   "Alpha 1",
			Side:      "EAST",
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err)

	// Wait for buffered handler
	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.soldierStates)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for soldier state")

	// Verify cache was updated: once a player, always a player
	cachedSoldier, found := entityCache.GetSoldier(10)
	require.True(t, found)
	assert.True(t, cachedSoldier.IsPlayer, "cached soldier should be marked as player after takeover")
	assert.Equal(t, "zigster", cachedSoldier.UnitName, "cached soldier name should be updated to player name")
}

func TestClassifyEntity_Vehicle(t *testing.T) {
	entityCache := cache.NewEntityCache()
	entityCache.AddVehicle(core.Vehicle{ID: 25})

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		EntityCache: entityCache,
		LogManager:  logging.NewSlogManager(),
	}, backend)

	soldierID, vehicleID := manager.classifyEntity(25)
	assert.Nil(t, soldierID)
	require.NotNil(t, vehicleID)
	assert.Equal(t, uint(25), *vehicleID)
}

func TestClassifyEntity_NotFound(t *testing.T) {
	entityCache := cache.NewEntityCache()

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		EntityCache: entityCache,
		LogManager:  logging.NewSlogManager(),
	}, backend)

	soldierID, vehicleID := manager.classifyEntity(999)
	assert.Nil(t, soldierID)
	assert.Nil(t, vehicleID)
}

func TestClassifyEntity_Soldier(t *testing.T) {
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 7})

	manager := NewManager(Dependencies{
		EntityCache: entityCache,
		LogManager:  logging.NewSlogManager(),
	}, &mockBackend{})

	soldierID, vehicleID := manager.classifyEntity(7)
	require.NotNil(t, soldierID)
	assert.Equal(t, uint(7), *soldierID)
	assert.Nil(t, vehicleID)
}

func TestClassifyHitParts_EntityNotFound(t *testing.T) {
	entityCache := cache.NewEntityCache()
	// Entity 99 is not in either cache

	manager := NewManager(Dependencies{
		EntityCache: entityCache,
		LogManager:  logging.NewSlogManager(),
	}, &mockBackend{})

	hits := manager.classifyHitParts([]parser.HitPart{
		{EntityID: 99, ComponentsHit: []string{"body"}, CaptureFrame: 100},
	})
	assert.Empty(t, hits, "unknown entity should be skipped")
}

// --- Parser error tests for sync handlers ---

func TestHandleNewSoldier_ParserError(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad soldier data"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to log new soldier")
}

func TestHandleNewVehicle_ParserError(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad vehicle data"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to log new vehicle")
}

func TestHandleMarkerCreate_ParserError(t *testing.T) {
	d, _ := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad marker data"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   cache.NewMarkerCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create marker")
}

// --- Backend error tests for sync handlers ---

func TestHandleNewSoldier_BackendError(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		soldier: core.Soldier{ID: 42, UnitName: "Test"},
	}
	backend := &addSoldierErrorBackend{err: errors.New("db write failed")}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add soldier")

	// Soldier should still be in cache (cached before backend call)
	_, found := entityCache.GetSoldier(42)
	assert.True(t, found, "soldier should be in cache even when backend fails")
}

func TestHandleNewVehicle_BackendError(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		vehicle: core.Vehicle{ID: 99, OcapType: "tank"},
	}
	backend := &addVehicleErrorBackend{err: errors.New("db write failed")}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:", Args: []string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add vehicle")

	// Vehicle should still be in cache
	_, found := entityCache.GetVehicle(99)
	assert.True(t, found, "vehicle should be in cache even when backend fails")
}

// --- Parser error tests for buffered handlers ---

func TestHandleSoldierState_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad state"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err) // buffered dispatch doesn't return handler errors

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.soldierStates)
}

func TestHandleVehicleState_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad state"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.vehicleStates)
}

func TestHandleTimeState_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad time"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:TIME:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.timeStates)
}

func TestHandleProjectileEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad projectile"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":PROJECTILE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.projectileEvents)
}

func TestHandleGeneralEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad event"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":EVENT:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.generalEvents)
}

func TestHandleKillEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad kill"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":KILL:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.killEvents)
}

func TestHandleChatEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad chat"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":CHAT:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.chatEvents)
}

func TestHandleRadioEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad radio"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":RADIO:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.radioEvents)
}

func TestHandleTelemetryEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad telemetry"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":TELEMETRY:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.telemetryEvents)
}

func TestHandleAce3DeathEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad death"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.ace3Deaths)
}

func TestHandleAce3UnconsciousEvent_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad uncon"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:UNCONSCIOUS:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.ace3Uncon)
}

func TestHandleMarkerMove_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad marker move"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   cache.NewMarkerCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.markerStates)
}

func TestHandleMarkerDelete_ParserError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{returnError: true, errorMsg: "bad marker delete"}
	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   cache.NewMarkerCache(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":DELETE:MARKER:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.deletedMarkers)
}

// --- Edge case tests ---

func TestHandleChatEvent_NilSoldierID(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		chatEvent: core.ChatEvent{SoldierID: nil, CaptureFrame: 100, Channel: "system", Message: "Server message"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":CHAT:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.chatEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for chat event with nil sender")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.chatEvents))
	assert.Equal(t, "Server message", backend.chatEvents[0].Message)
}

func TestHandleRadioEvent_NilSoldierID(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		radioEvent: core.RadioEvent{SoldierID: nil, CaptureFrame: 100, Radio: "system"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":RADIO:", Args: []string{}})
	require.NoError(t, err)

	waitFor(t, func() bool {
		backend.mu.Lock()
		n := len(backend.radioEvents)
		backend.mu.Unlock()
		return n > 0
	}, "timed out waiting for radio event with nil sender")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, 1, len(backend.radioEvents))
}

func TestHandleRadioEvent_SenderNotCached(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	senderID := uint(77)
	parserService := &mockParserService{
		radioEvent: core.RadioEvent{SoldierID: &senderID, CaptureFrame: 100},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":RADIO:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.radioEvents, "radio event should not be recorded when sender not cached")
}

func TestHandleMarkerMove_MarkerNotInCache(t *testing.T) {
	d, logger := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()
	// marker_beta is NOT in cache

	parserService := &mockParserService{
		markerMove: parser.MarkerMove{
			CaptureFrame: 200,
			MarkerName:   "marker_beta",
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   markerCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:STATE:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.markerStates, "marker state should not be recorded when marker not in cache")
}

func TestHandleAce3DeathEvent_SoldierNotCached(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// Soldier 50 is NOT in cache

	parserService := &mockParserService{
		ace3Death: core.Ace3DeathEvent{SoldierID: 50, CaptureFrame: 100, Reason: "bleedout"},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		LogManager:    logging.NewSlogManager(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.ace3Deaths, "ace3 death should not be recorded when soldier not cached")
}

func TestHandleAce3DeathEvent_DamageSourceNotFound(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 8}) // victim exists
	// But damage source 99 is NOT in either cache

	sourceID := uint(99)
	parserService := &mockParserService{
		ace3Death: core.Ace3DeathEvent{
			SoldierID:          8,
			CaptureFrame:       100,
			Reason:             "bleedout",
			LastDamageSourceID: &sourceID,
		},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		LogManager:    logging.NewSlogManager(),
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.ace3Deaths, "ace3 death should not be recorded when damage source not found")
}

func TestHandleAce3UnconsciousEvent_SoldierNotCached(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// Soldier 60 is NOT in cache

	parserService := &mockParserService{
		ace3Uncon: core.Ace3UnconsciousEvent{SoldierID: 60, CaptureFrame: 100, IsUnconscious: true},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:UNCONSCIOUS:", Args: []string{}})
	require.NoError(t, err)

	waitForLogMsg(t, logger, "event failed")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.ace3Uncon, "ace3 unconscious should not be recorded when soldier not cached")
}

// --- Backend Record* error backends ---

type recordSoldierStateErrorBackend struct {
	mockBackend
}

func (b *recordSoldierStateErrorBackend) RecordSoldierState(_ *core.SoldierState) error {
	return errors.New("record soldier state failed")
}

type recordVehicleStateErrorBackend struct {
	mockBackend
}

func (b *recordVehicleStateErrorBackend) RecordVehicleState(_ *core.VehicleState) error {
	return errors.New("record vehicle state failed")
}

type recordTimeStateErrorBackend struct {
	mockBackend
}

func (b *recordTimeStateErrorBackend) RecordTimeState(_ *core.TimeState) error {
	return errors.New("record time state failed")
}

type recordProjectileEventErrorBackend struct {
	mockBackend
}

func (b *recordProjectileEventErrorBackend) RecordProjectileEvent(_ *core.ProjectileEvent) error {
	return errors.New("record projectile event failed")
}

type recordGeneralEventErrorBackend struct {
	mockBackend
}

func (b *recordGeneralEventErrorBackend) RecordGeneralEvent(_ *core.GeneralEvent) error {
	return errors.New("record general event failed")
}

type recordKillEventErrorBackend struct {
	mockBackend
}

func (b *recordKillEventErrorBackend) RecordKillEvent(_ *core.KillEvent) error {
	return errors.New("record kill event failed")
}

type recordChatEventErrorBackend struct {
	mockBackend
}

func (b *recordChatEventErrorBackend) RecordChatEvent(_ *core.ChatEvent) error {
	return errors.New("record chat event failed")
}

type recordRadioEventErrorBackend struct {
	mockBackend
}

func (b *recordRadioEventErrorBackend) RecordRadioEvent(_ *core.RadioEvent) error {
	return errors.New("record radio event failed")
}

type recordTelemetryEventErrorBackend struct {
	mockBackend
}

func (b *recordTelemetryEventErrorBackend) RecordTelemetryEvent(_ *core.TelemetryEvent) error {
	return errors.New("record telemetry event failed")
}

type recordAce3DeathEventErrorBackend struct {
	mockBackend
}

func (b *recordAce3DeathEventErrorBackend) RecordAce3DeathEvent(_ *core.Ace3DeathEvent) error {
	return errors.New("record ace3 death event failed")
}

type recordAce3UnconsciousEventErrorBackend struct {
	mockBackend
}

func (b *recordAce3UnconsciousEventErrorBackend) RecordAce3UnconsciousEvent(_ *core.Ace3UnconsciousEvent) error {
	return errors.New("record ace3 unconscious event failed")
}

type recordMarkerStateErrorBackend struct {
	mockBackend
}

func (b *recordMarkerStateErrorBackend) RecordMarkerState(_ *core.MarkerState) error {
	return errors.New("record marker state failed")
}

type deleteMarkerErrorBackend struct {
	mockBackend
}

func (b *deleteMarkerErrorBackend) DeleteMarker(_ *core.DeleteMarker) error {
	return errors.New("delete marker failed")
}

// --- Backend Record* error tests ---

func TestHandleSoldierState_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 10, GroupID: "Alpha", Side: "WEST"})

	parserService := &mockParserService{
		soldierState: core.SoldierState{SoldierID: 10},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, &recordSoldierStateErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleVehicleState_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddVehicle(core.Vehicle{ID: 50})

	parserService := &mockParserService{
		vehicleState: core.VehicleState{VehicleID: 50},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, &recordVehicleStateErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:STATE:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleTimeState_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		timeState: core.TimeState{CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, &recordTimeStateErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:TIME:STATE:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleProjectileEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		projectile: parser.ProjectileEvent{CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, &recordProjectileEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":PROJECTILE:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleGeneralEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		generalEvent: core.GeneralEvent{CaptureFrame: 100, Name: "test"},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, &recordGeneralEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":EVENT:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleKillEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		killEvent: parser.KillEvent{CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, &recordKillEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":KILL:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleChatEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 5})

	senderID := uint(5)
	parserService := &mockParserService{
		chatEvent: core.ChatEvent{SoldierID: &senderID, CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, &recordChatEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":CHAT:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleRadioEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 3})

	senderID := uint(3)
	parserService := &mockParserService{
		radioEvent: core.RadioEvent{SoldierID: &senderID, CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, &recordRadioEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":RADIO:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleTelemetryEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		telemetryEvent: core.TelemetryEvent{CaptureFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
	}, &recordTelemetryEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":TELEMETRY:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleAce3DeathEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 8})

	parserService := &mockParserService{
		ace3Death: core.Ace3DeathEvent{SoldierID: 8, CaptureFrame: 100, Reason: "bleedout"},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
		LogManager:    logging.NewSlogManager(),
	}, &recordAce3DeathEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:DEATH:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleAce3UnconsciousEvent_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	entityCache.AddSoldier(core.Soldier{ID: 12})

	parserService := &mockParserService{
		ace3Uncon: core.Ace3UnconsciousEvent{SoldierID: 12, CaptureFrame: 100, IsUnconscious: true},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, &recordAce3UnconsciousEventErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":ACE3:UNCONSCIOUS:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleMarkerMove_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()
	markerCache.Set("marker_alpha", 42)

	parserService := &mockParserService{
		markerMove: parser.MarkerMove{CaptureFrame: 100, MarkerName: "marker_alpha"},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   markerCache,
	}, &recordMarkerStateErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:MARKER:STATE:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}

func TestHandleMarkerDelete_BackendError(t *testing.T) {
	d, logger := newTestDispatcher(t)

	parserService := &mockParserService{
		deleteMarker: &core.DeleteMarker{Name: "marker_1", EndFrame: 100},
	}

	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   cache.NewEntityCache(),
		MarkerCache:   cache.NewMarkerCache(),
	}, &deleteMarkerErrorBackend{})
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":DELETE:MARKER:", Args: []string{}})
	require.NoError(t, err)
	waitForLogMsg(t, logger, "event failed")
}
