package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/internal/model"
)

func TestParseMarkerCreate(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, m model.Marker)
		wantErr bool
	}{
		{
			name: "ICON marker (respawn)",
			input: []string{
				"respawn_west",  // 0: markerName
				"0",             // 1: direction
				"mil_dot",       // 2: type
				"Respawn West",  // 3: text
				"0",             // 4: frameNo
				"-1",            // 5: skip
				"-1",            // 6: ownerId
				"ColorBLUFOR",   // 7: color
				"[1,1]",         // 8: size
				"WEST",          // 9: side
				"6069.06,5627.81,17.81", // 10: position
				"ICON",          // 11: shape
				"1",             // 12: alpha
				"",              // 13: brush
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, "respawn_west", m.MarkerName)
				assert.Equal(t, float32(0), m.Direction)
				assert.Equal(t, "mil_dot", m.MarkerType)
				assert.Equal(t, "Respawn West", m.Text)
				assert.Equal(t, uint(0), m.CaptureFrame)
				assert.Equal(t, -1, m.OwnerID)
				assert.Equal(t, "ColorBLUFOR", m.Color)
				assert.Equal(t, "ICON", m.Shape)
				assert.Equal(t, float32(1), m.Alpha)
				assert.False(t, m.IsDeleted)
				assert.False(t, m.Position.IsEmpty())
			},
		},
		{
			name: "ELLIPSE marker (sector)",
			input: []string{
				"sector_alpha",  // 0: markerName
				"42.56",         // 1: direction
				"hd_objective",  // 2: type
				"Alpha",         // 3: text
				"100",           // 4: frameNo
				"-1",            // 5: skip
				"5",             // 6: ownerId
				"ColorRed",      // 7: color
				"[200,200]",     // 8: size
				"EAST",          // 9: side
				"5000.0,4000.0,0", // 10: position
				"ELLIPSE",       // 11: shape
				"0.5",           // 12: alpha
				"grid",          // 13: brush
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, "ELLIPSE", m.Shape)
				assert.InDelta(t, float32(42.56), m.Direction, 0.01)
				assert.InDelta(t, float32(0.5), m.Alpha, 0.01)
				assert.Equal(t, "grid", m.Brush)
				assert.Equal(t, 5, m.OwnerID)
				assert.Equal(t, uint(100), m.CaptureFrame)
			},
		},
		{
			name: "military symbol",
			input: []string{
				"mil_marker_1",  // 0: markerName
				"0",             // 1: direction
				"mil_triangle",  // 2: type
				"HQ",            // 3: text
				"50",            // 4: frameNo
				"-1",            // 5: skip
				"-1",            // 6: ownerId
				"ColorGreen",    // 7: color
				"[1,1]",         // 8: size
				"GUER",          // 9: side
				"3000.0,2000.0,100.0", // 10: position
				"ICON",          // 11: shape
				"1",             // 12: alpha
				"",              // 13: brush
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, "mil_triangle", m.MarkerType)
				assert.Equal(t, "ColorGreen", m.Color)
				assert.Equal(t, "GUER", m.Side)
			},
		},
		{
			name: "POLYLINE marker",
			input: []string{
				"polyline_1",    // 0: markerName
				"0",             // 1: direction
				"",              // 2: type
				"Route",         // 3: text
				"10",            // 4: frameNo
				"-1",            // 5: skip
				"-1",            // 6: ownerId
				"ColorBlack",    // 7: color
				"[1,1]",         // 8: size
				"",              // 9: side
				"[[100.0,200.0],[300.0,400.0],[500.0,600.0]]", // 10: polyline position
				"POLYLINE",      // 11: shape
				"1",             // 12: alpha
				"",              // 13: brush
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, "POLYLINE", m.Shape)
				assert.True(t, m.Position.IsEmpty())
				assert.False(t, m.Polyline.IsEmpty())
			},
		},
		{
			name: "bad ownerId falls back to -1",
			input: []string{
				"marker1", "0", "mil_dot", "text", "0", "-1", "not_a_number",
				"ColorRed", "[1,1]", "", "1000.0,2000.0,0", "ICON", "1", "",
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, -1, m.OwnerID)
			},
		},
		{
			name: "bad alpha falls back to 1.0",
			input: []string{
				"marker1", "0", "mil_dot", "text", "0", "-1", "-1",
				"ColorRed", "[1,1]", "", "1000.0,2000.0,0", "ICON", "not_a_float", "",
			},
			check: func(t *testing.T, m model.Marker) {
				assert.Equal(t, float32(1.0), m.Alpha)
			},
		},
		{
			name: "error: bad direction",
			input: []string{
				"marker1", "abc", "mil_dot", "text", "0", "-1", "-1",
				"ColorRed", "[1,1]", "", "1000.0,2000.0,0", "ICON", "1", "",
			},
			wantErr: true,
		},
		{
			name: "error: bad frame",
			input: []string{
				"marker1", "0", "mil_dot", "text", "abc", "-1", "-1",
				"ColorRed", "[1,1]", "", "1000.0,2000.0,0", "ICON", "1", "",
			},
			wantErr: true,
		},
		{
			name: "error: bad position",
			input: []string{
				"marker1", "0", "mil_dot", "text", "0", "-1", "-1",
				"ColorRed", "[1,1]", "", "not,a,valid,pos", "ICON", "1", "",
			},
			wantErr: true,
		},
		{
			name: "error: bad polyline",
			input: []string{
				"marker1", "0", "mil_dot", "text", "0", "-1", "-1",
				"ColorRed", "[1,1]", "", "not_valid_json", "POLYLINE", "1", "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseMarkerCreate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseMarkerMove(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, r ParsedMarkerMove)
		wantErr bool
	}{
		{
			name:  "normal move",
			input: []string{"markerName", "517", "6069.06,5627.81,17.81", "0", "1"},
			check: func(t *testing.T, r ParsedMarkerMove) {
				assert.Equal(t, "markerName", r.MarkerName)
				assert.Equal(t, uint(517), r.State.CaptureFrame)
				assert.False(t, r.State.Position.IsEmpty())
				assert.Equal(t, float32(0), r.State.Direction)
				assert.Equal(t, float32(1), r.State.Alpha)
			},
		},
		{
			name:  "with direction and alpha",
			input: []string{"sector_1", "200", "5000.0,4000.0,0", "-40.08", "0.5"},
			check: func(t *testing.T, r ParsedMarkerMove) {
				assert.Equal(t, "sector_1", r.MarkerName)
				assert.InDelta(t, float32(-40.08), r.State.Direction, 0.01)
				assert.InDelta(t, float32(0.5), r.State.Alpha, 0.01)
			},
		},
		{
			name:  "bad direction falls back to 0",
			input: []string{"marker1", "100", "1000.0,2000.0,0", "not_float", "1"},
			check: func(t *testing.T, r ParsedMarkerMove) {
				assert.Equal(t, float32(0), r.State.Direction)
			},
		},
		{
			name:  "bad alpha falls back to 1.0",
			input: []string{"marker1", "100", "1000.0,2000.0,0", "0", "not_float"},
			check: func(t *testing.T, r ParsedMarkerMove) {
				assert.Equal(t, float32(1.0), r.State.Alpha)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"marker1", "abc", "1000.0,2000.0,0", "0", "1"},
			wantErr: true,
		},
		{
			name:    "error: bad position",
			input:   []string{"marker1", "100", "not_valid", "0", "1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseMarkerMove(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseMarkerDelete(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name      string
		input     []string
		wantName  string
		wantFrame uint
		wantErr   bool
	}{
		{
			name:      "normal delete",
			input:     []string{"Projectile#1204.15", "137"},
			wantName:  "Projectile#1204.15",
			wantFrame: 137,
		},
		{
			name:      "float frame",
			input:     []string{"sector_alpha", "200.00"},
			wantName:  "sector_alpha",
			wantFrame: 200,
		},
		{
			name:    "error: bad frame",
			input:   []string{"marker1", "abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, frame, err := p.ParseMarkerDelete(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantFrame, frame)
		})
	}
}
