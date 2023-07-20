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

var (
	// this is provided when using GPSFromString to determine if the corresponding output to DB should be a string value (SQLite) or an actual geometry function (PostGIS)
	SAVE_LOCAL = false
)

// GEO POINTS
// https://pkg.go.dev/github.com/StampWallet/backend/internal/database
type GPSCoordinates geom.Point

var ErrInvalidCoordinates = errors.New("invalid coordinates")

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

func (g *GPSCoordinates) ToString() string {
	return strconv.FormatFloat(g.Coords().X(), 'f', -1, 64) +
		"," +
		strconv.FormatFloat(g.Coords().Y(), 'f', -1, 64)
}

func (g GPSCoordinates) GormDataType() string {
	return "geometry"
}

func (g GPSCoordinates) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	b := geom.Point(g)
	srid := b.SRID()
	var vars []interface{} = []interface{}{fmt.Sprintf(`SRID=%d;POINT(0 0)"`, srid)}
	// if !b.Empty() {
	if !SAVE_LOCAL {
		// postgis
		vars = []interface{}{fmt.Sprintf("SRID=%d;POINT(%f %f)", srid, b.X(), b.Y())}
		return clause.Expr{
			SQL:  "ST_GeomFromText(?)",
			Vars: vars,
		}
	} else {
		// sqlite
		return clause.Expr{
			SQL:  "?",
			Vars: []interface{}{fmt.Sprintf(`POINT(%f %f)`, b.X(), b.Y())},
		}
	}
	// } else {
	// 	return clause.Expr{
	// 		SQL: "NULL",
	// 	}
	// }
}

func GPSFromString(coords string, srid int, sqlite bool) (GPSCoordinates, float64, error) {
	SAVE_LOCAL = sqlite
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
	return GPSCoordinates(*geom.NewPointFlat(geom.XYZ, []float64{long, lat, elev}).SetSRID(srid)), elev, nil
}

func GPSFromCoords(longitude float64, latitude float64, srid int) GPSCoordinates {
	if srid == 0 {
		srid = 4326
	}
	return GPSCoordinates(*geom.NewPointFlat(geom.XY, geom.Coord{longitude, latitude}).SetSRID(srid))
}
