package logging

import (
	"context"
	"log/slog"
)

// ContextProvider is a function that returns dynamic context attributes.
type ContextProvider func() []slog.Attr

// ContextHandler wraps another handler and injects dynamic context attributes.
type ContextHandler struct {
	inner    slog.Handler
	provider ContextProvider
}

// NewContextHandler creates a handler that adds dynamic context to each record.
func NewContextHandler(inner slog.Handler, provider ContextProvider) *ContextHandler {
	return &ContextHandler{
		inner:    inner,
		provider: provider,
	}
}

// Enabled delegates to the inner handler.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle adds dynamic context attributes and delegates to the inner handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.provider != nil {
		attrs := h.provider()
		r.AddAttrs(attrs...)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler with the given attributes.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{
		inner:    h.inner.WithAttrs(attrs),
		provider: h.provider,
	}
}

// WithGroup returns a new ContextHandler with the given group.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &ContextHandler{
		inner:    h.inner.WithGroup(name),
		provider: h.provider,
	}
}
