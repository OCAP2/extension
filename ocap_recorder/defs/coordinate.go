package defs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkbhex"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GEO POINTS
// https://pkg.go.dev/github.com/StampWallet/backend/internal/database

// GPSCoordinates is a 2d or 3d point in space
type GPSCoordinates geom.Point

// ErrInvalidCoordinates is returned when the coordinates are invalid
var ErrInvalidCoordinates = errors.New("invalid coordinates")

// Scan implements the sql.Scanner interface
func (g *GPSCoordinates) Scan(input interface{}) error {
	gt, err := ewkbhex.Decode(input.(string))
	if err != nil {
		return err
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
	srid := b.SRID()
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
func GPSFromString(
	coords string,
	srid int,
) (
	GPSCoordinates,
	float64,
	error,
) {
	sp := strings.Split(coords, ",")
	if len(sp) != 2 && len(sp) != 3 {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	long, err := strconv.ParseFloat(sp[0], 64)
	if err != nil {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	lat, err := strconv.ParseFloat(sp[1], 64)
	if err != nil {
		return GPSCoordinates{}, 0, ErrInvalidCoordinates
	}
	elev := 0.0
	if len(sp) == 3 {
		elev, err = strconv.ParseFloat(sp[2], 64)
		if err != nil {
			return GPSCoordinates{}, 0, ErrInvalidCoordinates
		}
	}
	if srid == 0 {
		srid = 3857
	}

	geomPoint := GPSCoordinates(*geom.NewPointFlat(geom.XYZ, []float64{long, lat, elev}).SetSRID(srid))

	return geomPoint, elev, nil
}

// GPSFromCoords creates a GPS point from a longitude and latitude
func GPSFromCoords(
	longitude float64,
	latitude float64,
	srid int,
) (
	GPSCoordinates,
	error,
) {
	if srid == 0 {
		srid = 3857
	}
	point, err := geom.NewPoint(geom.XYZ).SetCoords([]float64{longitude, latitude, 0})
	if err != nil {
		return GPSCoordinates{}, err
	}
	return GPSCoordinates(*point.SetSRID(srid)), nil
}
