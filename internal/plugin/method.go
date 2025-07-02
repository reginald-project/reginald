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
	"fmt"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/log"
)

// exit sends the "exit" notification to the given plugin.
func exit(ctx context.Context, plugin Plugin) error {
	if err := plugin.notify(ctx, api.MethodExit, nil); err != nil {
		return err
	}

	log.Trace(ctx, "exit notification successful", "plugin", plugin.Manifest().Name)

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

	log.Trace(ctx, "handshake successful", "plugin", plugin.Manifest().Name, "result", result)

	return nil
}

// runCommand makes a "runCommand" call to the given plugin.
func runCommand(ctx context.Context, plugin Plugin, name string, cfg api.KeyValues) error {
	params := api.RunCommandParams{
		Cmd:    name,
		Config: cfg,
	}

	var result struct{}
	if err := plugin.call(ctx, api.MethodRunCommand, params, &result); err != nil {
		return err
	}

	log.Trace(ctx, "runCommand call successful", "plugin", plugin.Manifest().Name, "result", result)

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

	log.Trace(ctx, "shutdown call successful", "plugin", plugin.Manifest().Name, "result", result)

	return nil
}
