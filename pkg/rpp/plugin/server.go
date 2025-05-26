// Package plugin implements an RPP server for use in Reginald plugins that are
// written in Go.
package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp"
)

// A Plugin is a plugin server that contains the information for running the
// plugin. It holds the implementation of the plugin's commands and tasks.
type Plugin struct {
	name     string
	cmds     []Command
	tasks    []Task
	in       *bufio.Reader
	out      *bufio.Writer
	shutdown bool // set to true when the plugin should start shutdown
	exit     bool // set to true when the plugin should exit right away
}

// New returns a new Plugin for the given parameters.
func New(name string, impls ...any) *Plugin {
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
		name:     name,
		cmds:     cmds,
		tasks:    tasks,
		in:       bufio.NewReader(os.Stdin),
		out:      bufio.NewWriter(os.Stdout),
		shutdown: false,
		exit:     false,
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
			err := p.respondError(msg.ID, &rpp.Error{
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
	}

	cmdInfos := make([]rpp.CommandInfo, 0, len(p.cmds))
	taskInfos := make([]rpp.TaskInfo, 0, len(p.tasks))

	for _, c := range p.cmds {
		info := rpp.CommandInfo{
			Name:      c.Name(),
			UsageLine: c.UsageLine(),
			Flags:     c.Flags(),
			Configs:   nil,
		}
		cmdInfos = append(cmdInfos, info)
	}

	for _, t := range p.tasks {
		info := rpp.TaskInfo{
			Name:    t.Name(),
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
		PluginConfigs: nil,
		Commands:      cmdInfos,
		Tasks:         taskInfos,
	}

	if err := p.respond(msg.ID, result); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// runMethod runs the requested method and responds to it. It returns an error
// when an unrecoverable error is encountered.
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
	case rpp.MethodRunCommand:
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

		fmt.Fprintln(os.Stderr, "RUNNING COMMAND")

		if err := p.respond(msg.ID, nil); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
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
