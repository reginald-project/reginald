// Copyright 2025 The Reginald Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
)

// A handler is a wrapper for an [slog.handler] that is used with the logger in
// Reginald.
type handler struct {
	slog.Handler
}

// Handle handles the Record.
func (h *handler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // implements interface
	if r.Level <= slog.LevelDebug {
		src := source(&r)
		r.PC = 0
		r.AddAttrs(slog.Any("source", src))
	}

	if err := h.Handler.Handle(ctx, r); err != nil {
		return fmt.Errorf("failed to handle log record: %w", err)
	}

	return nil
}

// newHandler creates and returns a new Handler.
func newHandler(h slog.Handler) *handler {
	return &handler{h}
}

// source returns a Source for the log event.
func source(r *slog.Record) *slog.Source {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()

	return &slog.Source{
		Function: f.Function,
		File:     f.File,
		Line:     f.Line,
	}
}
