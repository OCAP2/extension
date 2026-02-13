package a3interface

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatDispatchResponse(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		result   any
		err      error
		expected string
	}{
		{
			name:     "success with string array (VERSION)",
			command:  ":VERSION:",
			result:   []string{"0.0.1", "2026-02-01"},
			err:      nil,
			expected: `["ok", ["0.0.1","2026-02-01"]]`,
		},
		{
			name:     "success with simple string",
			command:  ":INIT:",
			result:   "ok",
			err:      nil,
			expected: `["ok", "ok"]`,
		},
		{
			name:     "success with path string",
			command:  ":GETDIR:ARMA:",
			result:   `C:\Program Files\Arma 3`,
			err:      nil,
			expected: `["ok", "C:\Program Files\Arma 3"]`,
		},
		{
			name:     "success with nil result",
			command:  ":SOME:CMD:",
			result:   nil,
			err:      nil,
			expected: `["ok"]`,
		},
		{
			name:     "error response",
			command:  ":LOG:",
			result:   nil,
			err:      errors.New("no handler registered"),
			expected: `["error", "no handler registered"]`,
		},
		{
			name:     "success with int array",
			command:  ":DATA:",
			result:   []int{1, 2, 3},
			err:      nil,
			expected: `["ok", [1,2,3]]`,
		},
		{
			name:     "success with nested array",
			command:  ":NESTED:",
			result:   [][]string{{"a", "b"}, {"c", "d"}},
			err:      nil,
			expected: `["ok", [["a","b"],["c","d"]]]`,
		},
		{
			name:     "success with map",
			command:  ":MAP:",
			result:   map[string]int{"count": 42},
			err:      nil,
			expected: `["ok", {"count":42}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDispatchResponse(tt.command, tt.result, tt.err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestResponseFormatConsistency(t *testing.T) {
	t.Run("success responses start with ok", func(t *testing.T) {
		responses := []struct {
			result any
		}{
			{result: "simple string"},
			{result: []string{"a", "b"}},
			{result: nil},
			{result: 42},
		}

		for _, r := range responses {
			got := formatDispatchResponse(":TEST:", r.result, nil)
			assert.True(t, strings.HasPrefix(got, `["ok"`))
		}
	})

	t.Run("error responses start with error", func(t *testing.T) {
		got := formatDispatchResponse(":TEST:", nil, errors.New("test error"))
		expected := `["error", "test error"]`
		assert.Equal(t, expected, got)
	})
}
