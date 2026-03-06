package parser

import (
	"fmt"

	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/pkg/core"
)

// parseFocusFrame parses a single frame number from args.
// Args: [frameNo]
func (p *Parser) parseFocusFrame(data []string) (core.Frame, error) {
	if len(data) < 1 {
		return 0, fmt.Errorf("expected 1 arg, got %d", len(data))
	}

	cleaned := util.FixEscapeQuotes(util.TrimQuotes(data[0]))

	frame, err := parseUintFromFloat(cleaned)
	if err != nil {
		return 0, fmt.Errorf("error converting frame: %w", err)
	}

	return core.Frame(frame), nil
}

// ParseFocusStart parses a focus start command.
// Args: [frameNo]
func (p *Parser) ParseFocusStart(data []string) (core.Frame, error) {
	return p.parseFocusFrame(data)
}

// ParseFocusEnd parses a focus end command.
// Args: [frameNo]
func (p *Parser) ParseFocusEnd(data []string) (core.Frame, error) {
	return p.parseFocusFrame(data)
}
