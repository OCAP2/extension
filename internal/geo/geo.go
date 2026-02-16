package geo

import (
	"errors"
	"strconv"
	"strings"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// GEO POINTS
// We will always store as 3857, including for world locations, because SQLite has no spatial awareness and we need to be able to interpret point data from strings during migrations using inherent Scan function.
// Geometry data is stored in the WKB format, which is a binary representation of the geometry data.

// ErrInvalidCoordinates is returned when the coordinates are invalid
var ErrInvalidCoordinates = errors.New("invalid coordinates provided")

// Position3DFromString parses a "long,lat" or "long,lat,elev" string into a core.Position3D.
func Position3DFromString(coords string) (core.Position3D, error) {
	coordsSplit := strings.Split(coords, ",")
	if len(coordsSplit) < 2 {
		return core.Position3D{}, ErrInvalidCoordinates
	}
	long, err := strconv.ParseFloat(coordsSplit[0], 64)
	if err != nil {
		return core.Position3D{}, ErrInvalidCoordinates
	}
	lat, err := strconv.ParseFloat(coordsSplit[1], 64)
	if err != nil {
		return core.Position3D{}, ErrInvalidCoordinates
	}
	var elev float64
	if len(coordsSplit) > 2 {
		elev, err = strconv.ParseFloat(coordsSplit[2], 64)
		if err != nil {
			return core.Position3D{}, ErrInvalidCoordinates
		}
	}
	return core.Position3D{X: long, Y: lat, Z: elev}, nil
}
