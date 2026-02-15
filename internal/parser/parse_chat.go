package parser

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"
)

// ParseChatEvent parses chat event data and returns a ChatEvent model.
// SoldierObjectID is set directly (no cache validation - worker validates).
func (p *Parser) ParseChatEvent(data []string) (model.ChatEvent, error) {
	var chatEvent model.ChatEvent

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
	chatEvent.MissionID = p.getMissionID()

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		chatEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
	}

	// channel
	channelInt, err := parseIntFromFloat(data[2])
	if err != nil {
		return chatEvent, fmt.Errorf("error converting channel to int: %w", err)
	}
	channelName, ok := model.ChatChannels[int(channelInt)]
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

// ParseRadioEvent parses radio event data and returns a RadioEvent model.
// SoldierObjectID is set directly (no cache validation - worker validates).
func (p *Parser) ParseRadioEvent(data []string) (model.RadioEvent, error) {
	var radioEvent model.RadioEvent

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
	radioEvent.MissionID = p.getMissionID()

	// parse sender ObjectID
	senderObjectID, err := parseIntFromFloat(data[1])
	if err != nil {
		return radioEvent, fmt.Errorf("error converting sender ocap id to uint: %w", err)
	}

	// Set sender ObjectID if not -1
	if senderObjectID > -1 {
		radioEvent.SoldierObjectID = sql.NullInt32{Int32: int32(senderObjectID), Valid: true}
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
