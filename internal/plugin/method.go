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

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/logger"
)

// exit sends the "exit" notification to the given plugin.
func exit(ctx context.Context, plugin Plugin) error {
	if err := plugin.notify(ctx, api.MethodExit, nil); err != nil {
		return err
	}

	slog.Log(ctx, slog.Level(logger.LevelTrace), "exit notification successful", "plugin", plugin.Manifest().Name)

	return nil
}

// handleLog handles running the "log" method request sent from a plugin.
func handleLog(ctx context.Context, plugin Plugin, params *api.LogParams) error {
	level := params.Level

	if !slog.Default().Enabled(ctx, level) {
		return nil
	}

	msg := params.Message
	src := params.Source
	attrs := make([]slog.Attr, 0, len(params.Attrs)+1)

	if src != nil {
		attrs = append(attrs, slog.Any(slog.SourceKey, src))
	}

	for _, a := range params.Attrs {
		attr, err := unmarshalAttr(a)
		if err != nil {
			return err
		}

		attrs = append(attrs, attr)
	}

	attrs = append(attrs, slog.String("plugin", plugin.Manifest().Name))
	t := params.Time
	r := slog.NewRecord(t, level, msg, 0)

	r.AddAttrs(attrs...)

	if err := slog.Default().Handler().Handle(ctx, r); err != nil {
		panic(err)
	}

	return nil
}

// handshake performs the "handshake" method call with the given plugin.
func handshake(ctx context.Context, plugin Plugin) error {
	params := api.DefaultHandshakeParams()

	var result api.HandshakeResult

	if err := plugin.call(ctx, api.MethodHandshake, params, &result); err != nil {
		return err
	}

	switch {
	case params.Protocol != result.Protocol:
		return fmt.Errorf("%w: wrong protocol, want %q, got %q", errHandshake, params.Protocol, result.Protocol)
	case params.ProtocolVersion != result.ProtocolVersion:
		return fmt.Errorf(
			"%w: wrong protocol version, want %q, got %q",
			errHandshake,
			params.ProtocolVersion,
			result.ProtocolVersion,
		)
	case plugin.Manifest().Name != result.Name:
		return fmt.Errorf(
			"%w: mismatching plugin name, want %q, got %q",
			errHandshake,
			plugin.Manifest().Name,
			result.Name,
		)
	}

	slog.Log(
		ctx,
		slog.Level(logger.LevelTrace),
		"handshake successful",
		"plugin",
		plugin.Manifest().Name,
		"result",
		result,
	)

	return nil
}

// runCommand makes a "runCommand" call to the given plugin.
func runCommand(ctx context.Context, plugin Plugin, name string, cfg, pluginCfg api.KeyValues) error {
	params := api.RunCommandParams{
		Cmd:          name,
		Config:       cfg,
		PluginConfig: pluginCfg,
	}

	var result struct{}
	if err := plugin.call(ctx, api.MethodRunCommand, params, &result); err != nil {
		return err
	}

	slog.Log(
		ctx,
		slog.Level(logger.LevelTrace),
		"runCommand successful",
		"plugin",
		plugin.Manifest().Name,
		"result",
		result,
	)

	return nil
}

// shutdown makes a "shutdown" call to the given plugin.
func shutdown(ctx context.Context, plugin Plugin) error {
	var result bool
	if err := plugin.call(ctx, api.MethodShutdown, nil, &result); err != nil {
		return err
	}

	if !result {
		return fmt.Errorf("%w: shutdown returned \"%t\"", errInvalidResponse, result)
	}

	slog.Log(
		ctx,
		slog.Level(logger.LevelTrace),
		"shutdown call successful",
		"plugin",
		plugin.Manifest().Name,
		"result",
		result,
	)

	return nil
}

func unmarshalAttr(attr api.LogAttr) (slog.Attr, error) {
	var val any
	if err := json.Unmarshal(attr.Value, &val); err != nil {
		return slog.Attr{}, fmt.Errorf("failed to unmarshal attribute value: %w", err)
	}

	var value slog.Value

	switch v := val.(type) {
	case bool:
		value = slog.BoolValue(v)
	case float64:
		value = slog.Float64Value(v)
	case int64: // TODO: Might never be the case.
		value = slog.Int64Value(v)
	case string:
		value = slog.StringValue(v)
	case time.Time:
		value = slog.TimeValue(v)
	case []api.LogAttr:
		var as []slog.Attr

		for _, la := range v {
			a, err := unmarshalAttr(la)
			if err != nil {
				return slog.Attr{}, err
			}

			as = append(as, a)
		}

		value = slog.GroupValue(as...)
	default:
		value = slog.AnyValue(v)
	}

	return slog.Attr{
		Key:   attr.Key,
		Value: value,
	}, nil
}
