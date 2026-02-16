package geo

import (
	"encoding/json"
	"fmt"

	"github.com/OCAP2/extension/v5/internal/model/core"
	geom "github.com/peterstace/simplefeatures/geom"
)

// ParsePolyline parses a JSON array of coordinates into a geom.LineString.
// Input format: "[[x1,y1],[x2,y2],...]"
func ParsePolyline(input string) (geom.LineString, error) {
	var coords [][]float64
	if err := json.Unmarshal([]byte(input), &coords); err != nil {
		return geom.LineString{}, fmt.Errorf("failed to parse polyline JSON: %w", err)
	}

	if len(coords) < 2 {
		return geom.LineString{}, fmt.Errorf("polyline must have at least 2 points, got %d", len(coords))
	}

	// Build coordinate sequence for LineString
	flatCoords := make([]float64, 0, len(coords)*2)
	for i, coord := range coords {
		if len(coord) < 2 {
			return geom.LineString{}, fmt.Errorf("coordinate %d has insufficient values", i)
		}
		flatCoords = append(flatCoords, coord[0], coord[1])
	}

	seq := geom.NewSequence(flatCoords, geom.DimXY)
	ls := geom.NewLineString(seq)

	return ls, nil
}

// ParsePolylineToCore parses a JSON array of coordinates into a core.Polyline.
// Input format: "[[x1,y1],[x2,y2],...]"
func ParsePolylineToCore(input string) (core.Polyline, error) {
	var coords [][]float64
	if err := json.Unmarshal([]byte(input), &coords); err != nil {
		return nil, fmt.Errorf("failed to parse polyline JSON: %w", err)
	}

	if len(coords) < 2 {
		return nil, fmt.Errorf("polyline must have at least 2 points, got %d", len(coords))
	}

	polyline := make(core.Polyline, len(coords))
	for i, coord := range coords {
		if len(coord) < 2 {
			return nil, fmt.Errorf("coordinate %d has insufficient values", i)
		}
		polyline[i] = core.Position2D{X: coord[0], Y: coord[1]}
	}

	return polyline, nil
}
