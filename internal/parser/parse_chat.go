package parser

import (
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseChatEvent parses chat event data and returns a core ChatEvent.
// SoldierID is set directly (no cache validation - worker validates).
func (p *Parser) ParseChatEvent(data []string) (core.ChatEvent, error) {
	var chatEvent core.ChatEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return chatEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	chatEvent.Time = time.Now()
	chatEvent.CaptureFrame = uint(capframe)

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		ptr := uint(senderObjectID)
		chatEvent.SoldierID = &ptr
	}

	// channel
	channelInt, err := parseIntFromFloat(data[2])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting channel to int: %w", err)
	}
	channelName, ok := ChatChannels[int(channelInt)]
	if ok {
		chatEvent.Channel = channelName
	} else {
		if channelInt > 5 && channelInt < 16 {
			chatEvent.Channel = "Custom"
		} else {
			chatEvent.Channel = "System"
		}
	}

	chatEvent.FromName = data[3]
	chatEvent.SenderName = data[4]
	chatEvent.Message = data[5]
	chatEvent.PlayerUID = data[6]

	return chatEvent, nil
}

// ParseRadioEvent parses radio event data and returns a core RadioEvent.
// SoldierID is set directly (no cache validation - worker validates).
func (p *Parser) ParseRadioEvent(data []string) (core.RadioEvent, error) {
	var radioEvent core.RadioEvent

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// get frame
	capframe, err := strconv.ParseFloat(data[0], 64)
	if err != nil {
		return radioEvent, fmt.Errorf("error converting capture frame to int: %w", err)
	}

	radioEvent.Time = time.Now()
	radioEvent.CaptureFrame = uint(capframe)

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		ptr := uint(senderObjectID)
		radioEvent.SoldierID = &ptr
	}

	radioEvent.Radio = data[2]
	radioEvent.RadioType = data[3]
	radioEvent.StartEnd = data[4]

	channelInt, err := parseIntFromFloat(data[5])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting channel to int: %w", err)
	}
	radioEvent.Channel = int8(channelInt)

	isAddtl, err := strconv.ParseBool(data[6])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting isAddtl to bool: %w", err)
	}
	radioEvent.IsAdditional = isAddtl

	freq, err := strconv.ParseFloat(data[7], 64)
	if err != nil {
		return radioEvent, fmt.Errorf("error converting freq to float: %w", err)
	}
	radioEvent.Frequency = float32(freq)

	radioEvent.Code = data[8]

	return radioEvent, nil
}
