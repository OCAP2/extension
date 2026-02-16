package geo

import (
	"errors"
	"strconv"
	"strings"

	"github.com/OCAP2/extension/v5/pkg/core"
	geom "github.com/peterstace/simplefeatures/geom"
	"github.com/wroge/wgs84"
)

// GEO POINTS
// We will always store as 3857, including for world locations, because SQLite has no spatial awareness and we need to be able to interpret point data from strings during migrations using inherent Scan function.
// Geometry data is stored in the WKB format, which is a binary representation of the geometry data.

// ErrInvalidCoordinates is returned when the coordinates are invalid
var ErrInvalidCoordinates = errors.New("invalid coordinates provided")

// Coord3857FromString parses a string in the format "long,lat" or "long,lat,elev" into a GPS point, and returns the type and elevation
func Coord3857FromString(
	coords string,
) (
	point geom.Point,
	elev float64,
	err error,
) {
	// split the string into its components
	coordsSplit := strings.Split(coords, ",")
	if len(coordsSplit) < 2 {
		return geom.NewEmptyPoint(geom.DimXYZ), 0, ErrInvalidCoordinates
	}
	// parse the longitude
	long, err := strconv.ParseFloat(coordsSplit[0], 64)
	if err != nil {
		return geom.NewEmptyPoint(geom.DimXYZ), 0, ErrInvalidCoordinates
	}
	// parse the latitude
	lat, err := strconv.ParseFloat(coordsSplit[1], 64)
	if err != nil {
		return geom.NewEmptyPoint(geom.DimXYZ), 0, ErrInvalidCoordinates
	}
	// parse the elevation
	if len(coordsSplit) > 2 {
		elev, err = strconv.ParseFloat(coordsSplit[2], 64)
		if err != nil {
			return geom.NewEmptyPoint(geom.DimXYZ), 0, ErrInvalidCoordinates
		}
	}
	// create the point
	point = geom.NewPoint(
		geom.Coordinates{
			XY:   geom.XY{X: long, Y: lat},
			Z:    elev,
			Type: geom.CoordinatesType(geom.DimXYZ),
		},
	)
	return point, elev, nil
}

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

// Coords3857From4326 creates a GPS point from a longitude and latitude
func Coords3857From4326(
	longitude float64,
	latitude float64,
) (
	point geom.Point,
	err error,
) {
	var x, y float64
	// if provided SRID was 4326, convert to 3857
	epsg := wgs84.EPSG()
	f := epsg.Transform(4326, 3857)
	x, y, _ = f(longitude, latitude, 0)
	point = geom.NewPoint(
		geom.Coordinates{
			XY: geom.XY{X: x, Y: y},
			Z:  0,
		},
	)
	return point, nil
}
