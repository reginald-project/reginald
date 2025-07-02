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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"slices"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald-sdk-go/logs"
)

// An RPCHandler is a handler for slog that can be used for logging in
// the plugins. It sends the logging messages as JSON-RPC notifications to
// the client that handles writing the logs.
type RPCHandler struct {
	server *Server
	groups []string    // all groups started from WithGroup
	attrs  []slog.Attr // all attributes set with WithAttrs
	opts   RPCHandlerOptions
}

// RPCHandlerOptions are the options for the RPCHandler.
type RPCHandlerOptions struct {
	// AddSource causes the handler to compute the source code position
	// of the log statement and add a SourceKey attribute to the output.
	AddSource bool
}

// NewRPCHandler creates a NewRPCHandler for the given Server.
func NewRPCHandler(s *Server, opts *RPCHandlerOptions) *RPCHandler {
	if opts == nil {
		opts = &RPCHandlerOptions{AddSource: true}
	}

	return &RPCHandler{
		server: s,
		opts:   *opts,
		groups: nil,
		attrs:  nil,
	}
}

// Enabled reports whether the handler handles records at the given level.
// The handler ignores records whose level is lower.
func (*RPCHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle collects the level, attributes, and message in parameters and sends
// a "log" notification to the client.
//
//nolint:gocritic // implements interface
func (h *RPCHandler) Handle(_ context.Context, r slog.Record) error {
	var params api.LogParams

	params.Time = r.Time
	params.Level = logs.Level(r.Level)
	params.Message = r.Message

	if h.opts.AddSource {
		params.Source = source(&r)
	} else {
		params.Source = nil
	}

	logAttrs := make([]api.LogAttr, 0, r.NumAttrs())

	if r.NumAttrs() > 0 {
		var (
			err     error
			logAttr api.LogAttr
		)

		r.Attrs(func(a slog.Attr) bool {
			logAttr, err = toLogAttr(a)
			if err != nil {
				return false
			}

			logAttrs = append(logAttrs, logAttr)

			return true
		})
	}

	if len(h.groups) > 0 {
		var groupAttr api.LogAttr

		for i, s := range slices.Backward(h.groups) {
			var (
				err error
				raw json.RawMessage
			)

			if i == 0 {
				raw, err = json.Marshal(logAttrs)
			} else {
				raw, err = json.Marshal(groupAttr)
			}

			if err != nil {
				return fmt.Errorf("failed to marshal group attributes: %w", err)
			}

			groupAttr = api.LogAttr{
				Key:   s,
				Value: raw,
			}
		}

		logAttrs = []api.LogAttr{groupAttr}
	}

	params.Attrs = logAttrs

	return h.server.notify(api.MethodLog, params)
}

// WithAttrs returns a new Handler whose attributes consist of both
// the receiver's attributes and the arguments. The Handler owns the slice: it
// may retain, modify, or discard it.
func (h *RPCHandler) WithAttrs(as []slog.Attr) slog.Handler {
	h2 := h.clone()
	h2.attrs = append(h2.attrs, as...)

	return h2
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups. The keys of all subsequent attributes,
// whether added by With or in a Record, should be qualified by the sequence of
// group names.
//
// How this qualification happens is up to the Handler, so long as this
// Handler's attribute keys differ from those of another Handler with
// a different sequence of group names.
//
// A Handler should treat WithGroup as starting a Group of Attrs that ends at
// the end of the log event. That is,
//
//	logger.WithGroup("s").LogAttrs(ctx, level, msg, slog.Int("a", 1), slog.Int("b", 2))
//
// should behave like
//
//	logger.LogAttrs(ctx, level, msg, slog.Group("s", slog.Int("a", 1), slog.Int("b", 2)))
//
// If the name is empty, WithGroup returns the receiver.
func (h *RPCHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	h2 := h.clone()
	h2.groups = append(h2.groups, name)

	return h2
}

// clone returns a clone of the RPCHandler. The cloned instance includes pointer
// to the same server.
func (h *RPCHandler) clone() *RPCHandler {
	return &RPCHandler{
		server: h.server,
		opts:   h.opts,
		groups: slices.Clip(slices.Clone(h.groups)),
		attrs:  slices.Clip(slices.Clone(h.attrs)),
	}
}

// toLogAttr creates an api.LogAttr from an slog.Attr.
func toLogAttr(attr slog.Attr) (api.LogAttr, error) {
	var (
		err error
		raw json.RawMessage
	)

	// attr.Value = attr.Value.Resolve()

	switch attr.Value.Kind() {
	case slog.KindAny:
		raw, err = json.Marshal(attr.Value.Any())
	case slog.KindBool:
		raw, err = json.Marshal(attr.Value.Bool())
	case slog.KindDuration:
		raw, err = json.Marshal(attr.Value.Duration())
	case slog.KindFloat64:
		raw, err = json.Marshal(attr.Value.Float64())
	case slog.KindInt64:
		raw, err = json.Marshal(attr.Value.Int64())
	case slog.KindString:
		raw, err = json.Marshal(attr.Value.String())
	case slog.KindTime:
		raw, err = json.Marshal(attr.Value.Time())
	case slog.KindUint64:
		raw, err = json.Marshal(attr.Value.Uint64())
	case slog.KindGroup:
		var logAttrs []api.LogAttr

		attrs := attr.Value.Group()
		for _, a := range attrs {
			var la api.LogAttr

			if la, err = toLogAttr(a); err != nil {
				return api.LogAttr{}, err
			}

			logAttrs = append(logAttrs, la)
		}

		raw, err = json.Marshal(logAttrs)
	case slog.KindLogValuer:
		attr.Value = attr.Value.Resolve()

		return toLogAttr(attr)
	default:
		panic(fmt.Sprintf("invalid attribute kind: %v", attr.Value.Kind()))
	}

	if err != nil {
		return api.LogAttr{}, fmt.Errorf("failed to marshal attribute value: %w", err)
	}

	return api.LogAttr{
		Key:   attr.Key,
		Value: raw,
	}, nil
}

// source returns a slog.Source for the log event. If the Record was created
// without the necessary information, or if the location is unavailable, it
// returns a non-nil *Source with zero fields.
func source(r *slog.Record) *slog.Source {
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()

	return &slog.Source{
		Function: f.Function,
		File:     f.File,
		Line:     f.Line,
	}
}
