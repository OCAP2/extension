package websocket

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
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
