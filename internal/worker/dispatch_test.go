package worker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/dispatcher"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/parser"
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
	chatEvents        []*core.ChatEvent
	radioEvents       []*core.RadioEvent
	fpsEvents         []*core.ServerFpsEvent
	timeStates        []*core.TimeState
	ace3Deaths        []*core.Ace3DeathEvent
	ace3Uncon         []*core.Ace3UnconsciousEvent
	projectileEvents  []*core.ProjectileEvent
	deletedMarkers    map[string]uint // name -> endFrame
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
		if s.ID == ocapID {
			return s, true
		}
	}
	return nil, false
}

func (b *mockBackend) GetVehicleByObjectID(ocapID uint16) (*core.Vehicle, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, v := range b.vehicles {
		if v.ID == ocapID {
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

// mockParserService provides a minimal implementation for testing
type mockParserService struct {
	mu sync.Mutex

	// Return values
	mission       model.Mission
	world         model.World
	soldier       model.Soldier
	vehicle       model.Vehicle
	soldierState  model.SoldierState
	vehicleState  model.VehicleState
	projectile    parser.ParsedProjectileEvent
	generalEvent  model.GeneralEvent
	killEvent     parser.ParsedKillEvent
	chatEvent     model.ChatEvent
	radioEvent    model.RadioEvent
	fpsEvent      model.ServerFpsEvent
	timeState     model.TimeState
	ace3Death     model.Ace3DeathEvent
	ace3Uncon     model.Ace3UnconsciousEvent
	marker        model.Marker
	markerMove    parser.ParsedMarkerMove
	deletedMarker string
	deleteFrame   uint

	// Error simulation
	returnError bool
	errorMsg    string

	// Call tracking
	calls []string
}

func (h *mockParserService) ParseMission(args []string) (model.Mission, model.World, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMission")
	if h.returnError {
		return model.Mission{}, model.World{}, errors.New(h.errorMsg)
	}
	return h.mission, h.world, nil
}

func (h *mockParserService) ParseSoldier(args []string) (model.Soldier, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseSoldier")
	if h.returnError {
		return model.Soldier{}, errors.New(h.errorMsg)
	}
	return h.soldier, nil
}

func (h *mockParserService) ParseVehicle(args []string) (model.Vehicle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseVehicle")
	if h.returnError {
		return model.Vehicle{}, errors.New(h.errorMsg)
	}
	return h.vehicle, nil
}

func (h *mockParserService) ParseSoldierState(args []string) (model.SoldierState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseSoldierState")
	if h.returnError {
		return model.SoldierState{}, errors.New(h.errorMsg)
	}
	return h.soldierState, nil
}

func (h *mockParserService) ParseVehicleState(args []string) (model.VehicleState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseVehicleState")
	if h.returnError {
		return model.VehicleState{}, errors.New(h.errorMsg)
	}
	return h.vehicleState, nil
}

func (h *mockParserService) ParseProjectileEvent(args []string) (parser.ParsedProjectileEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseProjectileEvent")
	if h.returnError {
		return parser.ParsedProjectileEvent{}, errors.New(h.errorMsg)
	}
	return h.projectile, nil
}

func (h *mockParserService) ParseGeneralEvent(args []string) (model.GeneralEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseGeneralEvent")
	if h.returnError {
		return model.GeneralEvent{}, errors.New(h.errorMsg)
	}
	return h.generalEvent, nil
}

func (h *mockParserService) ParseKillEvent(args []string) (parser.ParsedKillEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseKillEvent")
	if h.returnError {
		return parser.ParsedKillEvent{}, errors.New(h.errorMsg)
	}
	return h.killEvent, nil
}

func (h *mockParserService) ParseChatEvent(args []string) (model.ChatEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseChatEvent")
	if h.returnError {
		return model.ChatEvent{}, errors.New(h.errorMsg)
	}
	return h.chatEvent, nil
}

func (h *mockParserService) ParseRadioEvent(args []string) (model.RadioEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseRadioEvent")
	if h.returnError {
		return model.RadioEvent{}, errors.New(h.errorMsg)
	}
	return h.radioEvent, nil
}

func (h *mockParserService) ParseFpsEvent(args []string) (model.ServerFpsEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseFpsEvent")
	if h.returnError {
		return model.ServerFpsEvent{}, errors.New(h.errorMsg)
	}
	return h.fpsEvent, nil
}

func (h *mockParserService) ParseTimeState(args []string) (model.TimeState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseTimeState")
	if h.returnError {
		return model.TimeState{}, errors.New(h.errorMsg)
	}
	return h.timeState, nil
}

func (h *mockParserService) ParseAce3DeathEvent(args []string) (model.Ace3DeathEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseAce3DeathEvent")
	if h.returnError {
		return model.Ace3DeathEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Death, nil
}

func (h *mockParserService) ParseAce3UnconsciousEvent(args []string) (model.Ace3UnconsciousEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseAce3UnconsciousEvent")
	if h.returnError {
		return model.Ace3UnconsciousEvent{}, errors.New(h.errorMsg)
	}
	return h.ace3Uncon, nil
}

func (h *mockParserService) ParseMarkerCreate(args []string) (model.Marker, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerCreate")
	if h.returnError {
		return model.Marker{}, errors.New(h.errorMsg)
	}
	return h.marker, nil
}

func (h *mockParserService) ParseMarkerMove(args []string) (parser.ParsedMarkerMove, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerMove")
	if h.returnError {
		return parser.ParsedMarkerMove{}, errors.New(h.errorMsg)
	}
	return h.markerMove, nil
}

func (h *mockParserService) ParseMarkerDelete(args []string) (string, uint, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, "ParseMarkerDelete")
	if h.returnError {
		return "", 0, errors.New(h.errorMsg)
	}
	return h.deletedMarker, h.deleteFrame, nil
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

func TestNewManager(t *testing.T) {
	backend := &mockBackend{}
	manager := NewManager(Dependencies{}, backend)

	require.NotNil(t, manager, "expected non-nil manager")
}

func TestHandleNewSoldier_CachesEntityWithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		soldier: model.Soldier{ObjectID: 42, UnitName: "Test Soldier"},
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
		vehicle: model.Vehicle{ObjectID: 99, OcapType: "car"},
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
	entityCache.AddSoldier(model.Soldier{ObjectID: 10, GroupID: "Alpha 1", Side: "WEST"})

	parserService := &mockParserService{
		soldierState: model.SoldierState{
			SoldierObjectID: 10,
			GroupID:         "", // empty - should be filled from cache
			Side:            "", // empty - should be filled from cache
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
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// No soldier cached

	parserService := &mockParserService{
		soldierState: model.SoldierState{SoldierObjectID: 999},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:SOLDIER:STATE:", Args: []string{}})
	require.NoError(t, err) // dispatch itself doesn't error for buffered handlers

	// Give buffer time to process
	time.Sleep(200 * time.Millisecond)

	// Backend should have no states since the soldier wasn't cached
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.soldierStates), "state should not be recorded when soldier not cached")
}

func TestHandleVehicleState_ReturnsErrorWhenNotCached(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	parserService := &mockParserService{
		vehicleState: model.VehicleState{VehicleObjectID: 888},
	}

	backend := &mockBackend{}
	manager := NewManager(Dependencies{
		ParserService: parserService,
		EntityCache:   entityCache,
	}, backend)
	manager.RegisterHandlers(d)

	_, err := d.Dispatch(dispatcher.Event{Command: ":NEW:VEHICLE:STATE:", Args: []string{}})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.vehicleStates), "state should not be recorded when vehicle not cached")
}

func TestHandleKillEvent_ClassifiesVictimAndKiller(t *testing.T) {
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()

	// Soldier victim, vehicle killer
	entityCache.AddSoldier(model.Soldier{ObjectID: 5})
	entityCache.AddVehicle(model.Vehicle{ObjectID: 20})

	parserService := &mockParserService{
		killEvent: parser.ParsedKillEvent{
			Event:    model.KillEvent{CaptureFrame: 100},
			VictimID: 5,
			KillerID: 20,
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

	entityCache.AddSoldier(model.Soldier{ObjectID: 7})
	entityCache.AddVehicle(model.Vehicle{ObjectID: 30})

	componentsJSON, _ := json.Marshal([]string{"head"})
	vehicleComponentsJSON, _ := json.Marshal([]string{"hull"})

	// Create a LineStringZM for positions
	coords := []float64{
		6456.5, 5345.7, 10.0, 620.0,
		6448.0, 5337.0, 15.0, 625.0,
	}
	seq := geom.NewSequence(coords, geom.DimXYZM)
	ls := geom.NewLineString(seq)

	parserService := &mockParserService{
		projectile: parser.ParsedProjectileEvent{
			Event: model.ProjectileEvent{
				FirerObjectID: 1,
				CaptureFrame:  620,
				Positions:     ls.AsGeometry(),
				HitSoldiers:   []model.ProjectileHitsSoldier{},
				HitVehicles:   []model.ProjectileHitsVehicle{},
			},
			HitParts: []parser.RawHitPart{
				{EntityID: 7, ComponentsHit: componentsJSON, CaptureFrame: 625},
				{EntityID: 30, ComponentsHit: vehicleComponentsJSON, CaptureFrame: 626},
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

	// Create a LineStringZM with 3 positions (thrown, mid-flight, impact)
	coords := []float64{
		6456.5, 5345.7, 10.443, 620.0,
		6448.0, 5337.0, 15.0, 625.0,
		6441.55, 5328.46, 9.88, 630.0,
	}
	seq := geom.NewSequence(coords, geom.DimXYZM)
	ls := geom.NewLineString(seq)

	parserService := &mockParserService{
		projectile: parser.ParsedProjectileEvent{
			Event: model.ProjectileEvent{
				FirerObjectID:   1,
				CaptureFrame:    620,
				Weapon:          "throw",
				WeaponDisplay:   "Throw",
				MagazineDisplay: "RGO Grenade",
				MagazineIcon:    `\A3\Weapons_F\Data\UI\gear_M67_CA.paa`,
				Mode:            "HandGrenadeMuzzle",
				Positions:       ls.AsGeometry(),
				HitSoldiers:     []model.ProjectileHitsSoldier{},
				HitVehicles:     []model.ProjectileHitsVehicle{},
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

	// Trajectory should have 3 points from the LineStringZM
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
		markerMove: parser.ParsedMarkerMove{
			State:      model.MarkerState{CaptureFrame: 100},
			MarkerName: "marker_alpha",
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
	d, _ := newTestDispatcher(t)
	entityCache := cache.NewEntityCache()
	// No soldier cached — sender validation should fail

	parserService := &mockParserService{
		chatEvent: model.ChatEvent{
			SoldierObjectID: sql.NullInt32{Int32: 5, Valid: true},
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

	time.Sleep(200 * time.Millisecond)

	// Chat event should not be recorded since sender doesn't exist
	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, 0, len(backend.chatEvents), "chat event should not be recorded when sender not cached")
}

func TestHandleMarkerDelete_WithBackend(t *testing.T) {
	d, _ := newTestDispatcher(t)
	markerCache := cache.NewMarkerCache()

	parserService := &mockParserService{
		marker:        model.Marker{MarkerName: "Projectile#123"},
		deletedMarker: "Projectile#123",
		deleteFrame:   500,
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
