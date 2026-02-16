package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoord3857FromString_ValidWithElevation(t *testing.T) {
	point, elev, err := Coord3857FromString("100.5,200.25,50.0")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 100.5, coords.X)
	assert.Equal(t, 200.25, coords.Y)
	assert.Equal(t, 50.0, elev)
}

func TestCoord3857FromString_ValidWithoutElevation(t *testing.T) {
	point, elev, err := Coord3857FromString("100.5,200.25")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 100.5, coords.X)
	assert.Equal(t, 200.25, coords.Y)
	assert.Equal(t, 0.0, elev)
}

func TestCoord3857FromString_NegativeCoordinates(t *testing.T) {
	point, elev, err := Coord3857FromString("-100.5,-200.25,-50.0")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, -100.5, coords.X)
	assert.Equal(t, -200.25, coords.Y)
	assert.Equal(t, -50.0, elev)
}

func TestCoord3857FromString_IntegerCoordinates(t *testing.T) {
	point, _, err := Coord3857FromString("100,200")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 100.0, coords.X)
	assert.Equal(t, 200.0, coords.Y)
}

func TestCoord3857FromString_ZeroCoordinates(t *testing.T) {
	point, elev, err := Coord3857FromString("0,0,0")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 0.0, coords.X)
	assert.Equal(t, 0.0, coords.Y)
	assert.Equal(t, 0.0, elev)
}

func TestCoord3857FromString_InvalidTooFewComponents(t *testing.T) {
	_, _, err := Coord3857FromString("100.5")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoord3857FromString_InvalidEmptyString(t *testing.T) {
	_, _, err := Coord3857FromString("")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoord3857FromString_InvalidLongitude(t *testing.T) {
	_, _, err := Coord3857FromString("abc,200.25")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoord3857FromString_InvalidLatitude(t *testing.T) {
	_, _, err := Coord3857FromString("100.5,xyz")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoord3857FromString_InvalidElevation(t *testing.T) {
	_, _, err := Coord3857FromString("100.5,200.25,invalid")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoord3857FromString_ExtraComponents(t *testing.T) {
	// Extra components beyond 3 should be ignored
	point, elev, err := Coord3857FromString("100.5,200.25,50.0,extra,ignored")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 100.5, coords.X)
	assert.Equal(t, 200.25, coords.Y)
	assert.Equal(t, 50.0, elev)
}

func TestCoord3857FromString_ScientificNotation(t *testing.T) {
	point, _, err := Coord3857FromString("1e2,2e3")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 100.0, coords.X)
	assert.Equal(t, 2000.0, coords.Y)
}

func TestCoord3857FromString_LargeCoordinates(t *testing.T) {
	point, _, err := Coord3857FromString("1000000.123456,2000000.654321")

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Equal(t, 1000000.123456, coords.X)
	assert.Equal(t, 2000000.654321, coords.Y)
}

func TestPosition3DFromString_ValidWithElevation(t *testing.T) {
	pos, err := Position3DFromString("100.5,200.25,50.0")
	require.NoError(t, err)
	assert.Equal(t, 100.5, pos.X)
	assert.Equal(t, 200.25, pos.Y)
	assert.Equal(t, 50.0, pos.Z)
}

func TestPosition3DFromString_ValidWithoutElevation(t *testing.T) {
	pos, err := Position3DFromString("100.5,200.25")
	require.NoError(t, err)
	assert.Equal(t, 100.5, pos.X)
	assert.Equal(t, 200.25, pos.Y)
	assert.Equal(t, 0.0, pos.Z)
}

func TestPosition3DFromString_InvalidTooFewComponents(t *testing.T) {
	_, err := Position3DFromString("100.5")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestPosition3DFromString_InvalidLongitude(t *testing.T) {
	_, err := Position3DFromString("abc,200.25")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestPosition3DFromString_InvalidLatitude(t *testing.T) {
	_, err := Position3DFromString("100.5,xyz")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestPosition3DFromString_InvalidElevation(t *testing.T) {
	_, err := Position3DFromString("100.5,200.25,invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCoordinates)
}

func TestCoords3857From4326_ValidCoordinates(t *testing.T) {
	// Test converting WGS84 (EPSG:4326) to Web Mercator (EPSG:3857)
	// Approximate coordinates for a point
	point, err := Coords3857From4326(0, 0)

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	// At (0, 0) in 4326, the 3857 coordinates should also be (0, 0)
	assert.Equal(t, 0.0, coords.X)
	assert.Equal(t, 0.0, coords.Y)
}

func TestCoords3857From4326_NonZeroCoordinates(t *testing.T) {
	// Test a point at 10 degrees longitude, 10 degrees latitude
	point, err := Coords3857From4326(10, 10)

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	// In Web Mercator, these should be non-zero positive values
	assert.Greater(t, coords.X, 0.0)
	assert.Greater(t, coords.Y, 0.0)
}

func TestCoords3857From4326_NegativeCoordinates(t *testing.T) {
	// Test a point in the Southern/Western hemisphere
	point, err := Coords3857From4326(-45, -30)

	require.NoError(t, err)

	coords, ok := point.Coordinates()
	require.True(t, ok, "expected valid coordinates")
	assert.Less(t, coords.X, 0.0)
	assert.Less(t, coords.Y, 0.0)
}
