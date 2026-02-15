package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"no quotes", "hello", "hello"},
		{"double quoted", `"hello"`, "hello"},
		{"single quotes only", "'hello'", "'hello'"},
		{"quotes in middle", `he"llo`, `he"llo`},
		{"only quotes", `""`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSQFStringArray(t *testing.T) {
	tests := []struct {
		name                            string
		input                           string
		wantVehicle, wantWeapon, wantMag string
	}{
		{"vehicle turret kill", `["Hunter HMG","Mk30 HMG .50",".50 BMG 200Rnd"]`, "Hunter HMG", "Mk30 HMG .50", ".50 BMG 200Rnd"},
		{"on-foot kill", `["","MX 6.5 mm","6.5 mm 30Rnd Sand Mag"]`, "", "MX 6.5 mm", "6.5 mm 30Rnd Sand Mag"},
		{"explosive kill", `["","M6 SLAM Mine",""]`, "", "M6 SLAM Mine", ""},
		{"vehicle no weapon", `["Hunter HMG","",""]`, "Hunter HMG", "", ""},
		{"all empty", `["","",""]`, "", "", ""},
		{"legacy plain string", `MX 6.5 mm`, "", "MX 6.5 mm", ""},
		{"empty string", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, w, m := ParseSQFStringArray(tt.input)
			assert.Equal(t, tt.wantVehicle, v)
			assert.Equal(t, tt.wantWeapon, w)
			assert.Equal(t, tt.wantMag, m)
		})
	}
}

func TestFormatWeaponText(t *testing.T) {
	tests := []struct {
		name                    string
		vehicle, weapon, mag    string
		expected                string
	}{
		{"vehicle turret", "Hunter HMG", "Mk30 HMG .50", ".50 BMG 200Rnd", "Hunter HMG: Mk30 HMG .50 [.50 BMG 200Rnd]"},
		{"on-foot", "", "MX 6.5 mm", "6.5 mm 30Rnd Sand Mag", "MX 6.5 mm [6.5 mm 30Rnd Sand Mag]"},
		{"explosive", "", "M6 SLAM Mine", "", "M6 SLAM Mine"},
		{"vehicle only", "Hunter HMG", "", "", "Hunter HMG"},
		{"all empty", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatWeaponText(tt.vehicle, tt.weapon, tt.mag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFixEscapeQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"no escaped quotes", "hello", "hello"},
		{"single escaped quote", `he""llo`, `he"llo`},
		{"multiple escaped quotes", `a""b""c`, `a"b"c`},
		{"consecutive escaped", `a""""b`, `a""b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixEscapeQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

