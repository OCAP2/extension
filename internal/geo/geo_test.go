package geo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
