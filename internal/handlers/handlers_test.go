package handlers

import (
	"testing"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/storage"
)

// mockBackend implements storage.Backend for testing
type mockBackend struct {
	missionStarted bool
	missionEnded   bool
	startedMission *core.Mission
	startedWorld   *core.World
}

func (b *mockBackend) Init() error                                         { return nil }
func (b *mockBackend) Close() error                                        { return nil }
func (b *mockBackend) AddSoldier(s *core.Soldier) error                    { return nil }
func (b *mockBackend) AddVehicle(v *core.Vehicle) error                    { return nil }
func (b *mockBackend) AddMarker(m *core.Marker) error                      { return nil }
func (b *mockBackend) RecordSoldierState(s *core.SoldierState) error       { return nil }
func (b *mockBackend) RecordVehicleState(v *core.VehicleState) error       { return nil }
func (b *mockBackend) RecordMarkerState(s *core.MarkerState) error         { return nil }
func (b *mockBackend) RecordFiredEvent(e *core.FiredEvent) error           { return nil }
func (b *mockBackend) RecordGeneralEvent(e *core.GeneralEvent) error       { return nil }
func (b *mockBackend) RecordHitEvent(e *core.HitEvent) error               { return nil }
func (b *mockBackend) RecordKillEvent(e *core.KillEvent) error             { return nil }
func (b *mockBackend) RecordChatEvent(e *core.ChatEvent) error             { return nil }
func (b *mockBackend) RecordRadioEvent(e *core.RadioEvent) error           { return nil }
func (b *mockBackend) RecordServerFpsEvent(e *core.ServerFpsEvent) error   { return nil }
func (b *mockBackend) RecordTimeState(t *core.TimeState) error             { return nil }
func (b *mockBackend) RecordAce3DeathEvent(e *core.Ace3DeathEvent) error   { return nil }
func (b *mockBackend) RecordAce3UnconsciousEvent(e *core.Ace3UnconsciousEvent) error {
	return nil
}
func (b *mockBackend) DeleteMarker(name string, endFrame uint)                    {}
func (b *mockBackend) GetSoldierByObjectID(ocapID uint16) (*core.Soldier, bool) { return nil, false }
func (b *mockBackend) GetVehicleByObjectID(ocapID uint16) (*core.Vehicle, bool) { return nil, false }
func (b *mockBackend) GetMarkerByName(name string) (*core.Marker, bool)       { return nil, false }

func (b *mockBackend) StartMission(mission *core.Mission, world *core.World) error {
	b.missionStarted = true
	b.startedMission = mission
	b.startedWorld = world
	return nil
}

func (b *mockBackend) EndMission() error {
	b.missionEnded = true
	return nil
}

var _ storage.Backend = (*mockBackend)(nil)

func newTestService() *Service {
	logManager := logging.NewSlogManager()
	logManager.Setup(nil, "info", nil)

	deps := Dependencies{
		DB:               nil, // memory-only mode
		EntityCache:      cache.NewEntityCache(),
		MarkerCache:      cache.NewMarkerCache(),
		LogManager:       logManager,
		ExtensionName:    "test",
		AddonVersion:     "1.0.0",
		ExtensionVersion: "2.0.0",
	}

	ctx := NewMissionContext()
	return NewService(deps, ctx)
}

func TestNewService_NilDB(t *testing.T) {
	svc := newTestService()

	if svc == nil {
		t.Fatal("expected service to be created")
	}

	if svc.deps.DB != nil {
		t.Error("expected DB to be nil")
	}
}

func TestSetBackend(t *testing.T) {
	svc := newTestService()

	backend := &mockBackend{}
	svc.SetBackend(backend)

	if svc.backend == nil {
		t.Error("expected backend to be set")
	}
}

func TestLogNewMission_MemoryOnlyMode(t *testing.T) {
	svc := newTestService()
	backend := &mockBackend{}
	svc.SetBackend(backend)

	worldData := `{"worldName":"Altis","displayName":"Altis","worldSize":30720,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"Test Mission",
		"missionNameSource":"file",
		"briefingName":"Test Briefing",
		"serverName":"Test Server",
		"serverProfile":"TestProfile",
		"onLoadName":"Loading Test",
		"author":"Tester",
		"tag":"TvT",
		"captureDelay":1.0,
		"addons":[["addon1","12345"],["addon2",67890]],
		"playableSlots":[10,10,5,0,2],
		"sideFriendly":[false,false,true]
	}`

	err := svc.LogNewMission([]string{worldData, missionData})

	if err != nil {
		t.Fatalf("LogNewMission failed in memory-only mode: %v", err)
	}

	// Verify mission context was set
	mission := svc.ctx.GetMission()
	if mission.MissionName != "Test Mission" {
		t.Errorf("expected mission name 'Test Mission', got '%s'", mission.MissionName)
	}

	world := svc.ctx.GetWorld()
	if world.WorldName != "Altis" {
		t.Errorf("expected world name 'Altis', got '%s'", world.WorldName)
	}

	// Verify backend was called
	if !backend.missionStarted {
		t.Error("expected backend.StartMission to be called")
	}

	if backend.startedMission == nil {
		t.Error("expected mission to be passed to backend")
	}

	if backend.startedWorld == nil {
		t.Error("expected world to be passed to backend")
	}
}

func TestLogNewMission_MemoryOnlyMode_NoBackend(t *testing.T) {
	svc := newTestService()
	// No backend set

	worldData := `{"worldName":"Stratis","displayName":"Stratis","worldSize":8192,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"No Backend Mission",
		"missionNameSource":"file",
		"briefingName":"Test",
		"serverName":"Server",
		"serverProfile":"Profile",
		"onLoadName":"Loading",
		"author":"Author",
		"tag":"Coop",
		"captureDelay":0.5,
		"addons":[],
		"playableSlots":[5,5,0,0,0],
		"sideFriendly":[false,false,false]
	}`

	err := svc.LogNewMission([]string{worldData, missionData})

	if err != nil {
		t.Fatalf("LogNewMission failed without backend: %v", err)
	}

	// Verify mission context was still set
	mission := svc.ctx.GetMission()
	if mission.MissionName != "No Backend Mission" {
		t.Errorf("expected mission name 'No Backend Mission', got '%s'", mission.MissionName)
	}
}

func TestLogNewMission_AddonsWithoutDB(t *testing.T) {
	svc := newTestService()

	worldData := `{"worldName":"Tanoa","displayName":"Tanoa","worldSize":15360,"latitude":-40.0,"longitude":30.0}`
	missionData := `{
		"missionName":"Addon Test",
		"missionNameSource":"file",
		"briefingName":"Test",
		"serverName":"Server",
		"serverProfile":"Profile",
		"onLoadName":"Loading",
		"author":"Author",
		"tag":"TvT",
		"captureDelay":1.0,
		"addons":[["CBA_A3","450814997"],["ACE3",463939057],["TFAR","894678801"]],
		"playableSlots":[10,10,0,0,0],
		"sideFriendly":[false,false,false]
	}`

	err := svc.LogNewMission([]string{worldData, missionData})

	if err != nil {
		t.Fatalf("LogNewMission with addons failed: %v", err)
	}

	// Verify addons were processed (even without DB)
	mission := svc.ctx.GetMission()
	if len(mission.Addons) != 3 {
		t.Errorf("expected 3 addons, got %d", len(mission.Addons))
	}
}

func TestLogVehicleState_CrewPreservesBrackets(t *testing.T) {
	svc := newTestService()
	svc.SetBackend(&mockBackend{})

	// Set up mission context
	mission := &model.Mission{}
	mission.ID = 1
	svc.ctx.SetMission(mission, &model.World{})

	// Add a vehicle to the entity cache so LogVehicleState can find it
	svc.deps.EntityCache.AddVehicle(model.Vehicle{ObjectID: 10, MissionID: 1})

	tests := []struct {
		name         string
		crewInput    string
		expectedCrew string
	}{
		{"multi-crew array", "[20,21]", "[20,21]"},
		{"single-crew array", "[108]", "[108]"},
		{"empty crew array", "[]", "[]"},
		{"large crew", "[1,2,3,4,5]", "[1,2,3,4,5]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate game data: [ocapId, pos, bearing, alive, crew, frame, fuel, damage, engine, locked, side, vecDir, vecUp, turretAz, turretEl]
			data := []string{
				"10",                 // 0: ocapId
				"[100.0,200.0,50.0]", // 1: position
				"90",                 // 2: bearing
				"true",               // 3: alive
				tt.crewInput,         // 4: crew
				"5",                  // 5: frame
				"0.85",              // 6: fuel
				"0.0",               // 7: damage
				"true",              // 8: engineOn
				"false",             // 9: locked
				"EAST",              // 10: side
				"[0,1,0]",           // 11: vectorDir
				"[0,0,1]",           // 12: vectorUp
				"45.0",              // 13: turretAzimuth
				"10.0",              // 14: turretElevation
			}

			state, err := svc.LogVehicleState(data)
			if err != nil {
				t.Fatalf("LogVehicleState failed: %v", err)
			}

			if state.Crew != tt.expectedCrew {
				t.Errorf("expected Crew=%q, got %q", tt.expectedCrew, state.Crew)
			}
		})
	}
}

func TestMissionContext_ThreadSafe(t *testing.T) {
	ctx := NewMissionContext()

	// Should have default values
	mission := ctx.GetMission()
	if mission.MissionName != "No mission loaded" {
		t.Errorf("expected default mission name, got '%s'", mission.MissionName)
	}

	world := ctx.GetWorld()
	if world.WorldName != "No world loaded" {
		t.Errorf("expected default world name, got '%s'", world.WorldName)
	}
}
