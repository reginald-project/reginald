// Copyright 2025 Antti Kivi
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

// Package plugin implements an RPP server for use in Reginald plugins that are
// written in Go.
package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/reginald-project/reginald/pkg/rpp"
)

// A Plugin is a plugin server that contains the information for running the
// plugin. It holds the implementation of the plugin's commands and tasks.
type Plugin struct {
	name    string
	version string
	in      *bufio.Reader
	out     *bufio.Writer
	configs []rpp.ConfigEntry

	// cmdConfig contains the parsed config values for the command that is
	// currently called by the client.
	cmdConfig []rpp.ConfigEntry
	cmds      []Command
	tasks     []Task
	shutdown  bool // set to true when the plugin should start shutdown
	exit      bool // set to true when the plugin should exit right away
}

// New returns a new Plugin for the given parameters.
func New(name, version string, impls ...any) *Plugin {
	var (
		cmds  []Command
		tasks []Task
	)

	for _, i := range impls {
		switch v := i.(type) {
		case Command:
			cmds = append(cmds, v)
		case Task:
			tasks = append(tasks, v)
		default:
			// TODO: Maybe panicking is too much.
			panic(fmt.Sprintf("invalid plugin implementation type: %T", v))
		}
	}

	return &Plugin{
		name:      name,
		version:   version,
		configs:   []rpp.ConfigEntry{},
		cmdConfig: []rpp.ConfigEntry{},
		cmds:      cmds,
		tasks:     tasks,
		in:        bufio.NewReader(os.Stdin),
		out:       bufio.NewWriter(os.Stdout),
		shutdown:  false,
		exit:      false,
	}
}

// Serve runs the plugin server handling all of the incoming messages. It exits
// gracefully when the client requests for shutdown and exit. It returns an
// error if there is an unrecoverable error in the plugin server.
func (p *Plugin) Serve() error {
	for !p.exit {
		msg, err := rpp.Read(p.in)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		if p.shutdown && msg.Method != rpp.MethodExit {
			err = p.respondError(msg.ID, &rpp.Error{
				Code: rpp.InvalidRequest,
				Message: fmt.Sprintf(
					"method %q was called after the plugin was requested to shut down",
					msg.Method,
				),
				Data: nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}

			return nil
		}

		if err = p.runMethod(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	return nil
}

// handshake handles responding to the handshake method.
func (p *Plugin) handshake(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("Method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	cmdInfos := make([]rpp.CommandInfo, 0, len(p.cmds))
	taskInfos := make([]rpp.TaskInfo, 0, len(p.tasks))

	for _, c := range p.cmds {
		info := rpp.CommandInfo{
			Name:      c.Name(),
			UsageLine: c.UsageLine(),
			Configs:   c.Configs(),
		}
		cmdInfos = append(cmdInfos, info)
	}

	for _, t := range p.tasks {
		info := rpp.TaskInfo{
			Type:    t.Type(),
			Configs: nil,
		}
		taskInfos = append(taskInfos, info)
	}

	result := rpp.HandshakeResult{
		Handshake: rpp.Handshake{
			Protocol:        rpp.Name,
			ProtocolVersion: rpp.Version,
		},
		Name:          p.name,
		Version:       p.version,
		PluginConfigs: p.configs,
		Commands:      cmdInfos,
		Tasks:         taskInfos,
	}

	if err := p.respond(msg.ID, result); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// handshake handles responding to the handshake method.
func (p *Plugin) initialize(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("Method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	var params rpp.InitializeParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: "Failed to decode params",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	for _, cfg := range params.Config {
		i := slices.IndexFunc(p.configs, func(c rpp.ConfigEntry) bool {
			return c.Key == cfg.Key
		})
		if i < 0 {
			err := p.respondError(msg.ID, &rpp.Error{
				Code:    rpp.InvalidParams,
				Message: fmt.Sprintf("Received invalid config value: %q", cfg.Key),
				Data:    nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}

			return nil
		}

		if p.configs[i].Type != cfg.Type {
			err := p.respondError(msg.ID, &rpp.Error{
				Code: rpp.InvalidParams,
				Message: fmt.Sprintf(
					"Invalid type for %q: wanted %s, got %s",
					cfg.Key,
					p.configs[i].Type,
					cfg.Type,
				),
				Data: nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}

			return nil
		}

		p.configs[i].Value = cfg.Value
	}

	// TODO: Handle the logging.

	if err := p.respond(msg.ID, struct{}{}); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// runMethod runs the requested method and responds to it. It returns an error
// when an unrecoverable error is encountered.
//
//nolint:cyclop // need to switch through the methods
func (p *Plugin) runMethod(msg *rpp.Message) error {
	switch msg.Method {
	case rpp.MethodExit:
		if msg.ID != nil {
			err := p.respondError(msg.ID, &rpp.Error{
				Code: rpp.InvalidRequest,
				Message: fmt.Sprintf(
					"method %q was not called using a notification",
					rpp.MethodExit,
				),
				Data: nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}
		}

		p.exit = true
	case rpp.MethodHandshake:
		if err := p.handshake(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	case rpp.MethodInitialize:
		if err := p.initialize(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	case rpp.MethodRunCommand:
		if err := p.runCmd(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	case rpp.MethodRunTask:
		if err := p.runTask(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	case rpp.MethodShutdown:
		if msg.ID == nil {
			err := p.respondError(msg.ID, &rpp.Error{
				Code:    rpp.InvalidRequest,
				Message: fmt.Sprintf("method %q was called using a notification", msg.Method),
				Data:    nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}
		}

		p.shutdown = true

		if err := p.respond(msg.ID, nil); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	case rpp.MethodSetupCommand:
		if err := p.setupCmd(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	case rpp.MethodValidateTask:
		if err := p.validateTask(msg); err != nil {
			return fmt.Errorf("%w", err)
		}
	default:
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.MethodNotFound,
			Message: fmt.Sprintf("invalid method name: %q", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}
	}

	return nil
}

// respond sends a response with the given information to the client.
func (p *Plugin) respond(id, result any) error {
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal call results: %w", err)
	}

	err = rpp.Write(p.out, &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		ID:      id,
		Result:  rawResult,
	})
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	if err = p.out.Flush(); err != nil {
		return fmt.Errorf("flushing the output buffer failed: %w", err)
	}

	return nil
}

// respondError sends an error response instead of the regular response if the
// plugin has encountered an error.
func (p *Plugin) respondError(id any, resErr *rpp.Error) error {
	rawErr, err := json.Marshal(resErr)
	if err != nil {
		return fmt.Errorf("failed to marshal error object: %w", err)
	}

	err = rpp.Write(p.out, &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		ID:      id,
		Error:   rawErr,
	})
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	if err = p.out.Flush(); err != nil {
		return fmt.Errorf("flushing the output buffer failed: %w", err)
	}

	return nil
}

// runCmd runs a command.
func (p *Plugin) runCmd(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("Method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	var params rpp.RunCmdParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: "Failed to decode params",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	i := slices.IndexFunc(p.cmds, func(c Command) bool {
		return c.Name() == params.Name
	})
	if i < 0 {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: fmt.Sprintf("Invalid command name: %q", params.Name),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	cmd := p.cmds[i]

	if err := cmd.Run(p.cmdConfig); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    -32000,
			Message: "Command failed",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := p.respond(msg.ID, struct{}{}); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// runTask runs the "runTask" method.
func (p *Plugin) runTask(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	var params rpp.RunTaskParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: "failed to decode params",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	i := slices.IndexFunc(p.tasks, func(t Task) bool {
		return t.Type() == params.Type
	})
	if i < 0 {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: fmt.Sprintf("invalid task type: %q", params.Type),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := p.tasks[i].Run(params.Dir, params.Config); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InternalError,
			Message: err.Error(),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := p.respond(msg.ID, struct{}{}); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// setupCmd runs the setup method for a command.
func (p *Plugin) setupCmd(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("Method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	var params rpp.SetupCmdParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: "Failed to decode params",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	i := slices.IndexFunc(p.cmds, func(c Command) bool {
		return c.Name() == params.Name
	})
	if i < 0 {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: fmt.Sprintf("Invalid command name: %q", params.Name),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	cmd := p.cmds[i]

	for _, cfg := range params.Config {
		i := slices.IndexFunc(cmd.Configs(), func(c rpp.ConfigEntry) bool {
			return c.Key == cfg.Key
		})
		if i < 0 {
			err := p.respondError(msg.ID, &rpp.Error{
				Code:    rpp.InvalidParams,
				Message: fmt.Sprintf("Received invalid config value: %q", cfg.Key),
				Data:    nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}

			return nil
		}

		c := cmd.Configs()[i]
		if c.Type != cfg.Type {
			err := p.respondError(msg.ID, &rpp.Error{
				Code: rpp.InvalidParams,
				Message: fmt.Sprintf(
					"Invalid type for %q: wanted %s, got %s",
					cfg.Key,
					c.Type,
					cfg.Type,
				),
				Data: nil,
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}

			return nil
		}

		c.Value = cfg.Value

		p.cmdConfig = append(p.cmdConfig, c)
	}

	// TODO: Call the seutp function defined by cmd.

	if err := p.respond(msg.ID, struct{}{}); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// validateTask runs the "validateTask" method.
func (p *Plugin) validateTask(msg *rpp.Message) error {
	if msg.ID == nil {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidRequest,
			Message: fmt.Sprintf("Method %q was called using a notification", msg.Method),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	var params rpp.ValidateTaskParams

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: "Failed to decode params",
			Data:    err,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	i := slices.IndexFunc(p.tasks, func(t Task) bool {
		return t.Type() == params.Type
	})
	if i < 0 {
		err := p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidParams,
			Message: fmt.Sprintf("Invalid task type: %q", params.Type),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := p.tasks[i].Validate(params.Config); err != nil {
		err = p.respondError(msg.ID, &rpp.Error{
			Code:    rpp.InvalidConfig,
			Message: err.Error(),
			Data:    nil,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	if err := p.respond(msg.ID, struct{}{}); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}
