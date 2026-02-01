package logging

import (
	"context"
	"log/slog"
)

// MultiHandler fans out log records to multiple handlers.
// All handlers receive every record.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to all provided handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	// Filter out nil handlers
	valid := make([]slog.Handler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			valid = append(valid, h)
		}
	}
	return &MultiHandler{handlers: valid}
}

// Enabled returns true if any handler is enabled for the given level.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle sends the record to all handlers.
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				// Continue to other handlers even if one fails
				continue
			}
		}
	}
	return nil
}

// WithAttrs returns a new MultiHandler with the given attributes added to all handlers.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

// WithGroup returns a new MultiHandler with the given group added to all handlers.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return m
	}
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}
