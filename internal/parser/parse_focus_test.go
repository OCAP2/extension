package parser

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFocusStart(t *testing.T) {
	p := newTestParser()

	tests := []struct {
		name    string
		input   []string
		want    core.Frame
		wantErr bool
	}{
		{
			name:  "integer frame",
			input: []string{"150"},
			want:  core.Frame(150),
		},
		{
			name:  "float frame",
			input: []string{"150.00"},
			want:  core.Frame(150),
		},
		{
			name:  "quoted frame",
			input: []string{`"300"`},
			want:  core.Frame(300),
		},
		{
			name:    "no args",
			input:   []string{},
			wantErr: true,
		},
		{
			name:    "invalid value",
			input:   []string{"abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseFocusStart(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseFocusEnd(t *testing.T) {
	p := newTestParser()

	frame, err := p.ParseFocusEnd([]string{"850"})
	require.NoError(t, err)
	assert.Equal(t, core.Frame(850), frame)
}
