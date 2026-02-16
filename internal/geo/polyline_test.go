package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePolyline_Valid(t *testing.T) {
	input := "[[100.5,200.25],[300.75,400.5],[500,600]]"

	ls, err := ParsePolyline(input)

	require.NoError(t, err)

	seq := ls.Coordinates()
	require.Equal(t, 3, seq.Length())

	expected := [][2]float64{
		{100.5, 200.25},
		{300.75, 400.5},
		{500, 600},
	}
	for i := 0; i < seq.Length(); i++ {
		pt := seq.GetXY(i)
		assert.Equal(t, expected[i][0], pt.X)
		assert.Equal(t, expected[i][1], pt.Y)
	}
}

func TestParsePolyline_TwoPoints(t *testing.T) {
	input := "[[0,0],[100,100]]"

	ls, err := ParsePolyline(input)

	require.NoError(t, err)
	require.Equal(t, 2, ls.Coordinates().Length())
}

func TestParsePolyline_InvalidJSON(t *testing.T) {
	input := "not valid json"

	_, err := ParsePolyline(input)

	require.Error(t, err)
}

func TestParsePolyline_TooFewPoints(t *testing.T) {
	input := "[[100,200]]"

	_, err := ParsePolyline(input)

	require.Error(t, err)
}

func TestParsePolyline_EmptyArray(t *testing.T) {
	input := "[]"

	_, err := ParsePolyline(input)

	require.Error(t, err)
}

func TestParsePolyline_InsufficientCoordinates(t *testing.T) {
	input := "[[100],[200,300]]"

	_, err := ParsePolyline(input)

	require.Error(t, err)
}

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
