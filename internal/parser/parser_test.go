package parser

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestParser() *Parser {
	p := NewParser(slog.Default(), "1.0.0", "2.0.0")
	return p
}

func TestNewParser(t *testing.T) {
	p := newTestParser()
	require.NotNil(t, p)
}

func TestParseUintFromFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"integer", "32", 32, false},
		{"zero", "0", 0, false},
		{"float with decimals", "32.00", 32, false},
		{"float with trailing zero", "30.0", 30, false},
		{"large integer", "65535", 65535, false},
		{"large float", "65535.00", 65535, false},
		{"fractional rejects", "10.99", 0, true},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"negative", "-1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUintFromFloat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseIntFromFloat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"integer", "32", 32, false},
		{"zero", "0", 0, false},
		{"negative integer", "-1", -1, false},
		{"float with decimals", "32.00", 32, false},
		{"negative float", "-1.00", -1, false},
		{"large integer", "65535", 65535, false},
		{"fractional rejects", "10.99", 0, true},
		{"empty string", "", 0, true},
		{"non-numeric", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIntFromFloat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

