package parser

import (
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseAce3DeathEvent parses ACE3 death event data and returns a core Ace3DeathEvent.
// SoldierID and LastDamageSourceID are set directly (no cache validation).
func (p *Parser) ParseAce3DeathEvent(data []string) (core.Ace3DeathEvent, error) {
	var deathEvent core.Ace3DeathEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return deathEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	deathEvent.Time = time.Now()
	deathEvent.CaptureFrame = uint(capframe)

	// parse victim ObjectID - set directly
	victimObjectID, err := parseUintFromFloat(data[1])
	if err != nil {
		return deathEvent, fmt.Errorf("error converting victim ocap id to uint: %w", err)
	}
	deathEvent.SoldierID = uint(victimObjectID)

	deathEvent.Reason = data[2]

	// get last damage source id [3]
	lastDamageSourceID, err := parseIntFromFloat(data[3])
	if err != nil {
		return deathEvent, fmt.Errorf("error converting last damage source id to uint: %w", err)
	}

	if lastDamageSourceID > -1 {
		ptr := uint(lastDamageSourceID)
		deathEvent.LastDamageSourceID = &ptr
	}

	return deathEvent, nil
}

// ParseAce3UnconsciousEvent parses ACE3 unconscious event data and returns a core Ace3UnconsciousEvent.
// SoldierID is set directly (no cache validation - worker validates).
func (p *Parser) ParseAce3UnconsciousEvent(data []string) (core.Ace3UnconsciousEvent, error) {
	var unconsciousEvent core.Ace3UnconsciousEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	unconsciousEvent.CaptureFrame = uint(capframe)

	ocapID, err := parseUintFromFloat(data[1])
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting ocap id to uint: %w", err)
	}
	unconsciousEvent.SoldierID = uint(ocapID)

	isUnconscious, err := strconv.ParseBool(data[2])
	if err != nil {
		return unconsciousEvent, fmt.Errorf("error converting isUnconscious to bool: %w", err)
	}
	unconsciousEvent.IsUnconscious = isUnconscious

	return unconsciousEvent, nil
}
