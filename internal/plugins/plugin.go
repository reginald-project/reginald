// Package plugins implements an RPP client in Reginald to run plugins.
package plugins

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/panichandler"
	"github.com/anttikivi/reginald/internal/pathname"
	"github.com/anttikivi/reginald/pkg/rpp"
)

// Default values associated with the plugin client.
const (
	DefaultHandshakeTimeout  = 5 * time.Second
	DefaultShutdownTimeout   = 15 * time.Second
	DefaultMaxProtocolErrors = 5
)

// Errors returned by plugin utility functions.
var (
	errHandshake     = errors.New("plugin handshake failed")
	errNoParams      = errors.New("notification has no params")
	errNoResponse    = errors.New("plugin disconnected before responding")
	errNotFile       = errors.New("plugin path is not a file")
	errUnknownMethod = errors.New("invalid method")
	errWrongProtocol = errors.New("mismatch in plugin protocol info")
)

// A Plugin represents a plugin that acts as an RPP server and is run from this
// client.
type Plugin struct {
	name           string                       // user-friendly name for the plugin
	kind           string                       // whether this plugin is a task or a command
	flags          []rpp.Flag                   // command-line flags defined by the plugin if it's a command
	cmd            *exec.Cmd                    // command struct used to run the server
	nextID         atomic.Int64                 // next ID to use in RPP call
	stdin          *bufio.Writer                // stdin pipe of the plugin command
	stdout         *bufio.Reader                // buffered reader for the stdout of the plugin
	stderr         *bufio.Scanner               // stderr pipe of the plugin command
	writeMu        sync.Mutex                   // lock for writing to writer to serialize the messages
	pending        map[rpp.ID]chan *rpp.Message // read messages from the plugin waiting for processing
	pendingMu      sync.Mutex                   // lock used with pending
	doneCh         chan error                   // channel to close when the plugin is done running
	protocolErrors atomic.Uint32                // current number of protocol errors for this plugin
}

// New returns a pointer to a newly created Plugin.
func New(ctx context.Context, path string) (*Plugin, error) {
	if ok, err := pathname.IsFile(path); err != nil {
		return nil, fmt.Errorf("failed to check if %s is a file: %w", path, err)
	} else if !ok {
		return nil, fmt.Errorf("%w: %s", errNotFile, path)
	}

	c := exec.CommandContext(ctx, filepath.Clean(path)) // #nosec G204 -- sanitized earlier

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard input pipe for %s: %w",
			filepath.Base(path),
			err,
		)
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard output pipe for %s: %w",
			filepath.Base(path),
			err,
		)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard error pipe for %s: %w",
			filepath.Base(path),
			err,
		)
	}

	p := &Plugin{
		// The base name of the plugin file is used until the real name is
		// received.
		name:           filepath.Base(path),
		kind:           "",
		flags:          nil,
		cmd:            c,
		nextID:         atomic.Int64{},
		stdin:          bufio.NewWriter(stdin),
		stdout:         bufio.NewReader(stdout),
		stderr:         bufio.NewScanner(stderr),
		writeMu:        sync.Mutex{},
		pending:        make(map[rpp.ID]chan *rpp.Message),
		pendingMu:      sync.Mutex{},
		doneCh:         nil, // this is initialized when the plugin has started
		protocolErrors: atomic.Uint32{},
	}

	return p, nil
}

// countProtocolError adds one protocol error to the plugins counter and kills
// the plugin if the maximum threshold for plugin protocol errors is reached.
func (p *Plugin) countProtocolError(ctx context.Context, reason string) {
	n := p.protocolErrors.Add(1)

	slog.WarnContext(ctx, "plugin protocol error", "reason", reason)

	if n >= DefaultMaxProtocolErrors {
		slog.ErrorContext(
			ctx,
			"too many protocol errors, shutting plugin down",
			"plugin",
			p.name,
			"count",
			n,
		)
		iostreams.Errorf("Too many protocol errors by %s, killing the process...", p.name)
		p.kill(ctx)
	}
}

// call calls the given method in the plugin over RPP. It returns the message it
// got as response and possible errors. The message is nil on error.
func (p *Plugin) call(ctx context.Context, method string, params any) (*rpp.Message, error) {
	id := rpp.ID(p.nextID.Add(1))

	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	req := &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		ID:      &id,
		Method:  method,
		Params:  rawParams,
	}

	slog.DebugContext(ctx, "calling method", "plugin", p.name, "msg", req, "params", params)

	// A channel is created for each request. It receives a values in the read loop.
	ch := make(chan *rpp.Message, 1)

	p.pendingMu.Lock()

	p.pending[id] = ch

	p.pendingMu.Unlock()
	p.writeMu.Lock()

	err = rpp.Write(p.stdin, req)
	if err == nil {
		err = p.stdin.Flush()
	}

	p.writeMu.Unlock()

	if err != nil {
		p.cleanPending(id, ch)

		return nil, fmt.Errorf(
			"failed to write call to method %q to plugin %s: %w",
			method,
			p.name,
			err,
		)
	}

	select {
	case res, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("%w: %s (method %s)", errNoResponse, p.name, method)
		}

		if res.Error != nil {
			var rpcErr *rpp.Error

			d := json.NewDecoder(bytes.NewReader(res.Error))
			d.DisallowUnknownFields()

			if err := d.Decode(rpcErr); err != nil {
				return nil, fmt.Errorf("invalid RPP error payload: %w", err)
			}

			return nil, rpcErr
		}

		return res, nil
	case <-ctx.Done():
		slog.DebugContext(ctx, "context canceled during plugin call")
		p.cleanPending(id, ch)

		return nil, fmt.Errorf("%w", ctx.Err())
	}
}

// cleanPending safely cleans up the given ID entry from the pending message
// channels and closes the channel.
func (p *Plugin) cleanPending(id rpp.ID, ch chan *rpp.Message) {
	p.pendingMu.Lock()
	delete(p.pending, id)
	p.pendingMu.Unlock()
	close(ch)
}

// closeAllPending closes all of the pending message channels.
func (p *Plugin) closeAllPending() {
	p.pendingMu.Lock()

	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}

	p.pendingMu.Unlock()
}

// handleNotification handles notifications the client receives from the server.
func (p *Plugin) handleNotification(ctx context.Context, msg *rpp.Message) error {
	var err error

	switch msg.Method {
	case rpp.MethodLog:
		err = p.logNotification(ctx, msg)
	default:
		err = fmt.Errorf("%w: %s", errUnknownMethod, msg.Method)
	}

	return err
}

// handshake performs the RPP handshake with the plugin and sets the relevant
// received values to p. If the handshake fails, the function returns an error.
func (p *Plugin) handshake(ctx context.Context) error {
	params := rpp.DefaultHandshakeParams()

	res, err := p.call(ctx, rpp.MethodHandshake, params)
	if err != nil {
		return fmt.Errorf(
			"method call %q to plugin %s failed: %w",
			rpp.MethodHandshake,
			p.name,
			err,
		)
	}

	// TODO: Disallow unknown fields.
	var result rpp.HandshakeResult
	if err = json.Unmarshal(res.Result, &result); err != nil {
		return fmt.Errorf(
			"failed to unmarshal result for the %q method call to %s: %w",
			rpp.MethodHandshake,
			p.name,
			err,
		)
	}

	if result.Protocol != params.Protocol || result.ProtocolVersion != params.ProtocolVersion {
		return fmt.Errorf("%w, wanted %v, got %v", errWrongProtocol, params, result)
	}

	if result.Name == "" {
		return fmt.Errorf("%w: plugin provided no name", errHandshake)
	}

	p.name = result.Name

	if result.Kind != "command" && result.Kind != "task" {
		return fmt.Errorf("%w: invalid value for \"kind\": %s", errHandshake, result.Kind)
	}

	p.kind = result.Kind

	if result.Kind != "command" && len(result.Flags) > 0 {
		return fmt.Errorf("%w: plugin provided flags even though it is not a command", errHandshake)
	}

	p.flags = append(p.flags, result.Flags...)

	slog.DebugContext(ctx, "handshake succeeded", "plugin", p.name)

	return nil
}

// kill kills the plugin process.
func (p *Plugin) kill(ctx context.Context) {
	if p.cmd.Process != nil {
		slog.DebugContext(ctx, "killing plugin", "plugin", p.name)

		if err := p.cmd.Process.Kill(); err != nil {
			panic(fmt.Sprintf("failed to kill a plugin process: %v", err))
		}
	}

	p.closeAllPending()
}

// logNotification handles the "log" notifications.
func (p *Plugin) logNotification(ctx context.Context, msg *rpp.Message) error {
	if msg.Params == nil {
		return fmt.Errorf("%w: %s", errNoParams, p.name)
	}

	d := json.NewDecoder(bytes.NewReader(msg.Params))
	d.DisallowUnknownFields()

	var params rpp.LogParams

	if err := d.Decode(&params); err != nil {
		return fmt.Errorf("failed to decode params: %w", err)
	}

	attrs := []slog.Attr{slog.String("plugin", p.name)}
	for k, v := range params.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	slog.LogAttrs(ctx, params.Level, params.Message, attrs...)

	return nil
}

// read runs the reading loop for the plugin. It listens to the standard output
// of the plugins and handles the messages when they come in.
func (p *Plugin) read(ctx context.Context, panicHandler func()) {
	defer panicHandler()

	for {
		msg, err := rpp.Read(p.stdout)
		if err != nil {
			// Error when reading or EOF.
			if !errors.Is(err, io.EOF) {
				slog.ErrorContext(ctx, "error when reading plugin output", "err", err)
			}

			p.closeAllPending()

			return
		}

		if msg.JSONRCP != rpp.JSONRCPVersion {
			p.countProtocolError(ctx, "JSON-RCP version mismatch")

			continue
		}

		// The message is a notification.
		if *msg.ID == 0 {
			if msg.Method == "" {
				p.countProtocolError(ctx, "no method in notification")

				continue
			}

			if err := p.handleNotification(ctx, msg); err != nil {
				p.countProtocolError(ctx, err.Error())
			}

			continue
		}

		// Otherwise the message is handled as a response.
		p.pendingMu.Lock()

		ch, ok := p.pending[*msg.ID]
		if ok {
			// Each request should accepts exactly one response, therefore the
			// entry from pending is deleted when the matching response is
			// received.
			delete(p.pending, *msg.ID)
		}

		p.pendingMu.Unlock()

		if ok {
			ch <- msg
			close(ch)
		} else {
			p.countProtocolError(ctx, "response ID does not match any sent request")
		}
	}
}

// readStderr runs a loop for reading the standard error output of the plugin.
func (p *Plugin) readStderr(ctx context.Context, panicHandler func()) {
	defer panicHandler()

	for p.stderr.Scan() {
		line := p.stderr.Text()

		iostreams.PrintErrf("[%s:err] %s\n", p.name, line)
		slog.WarnContext(ctx, "plugin printed to stderr", "plugin", p.name, "output", line)
	}

	if err := p.stderr.Err(); err != nil {
		slog.ErrorContext(ctx, "error reading stderr for plugin", "plugin", p.name, "err", err)
	}
}

// shutdown tries to shut Plugin p down gracefully within the given context.
func (p *Plugin) shutdown(ctx context.Context) error {
	// TODO: Check the response.
	_, err := p.call(ctx, rpp.MethodShutdown, nil)
	if err != nil {
		slog.WarnContext(ctx, "error when calling shutdown", "plugin", p.name, "err", err.Error())
	}

	p.writeMu.Lock()

	err = rpp.Write(p.stdin, &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		Method:  rpp.MethodExit,
	})
	if err == nil {
		err = p.stdin.Flush()
	}

	if err != nil {
		slog.WarnContext(
			ctx,
			"error when sending the exit notification",
			"plugin",
			p.name,
			"err",
			err.Error(),
		)
	}

	p.writeMu.Unlock()

	select {
	case err := <-p.doneCh:
		if err != nil {
			return fmt.Errorf("plugin run returned an error: %w", err)
		}

		return nil
	case <-ctx.Done():
		p.kill(ctx)

		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w", err)
		}

		return nil
	}
}

// start starts the execution of the plugin process and the related reading
// goroutines.
func (p *Plugin) start(ctx context.Context) error {
	slog.DebugContext(ctx, "executing plugin", "path", p.cmd.Path)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("plugin execution from %s failed: %w", p.cmd.Path, err)
	}

	slog.InfoContext(ctx, "started a plugin process", "path", p.cmd.Path, "pid", p.cmd.Process.Pid)

	p.doneCh = make(chan error, 1)

	handlePanic := panichandler.WithStackTrace()

	go p.read(ctx, handlePanic)
	go p.readStderr(ctx, handlePanic)

	go func() {
		defer handlePanic()
		p.doneCh <- p.cmd.Wait()

		close(p.doneCh)
	}()

	return nil
}
