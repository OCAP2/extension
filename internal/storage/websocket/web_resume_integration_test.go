package websocket

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/require"
)

// cuttableProxy forwards TCP to a target and can drop every live connection,
// as a network failure between the game server and the web server would.
type cuttableProxy struct {
	ln     net.Listener
	target string

	mu    sync.Mutex
	conns []net.Conn
}

func newCuttableProxy(t *testing.T, target string) *cuttableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &cuttableProxy{ln: ln, target: target}
	t.Cleanup(func() { _ = ln.Close(); p.cut() })
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			upstream, err := net.Dial("tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, client, upstream)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
			go func() { _, _ = io.Copy(client, upstream); _ = client.Close() }()
		}
	}()
	return p
}

func (p *cuttableProxy) cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// Run against a disposable OCAP web server with streaming enabled. The
// extension reaches it through a proxy that the test cuts mid-mission; the
// web server must end up with one complete recording.
func TestWebResumeIntegration(t *testing.T) {
	base := os.Getenv("OCAP_TEST_WEB_URL")
	if base == "" {
		t.Skip("set OCAP_TEST_WEB_URL to a disposable local web server")
	}
	secret := os.Getenv("OCAP_TEST_WEB_SECRET")
	u, err := url.Parse(base)
	require.NoError(t, err)
	proxy := newCuttableProxy(t, u.Host)
	wsBase := "ws://" + proxy.ln.Addr().String() + u.Path

	cfg := fastConfig(wsBase + "/api/v1/stream")
	cfg.Secret = secret
	cfg.SaveTimeout = time.Minute
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	name := fmt.Sprintf("Resume Integration %d", time.Now().UnixNano())
	require.NoError(t, b.StartMission(&core.Mission{MissionName: name, CaptureDelay: 1, Tag: "resume-test"}, &core.World{WorldName: "altis"}))
	require.True(t, b.conn.isResumable(), "the web server must support resume")
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, JoinFrame: 1, UnitName: "Resume Player", IsPlayer: true, Side: "WEST", ClassName: "B_Soldier_F"}))
	require.NoError(t, b.AddVehicle(&core.Vehicle{ID: 10, JoinFrame: 1, DisplayName: "Hunter", OcapType: "car"}))
	id, err := b.AddMarker(&core.Marker{MarkerName: "HQ", CaptureFrame: 1, MarkerType: "mil_dot", Shape: "ICON", Alpha: 1})
	require.NoError(t, err)

	state := func(frame int) {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(frame), Lifestate: 1, IsPlayer: true, UnitName: "Resume Player", Side: "WEST", Position: core.Position3D{X: float64(frame), Y: 200}}))
		require.NoError(t, b.RecordVehicleState(&core.VehicleState{VehicleID: 10, CaptureFrame: core.Frame(frame), IsAlive: true, Crew: "[]"}))
	}
	// Frames arrive at a steady pace while the link drops three times; each
	// outage begins only after the previous one was recovered.
	for f := 1; f <= 650; f++ {
		state(f)
		if f%25 == 0 {
			time.Sleep(2 * time.Millisecond)
		}
		if f == 450 {
			// A marker created before the outages still receives its updates.
			require.NoError(t, b.RecordMarkerState(&core.MarkerState{MarkerID: id, CaptureFrame: 450, Position: core.Position3D{X: 300, Y: 200}, Text: "moved", Color: "ColorBlue", Shape: "ICON"}))
		}
		if f == 200 || f == 350 || f == 500 {
			want := b.conn.resumes.Load() + 1
			proxy.cut()
			// Record during the outage before waiting for recovery.
			for g := f + 1; g <= f+20; g++ {
				state(g)
			}
			f += 20
			require.Eventually(t, func() bool { return b.conn.resumes.Load() >= want }, 10*time.Second, 5*time.Millisecond)
		}
	}
	require.NoError(t, b.EndMission(""))
	require.Equal(t, uint64(3), b.conn.resumes.Load())

	resp, err := http.Get(base + "/api/v1/operations")
	require.NoError(t, err)
	defer resp.Body.Close()
	var ops []struct {
		Name     string `json:"mission_name"`
		Filename string `json:"filename"`
		Tag      string `json:"tag"`
		Format   string `json:"storageFormat"`
		Status   string `json:"conversionStatus"`
		Chunks   int    `json:"chunkCount"`
		Duration float64 `json:"mission_duration"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ops))
	var found int
	for _, op := range ops {
		if !strings.HasPrefix(op.Name, name) {
			continue
		}
		found++
		require.Equal(t, "resume-test", op.Tag, "a resumed recording is not partial")
		require.Equal(t, "protobuf", op.Format)
		require.Equal(t, "completed", op.Status)
		require.Equal(t, 3, op.Chunks, "650 frames in 300-frame chunks")
		require.InDelta(t, 650, op.Duration, 1)
		t.Logf("stored recording %s", op.Filename)
	}
	require.Equal(t, 1, found, "the outages must not split the recording")
}

// TestWebRestartIntegration restarts the web server mid-mission with
// OCAP_TEST_WEB_RESTART_CMD (run through the shell). The server loses the
// session; the extension resends the mission in full from its spool and the
// web server must end up with one complete recording.
func TestWebRestartIntegration(t *testing.T) {
	base := os.Getenv("OCAP_TEST_WEB_URL")
	restart := os.Getenv("OCAP_TEST_WEB_RESTART_CMD")
	if base == "" || restart == "" {
		t.Skip("set OCAP_TEST_WEB_URL and OCAP_TEST_WEB_RESTART_CMD to restart a disposable local web server")
	}
	cfg := fastConfig(strings.Replace(base, "http", "ws", 1) + "/api/v1/stream")
	cfg.Secret = os.Getenv("OCAP_TEST_WEB_SECRET")
	cfg.SaveTimeout = time.Minute
	cfg.ReconnectMax = 500 * time.Millisecond
	cfg.AckStallTimeout = 5 * time.Second // the web server acknowledges every second
	cfg.SpoolDir = t.TempDir()
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	name := fmt.Sprintf("Restart Integration %d", time.Now().UnixNano())
	require.NoError(t, b.StartMission(&core.Mission{MissionName: name, CaptureDelay: 1, Tag: "restart-test"}, &core.World{WorldName: "altis"}))
	require.True(t, b.conn.isResumable())
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, JoinFrame: 1, UnitName: "Restart Player", IsPlayer: true, Side: "WEST"}))
	state := func(frame int) {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(frame), Lifestate: 1, IsPlayer: true, UnitName: "Restart Player", Side: "WEST", Position: core.Position3D{X: float64(frame), Y: 200}}))
	}
	for f := 1; f <= 300; f++ {
		state(f)
	}
	// Everything so far is acknowledged, so it lives only in the spool.
	require.Eventually(t, func() bool { return b.conn.acked.Load() == 302 }, 10*time.Second, 10*time.Millisecond)

	// No output capture: the restarted server would inherit the pipe and keep
	// the test's I/O open. A failing command still fails the test.
	require.NoError(t, exec.Command("sh", "-c", restart).Run(), "restart command")
	for f := 301; f <= 600; f++ {
		state(f)
	}
	require.NoError(t, b.EndMission(""))

	resp, err := http.Get(base + "/api/v1/operations")
	require.NoError(t, err)
	defer resp.Body.Close()
	var ops []struct {
		Name     string  `json:"mission_name"`
		Filename string  `json:"filename"`
		Tag      string  `json:"tag"`
		Status   string  `json:"conversionStatus"`
		Chunks   int     `json:"chunkCount"`
		Duration float64 `json:"mission_duration"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ops))
	var found int
	for _, op := range ops {
		if op.Name != name {
			continue
		}
		found++
		require.Equal(t, "restart-test", op.Tag, "the resent recording is complete")
		require.Equal(t, "completed", op.Status)
		require.Equal(t, 2, op.Chunks, "600 frames in 300-frame chunks")
		require.InDelta(t, 600, op.Duration, 1)
		t.Logf("stored recording %s", op.Filename)
	}
	require.Equal(t, 1, found, "the interrupted segment is replaced by the complete recording")
}

// TestCrashingProducerHelper is the producer process TestWebCrashRecoveryIntegration
// kills: it records a mission and exits without closing anything.
func TestCrashingProducerHelper(t *testing.T) {
	if os.Getenv("OCAP_TEST_CRASH_HELPER") == "" {
		t.Skip("run by TestWebCrashRecoveryIntegration")
	}
	cfg := fastConfig(os.Getenv("OCAP_TEST_CRASH_URL"))
	cfg.Secret = os.Getenv("OCAP_TEST_WEB_SECRET")
	cfg.AckStallTimeout = 5 * time.Second
	cfg.SpoolDir = os.Getenv("OCAP_TEST_CRASH_SPOOL")
	b := New(cfg)
	require.NoError(t, b.Init())
	require.NoError(t, b.StartMission(&core.Mission{MissionName: os.Getenv("OCAP_TEST_CRASH_NAME"), CaptureDelay: 1, Tag: "crash-test"}, &core.World{WorldName: "altis"}))
	require.NoError(t, b.AddSoldier(&core.Soldier{ID: 1, JoinFrame: 1, UnitName: "Crash Player", IsPlayer: true, Side: "WEST"}))
	for f := 1; f <= 450; f++ {
		require.NoError(t, b.RecordSoldierState(&core.SoldierState{SoldierID: 1, CaptureFrame: core.Frame(f), Lifestate: 1, IsPlayer: true, UnitName: "Crash Player", Side: "WEST", Position: core.Position3D{X: float64(f), Y: 200}}))
	}
	time.Sleep(2 * spoolFlushInterval) // let the spool reach the disk
	os.Exit(3)                         // the game server crashes
}

// TestWebCrashRecoveryIntegration kills a producer process mid-mission. A new
// producer recovers the mission from the spool, and the web server must end
// up with one recording of everything the crashed process recorded, marked
// partial.
func TestWebCrashRecoveryIntegration(t *testing.T) {
	base := os.Getenv("OCAP_TEST_WEB_URL")
	if base == "" {
		t.Skip("set OCAP_TEST_WEB_URL to a disposable local web server")
	}
	url := strings.Replace(base, "http", "ws", 1) + "/api/v1/stream"
	spoolDir := t.TempDir()
	name := fmt.Sprintf("Crash Integration %d", time.Now().UnixNano())

	helper := exec.Command(os.Args[0], "-test.run=^TestCrashingProducerHelper$", "-test.count=1")
	helper.Env = append(os.Environ(), "OCAP_TEST_CRASH_HELPER=1", "OCAP_TEST_CRASH_URL="+url,
		"OCAP_TEST_CRASH_SPOOL="+spoolDir, "OCAP_TEST_CRASH_NAME="+name)
	err := helper.Run()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 3, exit.ExitCode(), "the helper must crash, not fail")

	// The crashed process' live file goes stale after spoolStaleAfter.
	old := spoolStaleAfter
	spoolStaleAfter = 3 * time.Second
	t.Cleanup(func() { spoolStaleAfter = old })
	cfg := fastConfig(url)
	cfg.Secret = os.Getenv("OCAP_TEST_WEB_SECRET")
	cfg.AckStallTimeout = 5 * time.Second
	cfg.SpoolDir = spoolDir
	cfg.RecoveryInterval = 200 * time.Millisecond
	b := New(cfg)
	require.NoError(t, b.Init())
	defer b.Close()

	type op struct {
		Name     string  `json:"mission_name"`
		Filename string  `json:"filename"`
		Tag      string  `json:"tag"`
		Status   string  `json:"conversionStatus"`
		Chunks   int     `json:"chunkCount"`
		Duration float64 `json:"mission_duration"`
	}
	var found []op
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/api/v1/operations")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var ops []op
		if json.NewDecoder(resp.Body).Decode(&ops) != nil {
			return false
		}
		found = found[:0]
		for _, o := range ops {
			if o.Name == name {
				found = append(found, o)
			}
		}
		return len(found) == 1 && found[0].Status == "completed"
	}, 30*time.Second, 200*time.Millisecond, "the recovered mission must be saved")
	require.Equal(t, "crash-test,partial", found[0].Tag)
	require.Equal(t, 2, found[0].Chunks, "450 frames in 300-frame chunks")
	require.InDelta(t, 450, found[0].Duration, 1, "nothing the crashed process recorded is lost")
	require.Eventually(t, func() bool {
		entries, err := os.ReadDir(spoolDir)
		return err == nil && len(entries) == 0
	}, 5*time.Second, 50*time.Millisecond, "the recovered spool is deleted")
	t.Logf("recovered recording %s", found[0].Filename)
}
