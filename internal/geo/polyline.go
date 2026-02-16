package geo

import (
	"encoding/json"
	"fmt"

	"github.com/OCAP2/extension/v5/pkg/core"
)

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
