package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/require"
)

// Run against a disposable OCAP web server with streaming enabled. Ordinary
// extension tests use the in-process protocol server and need no web checkout.
func TestWebIngestionIntegration(t *testing.T) {
	base := os.Getenv("OCAP_TEST_WEB_URL")
	if base == "" {
		t.Skip("set OCAP_TEST_WEB_URL to a disposable local web server")
	}
	secret := os.Getenv("OCAP_TEST_WEB_SECRET")
	b := New(Config{URL: strings.Replace(base, "http", "ws", 1) + "/api/v1/stream", Secret: secret})
	require.NoError(t, b.Init())
	defer b.Close()
	name := fmt.Sprintf("Proxy Integration %d", time.Now().UnixNano())
	require.NoError(t, b.StartMission(&core.Mission{MissionName: name, CaptureDelay: 1, Tag: "stream-test"}, &core.World{WorldName: "altis"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, JoinFrame: 1, UnitName: "Proxy Player", IsPlayer: true, Side: "WEST", ClassName: "B_Soldier_F"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10, JoinFrame: 1, DisplayName: "Hunter", OcapType: "car"}))
	for _, frame := range []core.Frame{1, 301, 605} {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: frame, Lifestate: 1, IsPlayer: true, UnitName: "Proxy Player", Side: "WEST", Position: core.Position3D{X: float64(frame), Y: 200}}))
	}
	// A late vehicle state must amend chunk zero after the soldier crossed it.
	require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 10, CaptureFrame: 1, IsAlive: true, Fuel: 0.8, Crew: "[1]"}))
	id, err := b.AddMarker(&core.Marker{MarkerName: "HQ", CaptureFrame: 1, MarkerType: "mil_dot", Shape: "ICON", Alpha: 1})
	require.NoError(t, err)
	require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: id, CaptureFrame: 301, Position: core.Position3D{X: 300, Y: 200}, Text: "moved", Color: "ColorBlue", Shape: "ICON"}))
	require.NoError(t, b.DeleteMarker(&core.DeleteMarker{Name: "HQ", EndFrame: 606}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{CaptureFrame: 1, Name: "inventorySnapshot", Message: `{"unitId":1,"reason":"periodic"}`}))
	require.NoError(t, b.RecordGeneralEvent(&core.GeneralEvent{CaptureFrame: 301, Name: "staminaSnapshot", Message: `{"unitId":1,"stamina":0.5}`}))
	require.NoError(t, b.RecordTelemetryEvent(&core.TelemetryEvent{CaptureFrame: 605, FpsAverage: 48, FpsMin: 40}))
	unit := uint(1)
	require.NoError(t, b.RecordRadioEvent(&core.RadioEvent{CaptureFrame: 301, SoldierID: &unit, Radio: "radio", Frequency: 50, StartEnd: "start"}))
	require.NoError(t, b.RecordSectorEvent(&core.SectorEvent{CaptureFrame: 301, Name: "captured", Side: "WEST", UnitName: "Sector A", PosX: 100, PosY: 200}))
	require.NoError(t, b.RecordAce3UnconsciousEvent(&core.Ace3UnconsciousEvent{CaptureFrame: 302, SoldierID: 1, IsUnconscious: true}))
	require.NoError(t, b.RecordAce3DeathEvent(&core.Ace3DeathEvent{CaptureFrame: 606, SoldierID: 1, Reason: "test"}))
	require.NoError(t, b.AddPlacedObject(&core.PlacedObject{ID: 20, JoinFrame: 1, OwnerID: 1, MarkerIcon: "trench", Direction: 90}))
	require.NoError(t, b.RecordPlacedObjectEvent(&core.PlacedObjectEvent{PlacedID: 20, CaptureFrame: 606, EventType: "deleted"}))
	require.NoError(t, b.SetFocusStart(2))
	require.NoError(t, b.SetFocusEnd(605))
	require.NoError(t, b.RecordEndMissionEvent(&core.EndMissionEvent{CaptureFrame: 606, Side: "WEST", Message: "Complete"}))
	require.NoError(t, b.DeleteSoldier(1, 606))
	require.NoError(t, b.DeleteVehicle(10, 606))
	require.NoError(t, b.EndMission())
	// A persistent extension instance must support the next mission immediately,
	// even while the previous server-side WebSocket is being closed.
	require.NoError(t, b.StartMission(&core.Mission{MissionName: name + " second", CaptureDelay: 1}, &core.World{WorldName: "altis"}))
	require.NoError(t, b.RecordTelemetryEvent(&core.TelemetryEvent{CaptureFrame: 1, FpsAverage: 50}))
	require.NoError(t, b.EndMission())
	resp, err := http.Get(base + "/api/v1/operations")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var ops []struct {
		Name     string `json:"mission_name"`
		Filename string `json:"filename"`
		Format   string `json:"storageFormat"`
		Version  uint32 `json:"schemaVersion"`
		Status   string `json:"conversionStatus"`
		Chunks   int    `json:"chunkCount"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ops))
	for _, op := range ops {
		if op.Name == name {
			require.Equal(t, "protobuf", op.Format)
			require.Equal(t, uint32(2), op.Version)
			require.Equal(t, "completed", op.Status)
			require.Equal(t, 3, op.Chunks)
			for _, path := range []string{"/manifest.pb", "/chunks/0000.pb", "/chunks/0001.pb", "/chunks/0002.pb", "/snapshots/1.pb"} {
				r, err := http.Get(base + "/data/" + op.Filename + path)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, r.StatusCode, path)
				r.Body.Close()
			}
			t.Logf("stored recording %s", op.Filename)
			return
		}
	}
	t.Fatal("web server did not publish the acknowledged mission")
}
