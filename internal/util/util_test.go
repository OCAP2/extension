package util

import "testing"

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
			if result != tt.expected {
				t.Errorf("TrimQuotes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
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
			if result != tt.expected {
				t.Errorf("FixEscapeQuotes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

