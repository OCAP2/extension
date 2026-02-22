package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlacedObject(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, obj core.PlacedObject)
		wantErr bool
	}{
		{
			name: "mine placement",
			input: []string{
				"100",              // 0: frame
				"50",               // 1: placedId
				"APERSMine_Range",  // 2: className
				"APERS Mine",      // 3: displayName
				"5000.5,3000.2,10", // 4: position
				"12",               // 5: firerOcapId
				"WEST",             // 6: side
				"put",              // 7: weapon
				`\A3\icon.paa`,    // 8: magazineIcon
			},
			check: func(t *testing.T, obj core.PlacedObject) {
				assert.Equal(t, core.Frame(100), obj.JoinFrame)
				assert.Equal(t, uint16(50), obj.ID)
				assert.Equal(t, "APERSMine_Range", obj.ClassName)
				assert.Equal(t, "APERS Mine", obj.DisplayName)
				assert.InDelta(t, 5000.5, obj.Position.X, 0.01)
				assert.InDelta(t, 3000.2, obj.Position.Y, 0.01)
				assert.InDelta(t, 10.0, obj.Position.Z, 0.01)
				assert.Equal(t, uint16(12), obj.OwnerID)
				assert.Equal(t, "WEST", obj.Side)
				assert.Equal(t, "put", obj.Weapon)
				assert.Equal(t, `\A3\icon.paa`, obj.MagazineIcon)
			},
		},
		{
			name: "float IDs",
			input: []string{
				"200.00", "25.00", "SatchelCharge", "Satchel Charge",
				"1000,2000,5", "8.00", "EAST", "put", "icon.paa",
			},
			check: func(t *testing.T, obj core.PlacedObject) {
				assert.Equal(t, core.Frame(200), obj.JoinFrame)
				assert.Equal(t, uint16(25), obj.ID)
				assert.Equal(t, uint16(8), obj.OwnerID)
			},
		},
		{
			name:    "error: insufficient fields",
			input:   []string{"100", "50", "Mine"},
			wantErr: true,
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "50", "Mine", "Mine", "1,2,3", "12", "WEST", "put", "icon"},
			wantErr: true,
		},
		{
			name:    "error: bad placedId",
			input:   []string{"100", "abc", "Mine", "Mine", "1,2,3", "12", "WEST", "put", "icon"},
			wantErr: true,
		},
		{
			name:    "error: bad position",
			input:   []string{"100", "50", "Mine", "Mine", "bad", "12", "WEST", "put", "icon"},
			wantErr: true,
		},
		{
			name:    "error: bad ownerID",
			input:   []string{"100", "50", "Mine", "Mine", "1,2,3", "abc", "WEST", "put", "icon"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParsePlacedObject(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParsePlacedObjectEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, evt core.PlacedObjectEvent)
		wantErr bool
	}{
		{
			name: "detonation",
			input: []string{
				"500",              // 0: frame
				"50",               // 1: placedId
				"detonated",        // 2: eventType
				"5000.5,3000.2,10", // 3: position
			},
			check: func(t *testing.T, evt core.PlacedObjectEvent) {
				assert.Equal(t, core.Frame(500), evt.CaptureFrame)
				assert.Equal(t, uint16(50), evt.PlacedID)
				assert.Equal(t, "detonated", evt.EventType)
				assert.InDelta(t, 5000.5, evt.Position.X, 0.01)
				assert.InDelta(t, 3000.2, evt.Position.Y, 0.01)
				assert.InDelta(t, 10.0, evt.Position.Z, 0.01)
			},
		},
		{
			name: "deleted",
			input: []string{
				"600.00", "25.00", "deleted", "1000,2000,5",
			},
			check: func(t *testing.T, evt core.PlacedObjectEvent) {
				assert.Equal(t, core.Frame(600), evt.CaptureFrame)
				assert.Equal(t, uint16(25), evt.PlacedID)
				assert.Equal(t, "deleted", evt.EventType)
			},
		},
		{
			name:    "error: insufficient fields",
			input:   []string{"500", "50"},
			wantErr: true,
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "50", "detonated", "1,2,3"},
			wantErr: true,
		},
		{
			name:    "error: bad placedId",
			input:   []string{"500", "abc", "detonated", "1,2,3"},
			wantErr: true,
		},
		{
			name:    "error: bad position",
			input:   []string{"500", "50", "detonated", "bad"},
			wantErr: true,
		},
		{
			name: "hit event with target",
			input: []string{
				"450",              // 0: frame
				"50",               // 1: placedId
				"hit",              // 2: eventType
				"5100.5,3100.2,10", // 3: position (victim pos)
				"12",               // 4: hitEntityOcapId
			},
			check: func(t *testing.T, evt core.PlacedObjectEvent) {
				assert.Equal(t, core.Frame(450), evt.CaptureFrame)
				assert.Equal(t, uint16(50), evt.PlacedID)
				assert.Equal(t, "hit", evt.EventType)
				assert.InDelta(t, 5100.5, evt.Position.X, 0.01)
				assert.InDelta(t, 3100.2, evt.Position.Y, 0.01)
				assert.InDelta(t, 10.0, evt.Position.Z, 0.01)
				require.NotNil(t, evt.HitEntityID)
				assert.Equal(t, uint16(12), *evt.HitEntityID)
			},
		},
		{
			name: "detonation has no hit entity",
			input: []string{
				"500", "50", "detonated", "5000.5,3000.2,10",
			},
			check: func(t *testing.T, evt core.PlacedObjectEvent) {
				assert.Equal(t, "detonated", evt.EventType)
				assert.Nil(t, evt.HitEntityID)
			},
		},
		{
			name:    "error: bad hitEntityOcapId",
			input:   []string{"500", "50", "hit", "1,2,3", "abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParsePlacedObjectEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}
