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
// plugin. It holds the implementation of the plugin's task's or command's
// functionality as Impl, which must implement either [Command] or [Task]
type Plugin struct {
	name string
	impl any
	kind string
	in   *bufio.Reader
	out  *bufio.Writer
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
		name: name,
		impl: impl,
		kind: kind,
		in:   bufio.NewReader(os.Stdin),
		out:  bufio.NewWriter(os.Stdout),
	}
}

// Serve runs the plugin server handling all of the incoming messages. It exits
// when the client requests for shutdown and exit.
func (p *Plugin) Serve() {
	for {
		msg, err := rpp.Read(p.in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERROR IN PLUGIN: ", err.Error())

			os.Exit(1)
		}

		if err = p.runMethod(msg); err != nil {
			fmt.Fprintln(os.Stderr, "ERROR IN PLUGIN: ", err.Error())

			os.Exit(1)
		}
	}
}

func (p *Plugin) runMethod(msg *rpp.Message) error {
	switch msg.Method {
	case rpp.MethodHandshake:
		result := rpp.HandshakeResult{
			Protocol:        rpp.Name,
			ProtocolVersion: rpp.Version,
			Kind:            p.kind,
			Name:            p.name,
		}
		if err := p.respond(msg.ID, result); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())

			return fmt.Errorf("response in %s failed: %w", p.name, err)
		}
	}

	return nil
}

func (p *Plugin) respond(id rpp.ID, result any) error {
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
