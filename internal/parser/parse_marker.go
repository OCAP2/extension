package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseMarkerCreate parses marker create data and returns a core Marker
func (p *Parser) ParseMarkerCreate(data []string) (core.Marker, error) {
	var marker core.Marker

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// markerName
	marker.MarkerName = data[0]

	// direction
	dir, err := strconv.ParseFloat(data[1], 32)
	if err != nil {
		return marker, fmt.Errorf("error parsing direction: %w", err)
	}
	marker.Direction = float32(dir)

	// type
	marker.MarkerType = data[2]

	// text
	marker.Text = data[3]

	// frameNo
	capframe, err := strconv.ParseFloat(data[4], 64)
	if err != nil {
		return marker, fmt.Errorf("error parsing capture frame: %w", err)
	}
	marker.CaptureFrame = uint(capframe)

	// data[5] is -1, skip

	// ownerId
	ownerId, err := strconv.Atoi(data[6])
	if err != nil {
		p.logger.Warn("Error parsing ownerId", "error", err)
		ownerId = -1
	}
	marker.OwnerID = ownerId

	// color
	marker.Color = data[7]

	// size
	marker.Size = data[8]

	// side
	marker.Side = data[9]

	// shape - read first to determine position format
	marker.Shape = data[11]

	// position - parse based on shape
	pos := data[10]
	if marker.Shape == "POLYLINE" {
		polyline, err := geo.ParsePolylineToCore(pos)
		if err != nil {
			return marker, fmt.Errorf("error parsing polyline: %w", err)
		}
		marker.Polyline = polyline
	} else {
		pos = strings.TrimPrefix(pos, "[")
		pos = strings.TrimSuffix(pos, "]")
		pos3d, err := geo.Position3DFromString(pos)
		if err != nil {
			return marker, fmt.Errorf("error parsing position: %w", err)
		}
		marker.Position = pos3d
	}

	// alpha
	alpha, err := strconv.ParseFloat(data[12], 32)
	if err != nil {
		p.logger.Warn("Error parsing alpha", "error", err)
		alpha = 1.0
	}
	marker.Alpha = float32(alpha)

	// brush
	marker.Brush = data[13]

	marker.Time = time.Now()
	marker.IsDeleted = false

	return marker, nil
}

// ParseMarkerMove parses marker move data into a MarkerMove.
// The MarkerName is returned for the worker to resolve to a MarkerID via MarkerCache.
func (p *Parser) ParseMarkerMove(data []string) (MarkerMove, error) {
	var result MarkerMove

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// markerName - return for worker to resolve
	result.MarkerName = data[0]

	// frameNo
	capframe, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing capture frame: %w", err)
	}
	result.CaptureFrame = uint(capframe)

	// position
	pos := data[2]
	pos = strings.TrimPrefix(pos, "[")
	pos = strings.TrimSuffix(pos, "]")
	pos3d, err := geo.Position3DFromString(pos)
	if err != nil {
		return result, fmt.Errorf("error parsing position: %w", err)
	}
	result.Position = pos3d

	// direction
	dir, err := strconv.ParseFloat(data[3], 32)
	if err != nil {
		p.logger.Warn("Error parsing direction", "error", err)
		dir = 0
	}
	result.Direction = float32(dir)

	// alpha
	alpha, err := strconv.ParseFloat(data[4], 32)
	if err != nil {
		p.logger.Warn("Error parsing alpha", "error", err)
		alpha = 1.0
	}
	result.Alpha = float32(alpha)

	result.Time = time.Now()

	return result, nil
}

// ParseMarkerDelete parses marker delete data and returns the marker name and frame number
func (p *Parser) ParseMarkerDelete(data []string) (string, uint, error) {
	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	markerName := data[0]

	capframe, err := strconv.ParseFloat(data[1], 64)
	if err != nil {
		return markerName, 0, fmt.Errorf("error parsing capture frame: %w", err)
	}

	return markerName, uint(capframe), nil
}
