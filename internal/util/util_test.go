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

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{"empty slice", []string{}, "a", false},
		{"found first", []string{"a", "b", "c"}, "a", true},
		{"found middle", []string{"a", "b", "c"}, "b", true},
		{"found last", []string{"a", "b", "c"}, "c", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty string in slice", []string{"a", "", "c"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.str)
			if result != tt.expected {
				t.Errorf("Contains(%v, %q) = %v, want %v", tt.slice, tt.str, result, tt.expected)
			}
		})
	}
}
