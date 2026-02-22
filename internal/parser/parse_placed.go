package parser

import (
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/pkg/core"
)

// ParsePlacedObject parses placed object data into a core PlacedObject.
// Args: [frame, id, className, displayName, "x,y,z", ownerOcapId, side, weapon, magazineIcon]
func (p *Parser) ParsePlacedObject(data []string) (core.PlacedObject, error) {
	var result core.PlacedObject

	if len(data) < 9 {
		return result, fmt.Errorf("insufficient data fields: got %d, need 9", len(data))
	}

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// [0] captureFrameNo
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing captureFrame: %w", err)
	}
	result.JoinFrame = core.Frame(capframe)
	result.JoinTime = time.Now()

	// [1] placedId
	placedID, err := parseUintFromFloat(data[1])
	if err != nil {
		return result, fmt.Errorf("error parsing placedId: %w", err)
	}
	result.ID = uint16(placedID)

	// [2] className
	result.ClassName = data[2]

	// [3] displayName
	result.DisplayName = data[3]

	// [4] position "x,y,z"
	pos3d, err := geo.Position3DFromString(data[4])
	if err != nil {
		return result, fmt.Errorf("error parsing position: %w", err)
	}
	result.Position = pos3d

	// [5] firerOcapId
	ownerID, err := parseUintFromFloat(data[5])
	if err != nil {
		return result, fmt.Errorf("error parsing firerOcapId: %w", err)
	}
	result.OwnerID = uint16(ownerID)

	// [6] side
	result.Side = data[6]

	// [7] weapon
	result.Weapon = data[7]

	// [8] magazineIcon
	result.MagazineIcon = data[8]

	return result, nil
}

// ParsePlacedObjectEvent parses placed object event data into a core PlacedObjectEvent.
// Args: [frame, id, eventType, "x,y,z", hitEntityOcapId?]
func (p *Parser) ParsePlacedObjectEvent(data []string) (core.PlacedObjectEvent, error) {
	var result core.PlacedObjectEvent

	if len(data) < 4 {
		return result, fmt.Errorf("insufficient data fields: got %d, need 4", len(data))
	}

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// [0] captureFrameNo
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return result, fmt.Errorf("error parsing captureFrame: %w", err)
	}
	result.CaptureFrame = core.Frame(capframe)

	// [1] placedId
	placedID, err := parseUintFromFloat(data[1])
	if err != nil {
		return result, fmt.Errorf("error parsing placedId: %w", err)
	}
	result.PlacedID = uint16(placedID)

	// [2] eventType
	result.EventType = data[2]

	// [3] position "x,y,z"
	pos3d, err := geo.Position3DFromString(data[3])
	if err != nil {
		return result, fmt.Errorf("error parsing position: %w", err)
	}
	result.Position = pos3d

	// [4] hitEntityOcapId (optional, only for "hit" events)
	if len(data) >= 5 {
		hitID, err := parseUintFromFloat(data[4])
		if err != nil {
			return result, fmt.Errorf("error parsing hitEntityOcapId: %w", err)
		}
		hitID16 := uint16(hitID)
		result.HitEntityID = &hitID16
	}

	return result, nil
}
