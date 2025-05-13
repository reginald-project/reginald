package logging

import (
	"context"
	"log/slog"
)

// A NullHandler is an [slog.Handler] that discards all logs.
type NullHandler struct{}

// Enabled reports whether the handler handles records at the given level. This
// is always false for NullHandler.
func (h NullHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

// Handle ignores the [slog.Record] passed to NullHandler and returns nil.
func (h NullHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

// WithAttrs returns the NullHandler itself.
func (h NullHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns the NullHandler itself.
func (h NullHandler) WithGroup(_ string) slog.Handler {
	return h
}
