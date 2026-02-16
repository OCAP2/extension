package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePolylineToCore_Valid(t *testing.T) {
	input := "[[100.5,200.25],[300.75,400.5],[500,600]]"
	poly, err := ParsePolylineToCore(input)

	require.NoError(t, err)
	require.Len(t, poly, 3)
	assert.Equal(t, 100.5, poly[0].X)
	assert.Equal(t, 200.25, poly[0].Y)
	assert.Equal(t, 500.0, poly[2].X)
	assert.Equal(t, 600.0, poly[2].Y)
}

func TestParsePolylineToCore_InvalidJSON(t *testing.T) {
	_, err := ParsePolylineToCore("not valid json")
	require.Error(t, err)
}

func TestParsePolylineToCore_TooFewPoints(t *testing.T) {
	_, err := ParsePolylineToCore("[[100,200]]")
	require.Error(t, err)
}

func TestParsePolylineToCore_InsufficientCoordinates(t *testing.T) {
	_, err := ParsePolylineToCore("[[100],[200,300]]")
	require.Error(t, err)
}
