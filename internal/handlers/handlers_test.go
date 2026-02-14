package handlers

import (
	"testing"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	require.NotNil(t, svc)
	assert.Nil(t, svc.deps.DB)
}

func TestSetBackend(t *testing.T) {
	svc := newTestService()

	backend := &mockBackend{}
	svc.SetBackend(backend)

	assert.NotNil(t, svc.backend)
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

	require.NoError(t, err)

	// Verify mission context was set
	mission := svc.ctx.GetMission()
	assert.Equal(t, "Test Mission", mission.MissionName)

	world := svc.ctx.GetWorld()
	assert.Equal(t, "Altis", world.WorldName)

	// Verify backend was called
	assert.True(t, backend.missionStarted, "expected backend.StartMission to be called")
	assert.NotNil(t, backend.startedMission, "expected mission to be passed to backend")
	assert.NotNil(t, backend.startedWorld, "expected world to be passed to backend")
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

	require.NoError(t, err)

	// Verify mission context was still set
	mission := svc.ctx.GetMission()
	assert.Equal(t, "No Backend Mission", mission.MissionName)
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

	require.NoError(t, err)

	// Verify addons were processed (even without DB)
	mission := svc.ctx.GetMission()
	assert.Len(t, mission.Addons, 3)
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
			require.NoError(t, err)

			assert.Equal(t, tt.expectedCrew, state.Crew)
		})
	}
}

func TestLogSoldierState_ScoresParsingPanic(t *testing.T) {
	svc := newTestService()
	svc.SetBackend(&mockBackend{})

	// Set up mission context
	mission := &model.Mission{}
	mission.ID = 1
	svc.ctx.SetMission(mission, &model.World{})

	// Add a soldier to the entity cache so LogSoldierState can find it
	svc.deps.EntityCache.AddSoldier(model.Soldier{ObjectID: 42, MissionID: 1})

	// Base data shared by all subtests: 15 fields matching the expected format
	// [ocapId, pos, bearing, lifestate, inVehicle, name, isPlayer, currentRole, frame,
	//  hasStableVitals, isDraggedCarried, scores, vehicleRole, inVehicleID, stance]
	makeData := func(isPlayer string, scores string) []string {
		return []string{
			"42",                 // 0: ocapId
			"[100.0,200.0,50.0]", // 1: position
			"90",                 // 2: bearing
			"1",                  // 3: lifestate
			"false",              // 4: inVehicle
			"TestUnit",           // 5: name
			isPlayer,             // 6: isPlayer
			"rifleman",           // 7: currentRole
			"10",                 // 8: frame
			"true",               // 9: hasStableVitals
			"false",              // 10: isDraggedCarried
			scores,               // 11: scores
			"",                   // 12: vehicleRole
			"-1",                 // 13: inVehicleID
			"STAND",              // 14: stance
		}
	}

	tests := []struct {
		name           string
		isPlayer       string
		scores         string
		expectScores   model.SoldierScores
	}{
		{
			name:     "valid 6 scores",
			isPlayer: "true",
			scores:   "1,2,3,4,5,100",
			expectScores: model.SoldierScores{
				InfantryKills: 1, VehicleKills: 2, ArmorKills: 3,
				AirKills: 4, Deaths: 5, TotalScore: 100,
			},
		},
		{
			name:         "single score value should not panic",
			isPlayer:     "true",
			scores:       "0",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "empty scores string should not panic",
			isPlayer:     "true",
			scores:       "",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "partial scores should not panic",
			isPlayer:     "true",
			scores:       "1,2,3",
			expectScores: model.SoldierScores{},
		},
		{
			name:         "non-player ignores scores",
			isPlayer:     "false",
			scores:       "garbage",
			expectScores: model.SoldierScores{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeData(tt.isPlayer, tt.scores)

			// Must not panic
			state, err := svc.LogSoldierState(data)
			require.NoError(t, err)

			assert.Equal(t, tt.expectScores, state.Scores)
		})
	}
}

func TestLogSoldierState_GroupAndSide(t *testing.T) {
	svc := newTestService()
	svc.SetBackend(&mockBackend{})

	mission := &model.Mission{}
	mission.ID = 1
	svc.ctx.SetMission(mission, &model.World{})

	svc.deps.EntityCache.AddSoldier(model.Soldier{
		ObjectID: 42, MissionID: 1,
		GroupID: "InitGroup", Side: "WEST",
	})

	baseData := []string{
		"42",                  // 0: ocapId
		"[100.0,200.0,50.0]", // 1: position
		"90",                  // 2: bearing
		"1",                   // 3: lifestate
		"false",               // 4: inVehicle
		"TestUnit",            // 5: name
		"false",               // 6: isPlayer
		"rifleman",            // 7: currentRole
		"10",                  // 8: frame
		"true",                // 9: hasStableVitals
		"false",               // 10: isDraggedCarried
		"",                    // 11: scores
		"",                    // 12: vehicleRole
		"-1",                  // 13: inVehicleID
		"STAND",               // 14: stance
	}

	t.Run("17 fields parses group and side", func(t *testing.T) {
		data := append(append([]string{}, baseData...), "Alpha 1-1", "EAST")
		state, err := svc.LogSoldierState(data)
		require.NoError(t, err)
		assert.Equal(t, "Alpha 1-1", state.GroupID)
		assert.Equal(t, "EAST", state.Side)
	})

	t.Run("15 fields falls back to initial soldier data", func(t *testing.T) {
		state, err := svc.LogSoldierState(append([]string{}, baseData...))
		require.NoError(t, err)
		assert.Equal(t, "InitGroup", state.GroupID)
		assert.Equal(t, "WEST", state.Side)
	})
}

func TestMissionContext_ThreadSafe(t *testing.T) {
	ctx := NewMissionContext()

	// Should have default values
	mission := ctx.GetMission()
	assert.Equal(t, "No mission loaded", mission.MissionName)

	world := ctx.GetWorld()
	assert.Equal(t, "No world loaded", world.WorldName)
}
