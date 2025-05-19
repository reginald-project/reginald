// Package plugin implements an RPP server for use in Reginald plugins that are
// written in Go.
package plugin

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp"
)

// Errors returned by the server functions when there is an unrecoverable
// failure.
var (
	errImplType = errors.New("failed to cast impl to the expected type")
)

// A Plugin is a plugin server that contains the information for running the
// plugin. It holds the implementation of the plugin's task's or command's
// functionality as Impl, which must implement either [Command] or [Task].
type Plugin struct {
	name     string
	impl     any
	kind     string
	in       *bufio.Reader
	out      *bufio.Writer
	shutdown bool // set to true when the plugin should start shutdown
	exit     bool // set to true when the plugin should exit right away
}

// New returns a new Plugin for the given parameters.
func New(name string, impl any) *Plugin {
	var kind string

	switch v := impl.(type) {
	case Command:
		kind = "command"
	case Task:
		kind = "task"
	default:
		// TODO: Maybe panicking is too much.
		panic(fmt.Sprintf("invalid plugin implementation type: %T", v))
	}

	return &Plugin{
		name:     name,
		impl:     impl,
		kind:     kind,
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

		// TODO: When shutdown has started, only "exit" will be accepted.
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

	result := rpp.HandshakeResult{
		Protocol:        rpp.Name,
		ProtocolVersion: rpp.Version,
		Kind:            p.kind,
		Name:            p.name,
		Flags:           nil,
	}

	if p.kind == "command" {
		impl, ok := p.impl.(Command)
		if !ok {
			// Let's assume we should not recover from this.
			return fmt.Errorf("%w: %T", errImplType, p.impl)
		}

		result.Flags = append(result.Flags, impl.Flags()...)
	}

	if err := p.respond(msg.ID, result); err != nil {
		return fmt.Errorf("response in %s failed: %w", p.name, err)
	}

	return nil
}

// runMethod runs the requested method and responds to it. It returns an error
// when an unrecoverable error is encountered.
func (p *Plugin) runMethod(msg *rpp.Message) error {
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
func (p *Plugin) respond(id *rpp.ID, result any) error {
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
func (p *Plugin) respondError(id *rpp.ID, resErr *rpp.Error) error {
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
