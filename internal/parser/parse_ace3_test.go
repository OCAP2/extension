package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
)

func TestParseAce3DeathEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.Ace3DeathEvent)
		wantErr bool
	}{
		{
			name:  "death with source",
			input: []string{"100", "5", "bleeding", "10"},
			check: func(t *testing.T, e core.Ace3DeathEvent) {
				assert.Equal(t, uint(100), e.CaptureFrame)
				assert.Equal(t, uint(5), e.SoldierID)
				assert.Equal(t, "bleeding", e.Reason)
				assert.NotNil(t, e.LastDamageSourceID)
				assert.Equal(t, uint(10), *e.LastDamageSourceID)
			},
		},
		{
			name:  "death no source (-1)",
			input: []string{"100", "5", "explosion", "-1"},
			check: func(t *testing.T, e core.Ace3DeathEvent) {
				assert.Equal(t, uint(5), e.SoldierID)
				assert.Equal(t, "explosion", e.Reason)
				assert.Nil(t, e.LastDamageSourceID)
			},
		},
		{
			name:  "float frame and objectIDs",
			input: []string{"100.00", "5.00", "drowning", "10.00"},
			check: func(t *testing.T, e core.Ace3DeathEvent) {
				assert.Equal(t, uint(100), e.CaptureFrame)
				assert.Equal(t, uint(5), e.SoldierID)
				assert.Equal(t, uint(10), *e.LastDamageSourceID)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "5", "bleeding", "10"},
			wantErr: true,
		},
		{
			name:    "error: bad victimID",
			input:   []string{"100", "abc", "bleeding", "10"},
			wantErr: true,
		},
		{
			name:    "error: bad lastDamageSourceID",
			input:   []string{"100", "5", "bleeding", "abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseAce3DeathEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseAce3UnconsciousEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.Ace3UnconsciousEvent)
		wantErr bool
	}{
		{
			name:  "goes unconscious",
			input: []string{"100", "5", "true"},
			check: func(t *testing.T, e core.Ace3UnconsciousEvent) {
				assert.Equal(t, uint(100), e.CaptureFrame)
				assert.Equal(t, uint(5), e.SoldierID)
				assert.True(t, e.IsUnconscious)
			},
		},
		{
			name:  "regains consciousness",
			input: []string{"100", "5", "false"},
			check: func(t *testing.T, e core.Ace3UnconsciousEvent) {
				assert.Equal(t, uint(5), e.SoldierID)
				assert.False(t, e.IsUnconscious)
			},
		},
		{
			name:  "float IDs",
			input: []string{"200.00", "42.00", "true"},
			check: func(t *testing.T, e core.Ace3UnconsciousEvent) {
				assert.Equal(t, uint(200), e.CaptureFrame)
				assert.Equal(t, uint(42), e.SoldierID)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "5", "true"},
			wantErr: true,
		},
		{
			name:    "error: bad objectID",
			input:   []string{"100", "abc", "true"},
			wantErr: true,
		},
		{
			name:    "error: bad bool",
			input:   []string{"100", "5", "maybe"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseAce3UnconsciousEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}
