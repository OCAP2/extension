package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OCAP2/extension/v5/pkg/core"
)

func TestParseChatEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.ChatEvent)
		wantErr bool
	}{
		{
			name:  "player chat global channel",
			input: []string{"50", "5", "0", "Player1", "Player1", "Hello world", "76561198000074241"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, uint(50), e.CaptureFrame)
				assert.NotNil(t, e.SoldierID)
				assert.Equal(t, uint(5), *e.SoldierID)
				assert.Equal(t, "Global", e.Channel)
				assert.Equal(t, "Player1", e.FromName)
				assert.Equal(t, "Player1", e.SenderName)
				assert.Equal(t, "Hello world", e.Message)
				assert.Equal(t, "76561198000074241", e.PlayerUID)
			},
		},
		{
			name:  "side channel",
			input: []string{"50", "5", "1", "Player1", "Player1", "Side msg", "76561198000074241"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Side", e.Channel)
			},
		},
		{
			name:  "command channel",
			input: []string{"50", "5", "2", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Command", e.Channel)
			},
		},
		{
			name:  "group channel",
			input: []string{"50", "5", "3", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Group", e.Channel)
			},
		},
		{
			name:  "vehicle channel",
			input: []string{"50", "5", "4", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Vehicle", e.Channel)
			},
		},
		{
			name:  "direct channel",
			input: []string{"50", "5", "5", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Direct", e.Channel)
			},
		},
		{
			name:  "system message sender=-1",
			input: []string{"50", "-1", "16", "Server", "Server", "System msg", ""},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Nil(t, e.SoldierID)
				assert.Equal(t, "System", e.Channel)
			},
		},
		{
			name:  "custom channel (6-15)",
			input: []string{"50", "5", "10", "Player1", "Player1", "Custom msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Custom", e.Channel)
			},
		},
		{
			name:  "custom channel lower bound (6)",
			input: []string{"50", "5", "6", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Custom", e.Channel)
			},
		},
		{
			name:  "custom channel upper bound (15)",
			input: []string{"50", "5", "15", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "Custom", e.Channel)
			},
		},
		{
			name:  "unknown channel >=16 maps to System",
			input: []string{"50", "5", "99", "Player1", "Player1", "msg", "uid"},
			check: func(t *testing.T, e core.ChatEvent) {
				assert.Equal(t, "System", e.Channel)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "5", "0", "Player1", "Player1", "msg", "uid"},
			wantErr: true,
		},
		{
			name:    "error: bad senderID",
			input:   []string{"50", "abc", "0", "Player1", "Player1", "msg", "uid"},
			wantErr: true,
		},
		{
			name:    "error: bad channel",
			input:   []string{"50", "5", "abc", "Player1", "Player1", "msg", "uid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseChatEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}

func TestParseRadioEvent(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		check   func(t *testing.T, e core.RadioEvent)
		wantErr bool
	}{
		{
			name:  "TFAR start transmission",
			input: []string{"50", "5", "TFAR_anprc152", "LR", "START", "1", "false", "87.5", "_lr_code"},
			check: func(t *testing.T, e core.RadioEvent) {
				assert.Equal(t, uint(50), e.CaptureFrame)
				assert.NotNil(t, e.SoldierID)
				assert.Equal(t, uint(5), *e.SoldierID)
				assert.Equal(t, "TFAR_anprc152", e.Radio)
				assert.Equal(t, "LR", e.RadioType)
				assert.Equal(t, "START", e.StartEnd)
				assert.Equal(t, int8(1), e.Channel)
				assert.False(t, e.IsAdditional)
				assert.InDelta(t, float32(87.5), e.Frequency, 0.01)
				assert.Equal(t, "_lr_code", e.Code)
			},
		},
		{
			name:  "end transmission sender=-1",
			input: []string{"60", "-1", "TFAR_anprc148jem", "SR", "END", "0", "true", "100.0", "code2"},
			check: func(t *testing.T, e core.RadioEvent) {
				assert.Nil(t, e.SoldierID)
				assert.Equal(t, "END", e.StartEnd)
				assert.Equal(t, "SR", e.RadioType)
				assert.True(t, e.IsAdditional)
			},
		},
		{
			name:    "error: bad frame",
			input:   []string{"abc", "5", "radio", "LR", "START", "1", "false", "87.5", "code"},
			wantErr: true,
		},
		{
			name:    "error: bad senderID",
			input:   []string{"50", "abc", "radio", "LR", "START", "1", "false", "87.5", "code"},
			wantErr: true,
		},
		{
			name:    "error: bad channel",
			input:   []string{"50", "5", "radio", "LR", "START", "abc", "false", "87.5", "code"},
			wantErr: true,
		},
		{
			name:    "error: bad isAdditional",
			input:   []string{"50", "5", "radio", "LR", "START", "1", "maybe", "87.5", "code"},
			wantErr: true,
		},
		{
			name:    "error: bad frequency",
			input:   []string{"50", "5", "radio", "LR", "START", "1", "false", "abc", "code"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseRadioEvent(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.check(t, result)
		})
	}
}
