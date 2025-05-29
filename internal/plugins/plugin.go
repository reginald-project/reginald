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
	"sync"
	"sync/atomic"
	"time"

	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/panichandler"
	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/spf13/afero"
)

// Default values associated with the plugin client.
const (
	DefaultHandshakeTimeout  = 5 * time.Second
	DefaultShutdownTimeout   = 15 * time.Second
	DefaultMaxProtocolErrors = 5
)

// Errors returned by plugin utility functions.
var (
	errCommandNotFound = errors.New("given command is not present in the plugin")
	errHandshake       = errors.New("plugin handshake failed")
	errNoParams        = errors.New("notification has no params")
	errNoResponse      = errors.New("plugin disconnected before responding")
	errNotFile         = errors.New("plugin path is not a file")
	errUnknownMethod   = errors.New("invalid method")
	errWrongProtocol   = errors.New("mismatch in plugin protocol info")
)

// A Plugin represents a plugin that acts as an RPP server and is run from this
// client.
type Plugin struct {
	rpp.HandshakeResult

	// lastID is the ID that was last used in a method call. Even though
	// the protocol supports both strings and ints as the ID, we just default to
	// ints to make the client more reasonable.
	lastID atomic.Int64

	// cmd is the underlying command running the plugin process.
	cmd *exec.Cmd

	// stdin is the standard input pipe of the underlying command as a buffered
	// writer.
	stdin *bufio.Writer

	// stdout is the standard output pipe of the underlying command as
	// a buffered reader.
	stdout *bufio.Reader

	// stderr is the standard error pipe of the underlying command wrapped in
	// a scanner.
	stderr *bufio.Scanner

	// writeMu is used to lock writing to the plugins standard input to
	// serialize the message.
	writeMu sync.Mutex

	// pending is used to store channels for the method calls that when calling
	// the method. Each method has its own channel stored by ID as
	// an JSON-encoded string in pending. The IDs are JSON-encoded to avoid
	// collisions between a string ID having an int in it and to avoid the fact
	// the actual type of the IDs, any, cannot be used. The reading loop gets
	// the channel from pending and checks if the method calling functions
	// writes a response to it. The the read function passes the response to
	// the channel so the method call function can return the result we got from
	// the plugin.
	pending map[string]chan *rpp.Message

	// pendingMu is used to lock pending.
	pendingMu sync.Mutex

	// doneCh is closed when the plugin is done running.
	doneCh chan error

	// protocolErrors is a counter of protocol errors this plugin has made. If
	// the number of protocol errors in a plugin exceeds a threshold, the plugin
	// is shut down.
	protocolErrors atomic.Uint32
}

// New returns a pointer to a newly created Plugin.
func New(ctx context.Context, fs afero.Fs, path fspath.Path) (*Plugin, error) {
	if ok, err := path.IsFile(fs); err != nil {
		return nil, fmt.Errorf("failed to check if %s is a file: %w", path, err)
	} else if !ok {
		return nil, fmt.Errorf("%w: %s", errNotFile, path)
	}

	c := exec.CommandContext(ctx, string(path.Clean())) // #nosec G204 -- sanitized earlier

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard input pipe for %s: %w",
			path.Base(),
			err,
		)
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard output pipe for %s: %w",
			path.Base(),
			err,
		)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create standard error pipe for %s: %w",
			path.Base(),
			err,
		)
	}

	p := &Plugin{
		// The base name of the plugin file is used until the real name is
		// received.
		HandshakeResult: rpp.HandshakeResult{
			Handshake:     rpp.DefaultHandshakeParams().Handshake,
			Name:          string(path.Base()),
			PluginConfigs: []rpp.ConfigValue{},
			Commands:      []rpp.CommandInfo{},
			Tasks:         []rpp.TaskInfo{},
		},
		lastID:         atomic.Int64{},
		cmd:            c,
		stdin:          bufio.NewWriter(stdin),
		stdout:         bufio.NewReader(stdout),
		stderr:         bufio.NewScanner(stderr),
		writeMu:        sync.Mutex{},
		pending:        make(map[string]chan *rpp.Message),
		pendingMu:      sync.Mutex{},
		doneCh:         nil, // this is initialized when the plugin has started
		protocolErrors: atomic.Uint32{},
	}

	return p, nil
}

// RunCmd runs a command with the given name from this plugin. It calls
// the plugin in order to invoke the "run/<name>" method which is supposed to
// run the commands functionality.
func (p *Plugin) RunCmd(ctx context.Context, name string, args []string) error {
	ok := false

	for _, c := range p.Commands {
		if c.Name == name {
			ok = true

			break
		}
	}

	if !ok {
		return fmt.Errorf("%w: command %q in plugin %q", errCommandNotFound, name, p.Name)
	}

	params := rpp.RunCmdParams{
		Name: name,
		Args: args,
	}
	method := rpp.MethodRunCommand

	res, err := p.call(ctx, method, params)
	if err != nil {
		return fmt.Errorf(
			"method call %q to plugin %s failed: %w",
			method,
			p.Name,
			err,
		)
	}

	// TODO: Add some sensible return type.
	var result any
	if err = json.Unmarshal(res.Result, &result); err != nil {
		return fmt.Errorf(
			"failed to unmarshal result for the %q method call to %s: %w",
			method,
			p.Name,
			err,
		)
	}

	logging.DebugContext(ctx, "running command succeeded", "plugin", p.Name, "result", result)

	return nil
}

// countProtocolError adds one protocol error to the plugins counter and kills
// the plugin if the maximum threshold for plugin protocol errors is reached.
func (p *Plugin) countProtocolError(ctx context.Context, reason string) {
	n := p.protocolErrors.Add(1)

	logging.WarnContext(ctx, "plugin protocol error", "reason", reason)

	if n >= DefaultMaxProtocolErrors {
		logging.ErrorContext(
			ctx,
			"too many protocol errors, shutting plugin down",
			"plugin",
			p.Name,
			"count",
			n,
		)
		iostreams.Errorf("Too many protocol errors by %s, killing the process...", p.Name)
		p.kill(ctx)
	}
}

// call calls the given method in the plugin over RPP. It returns the message it
// got as response and possible errors. The message is nil on error.
func (p *Plugin) call(ctx context.Context, method string, params any) (*rpp.Message, error) {
	id := p.lastID.Add(1) //nolint:varnamelen

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

	logging.TraceContext(ctx, "calling method", "plugin", p.Name, "method", method, "req", req)

	// A channel is created for each request. It receives a values in the read
	// loop.
	ch := make(chan *rpp.Message, 1) //nolint:varnamelen

	p.pendingMu.Lock()

	p.pending[idToKey(id)] = ch

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
			p.Name,
			err,
		)
	}

	select {
	case res, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("%w: %s (method %s)", errNoResponse, p.Name, method)
		}

		logging.TraceContext(ctx, "received response", "plugin", p.Name, "res", res)

		if res.Error != nil {
			var rpcErr rpp.Error

			d := json.NewDecoder(bytes.NewReader(res.Error))
			d.DisallowUnknownFields()

			if err := d.Decode(&rpcErr); err != nil {
				return nil, fmt.Errorf("invalid RPP error payload: %w", err)
			}

			return nil, &rpcErr
		}

		return res, nil
	case <-ctx.Done():
		logging.TraceContext(ctx, "context canceled during plugin call")
		p.cleanPending(id, ch)

		return nil, fmt.Errorf("%w", ctx.Err())
	}
}

// cleanPending safely cleans up the given ID entry from the pending message
// channels and closes the channel.
func (p *Plugin) cleanPending(id any, ch chan *rpp.Message) {
	p.pendingMu.Lock()
	delete(p.pending, idToKey(id))
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
			p.Name,
			err,
		)
	}

	// TODO: Disallow unknown fields.
	var result rpp.HandshakeResult
	if err = json.Unmarshal(res.Result, &result); err != nil {
		return fmt.Errorf(
			"failed to unmarshal result for the %q method call to %s: %w",
			rpp.MethodHandshake,
			p.Name,
			err,
		)
	}

	if result.Protocol != params.Protocol || result.ProtocolVersion != params.ProtocolVersion {
		return fmt.Errorf("%w, wanted %v, got %v", errWrongProtocol, params, result)
	}

	if result.Name == "" {
		return fmt.Errorf("%w: plugin provided no name", errHandshake)
	}

	for _, t := range result.Tasks {
		for _, c := range t.Configs {
			if c.Flag != nil {
				return fmt.Errorf(
					"%w: plugin %q defined flag %q for task %q",
					errHandshake,
					result.Name,
					c.Key,
					t.Name,
				)
			}

			if c.FlagOnly {
				return fmt.Errorf(
					"%w: plugin %q marked config %q for task %q as flag only",
					errHandshake,
					result.Name,
					c.Key,
					t.Name,
				)
			}
		}
	}

	p.HandshakeResult = result

	logging.TraceContext(ctx, "handshake succeeded", "plugin", p.Name)

	return nil
}

// kill kills the plugin process.
func (p *Plugin) kill(ctx context.Context) {
	if p.cmd.Process != nil {
		logging.DebugContext(ctx, "killing plugin", "plugin", p.Name)

		if err := p.cmd.Process.Kill(); err != nil {
			panic(fmt.Sprintf("failed to kill a plugin process: %v", err))
		}
	}

	p.closeAllPending()
}

// logNotification handles the "log" notifications.
func (p *Plugin) logNotification(ctx context.Context, msg *rpp.Message) error {
	if msg.Params == nil {
		return fmt.Errorf("%w: %s", errNoParams, p.Name)
	}

	d := json.NewDecoder(bytes.NewReader(msg.Params))
	d.DisallowUnknownFields()

	var params rpp.LogParams

	if err := d.Decode(&params); err != nil {
		return fmt.Errorf("failed to decode params: %w", err)
	}

	attrs := []slog.Attr{slog.String("plugin", p.Name)}
	for k, v := range params.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	logging.LogAttrs(ctx, params.Level, params.Message, attrs...)

	return nil
}

// notify send a notification to the plugin.
func (p *Plugin) notify(ctx context.Context, method string, params any) error {
	logging.DebugContext(
		ctx,
		"sending notification",
		"plugin",
		p.Name,
		"method",
		method,
		"params",
		params,
	)

	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	err = rpp.Write(p.stdin, &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		Method:  method,
		Params:  rawParams,
	})
	if err == nil {
		err = p.stdin.Flush()
	}

	if err != nil {
		return fmt.Errorf("error when sending notification: %w", err)
	}

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
				logging.ErrorContext(ctx, "error when reading plugin output", "err", err)
			}

			p.closeAllPending()

			return
		}

		if msg.JSONRCP != rpp.JSONRCPVersion {
			p.countProtocolError(ctx, "JSON-RCP version mismatch")

			continue
		}

		// The message is a notification.
		if msg.ID == nil {
			// To be a valid notification, the message must have a method.
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

		ch, ok := p.pending[idToKey(msg.ID)]
		if ok {
			// Each request should accepts exactly one response, therefore the
			// entry from pending is deleted when the matching response is
			// received.
			delete(p.pending, idToKey(msg.ID))
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

		iostreams.PrintErrf("[%s:err] %s\n", p.Name, line)
		logging.WarnContext(ctx, "plugin printed to stderr", "plugin", p.Name, "output", line)
	}

	if err := p.stderr.Err(); err != nil {
		logging.ErrorContext(ctx, "error reading stderr for plugin", "plugin", p.Name, "err", err)
	}
}

// shutdown tries to shut Plugin p down gracefully within the given context.
func (p *Plugin) shutdown(ctx context.Context) error {
	// TODO: Check the response.
	_, err := p.call(ctx, rpp.MethodShutdown, nil)
	if err != nil {
		logging.WarnContext(
			ctx,
			"error when calling shutdown",
			"plugin",
			p.Name,
			"err",
			err.Error(),
		)
	}

	err = p.notify(ctx, rpp.MethodExit, nil)
	if err != nil {
		logging.WarnContext(
			ctx,
			"error when sending the exit notification",
			"plugin",
			p.Name,
			"err",
			err.Error(),
		)
	}

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
	logging.TraceContext(ctx, "executing plugin", "path", p.cmd.Path)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("plugin execution from %s failed: %w", p.cmd.Path, err)
	}

	logging.DebugContext(
		ctx,
		"started a plugin process",
		"path",
		p.cmd.Path,
		"pid",
		p.cmd.Process.Pid,
	)

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

// idToKey is a helper function that converts the given message ID into a string
// that can be used as a key in pending channels map of the plugin client.
func idToKey(id any) string {
	b, err := json.Marshal(id)
	if err != nil {
		panic(fmt.Sprintf("failed to convert ID %v to JSON-encoded value: %v", id, err))
	}

	return string(b)
}
