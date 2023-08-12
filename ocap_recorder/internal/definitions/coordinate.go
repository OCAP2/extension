package ocapdefs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkbhex"
	"github.com/wroge/wgs84"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GEO POINTS
// https://pkg.go.dev/github.com/StampWallet/backend/internal/database

// We will always store as 3857, including for world locations, because SQLite has no spatial awareness and we need to be able to interpret point data from strings during migrations using inherent Scan function.

// GPSCoordinates is a 2d or 3d point in space
type GPSCoordinates geom.Point

// ErrInvalidCoordinates is returned when the coordinates are invalid
var ErrInvalidCoordinates = errors.New("invalid coordinates")

// Scan implements the sql.Scanner interface
func (g *GPSCoordinates) Scan(input interface{}) error {
	gt, err := ewkbhex.Decode(input.(string))
	if err != nil {
		// this will definitely error if using SQLite due to invalid byte read
		// so if InvalidByteError, try to parse it as a string
		if _, ok := err.(*hex.InvalidByteError); ok {
			if strings.HasPrefix(input.(string), "POINT") {
				// this is a point
				start := strings.TrimPrefix(input.(string), "POINT(")
				end := strings.TrimSuffix(start, ")")
				coords := strings.Split(end, " ")
				if len(coords) != 2 {
					return ErrInvalidCoordinates
				}
				long, err := strconv.ParseFloat(coords[0], 64)
				if err != nil {
					return ErrInvalidCoordinates
				}
				lat, err := strconv.ParseFloat(coords[1], 64)
				if err != nil {
					return ErrInvalidCoordinates
				}
				*g = GPSCoordinates(*geom.NewPointFlat(geom.XY, []float64{long, lat}).SetSRID(3857))
				return nil
			}
		} else {
			return err
		}
	}
	gp := gt.(*geom.Point)
	gc := GPSCoordinates(*gp)
	*g = gc
	return nil
}

// ToString returns a string representation of the GPS point
func (g *GPSCoordinates) ToString() string {
	return strconv.FormatFloat(g.Coords().X(), 'f', -1, 64) +
		"," +
		strconv.FormatFloat(g.Coords().Y(), 'f', -1, 64)
}

// GormDataType gorm common data type
func (g GPSCoordinates) GormDataType() string {
	return "GEOMETRY"
}

// GormValue gorm value
func (g GPSCoordinates) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	b := geom.Point(g)
	srid := 3857
	var vars []interface{} = []interface{}{fmt.Sprintf(`SRID=%d;POINT(0 0)"`, srid)}
	// if !b.Empty() {
	if db.Dialector.Name() == "postgres" {
		// postgis
		vars = []interface{}{
			b.X(),
			b.Y(),
			srid,
		}
		return clause.Expr{
			SQL:  "ST_SetSRID(ST_Point(?, ?), ?)",
			Vars: vars,
		}
	}

	// sqlite
	return clause.Expr{
		SQL:  "?",
		Vars: []interface{}{fmt.Sprintf(`POINT(%f %f)`, b.X(), b.Y())},
	}

}

// GPSFromString parses a string in the format "long,lat" or "long,lat,elev" into a GPS point, and returns the type and elevation
func Coord3857FromString(
	coords string,
) (
	GPSCoordinates,
	float64,
	error,
) {
	sp := strings.Split(coords, ",")
	if len(sp) != 2 && len(sp) != 3 {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	x, err := strconv.ParseFloat(sp[0], 64)
	if err != nil {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	y, err := strconv.ParseFloat(sp[1], 64)
	if err != nil {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	z := 0.0
	if len(sp) == 3 {
		z, err = strconv.ParseFloat(sp[2], 64)
		if err != nil {
			return GPSCoordinates{}, 0, ErrInvalidCoordinates
		}
	}

	geomPoint := GPSCoordinates(*geom.NewPointFlat(geom.XYZ, []float64{x, y, z}).SetSRID(3857))

	return geomPoint, z, nil
}

// GPSFromCoords creates a GPS point from a longitude and latitude
func Coords3857From4326(
	longitude float64,
	latitude float64,
) (
	GPSCoordinates,
	error,
) {
	var x, y float64
	// if provided SRID was 4326, convert to 3857
	epsg := wgs84.EPSG()
	f := epsg.Transform(4326, 3857)
	x, y, _ = f(longitude, latitude, 0)
	point, err := geom.NewPoint(geom.XY).SetCoords([]float64{x, y})
	if err != nil {
		return GPSCoordinates{}, err
	}
	return GPSCoordinates(*point.SetSRID(3857)), nil
}
