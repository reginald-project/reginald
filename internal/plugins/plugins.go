// Package plugins implements an RPP client in Reginald to run plugins.
package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/anttikivi/reginald/internal/pathname"
	"github.com/anttikivi/reginald/pkg/rpp"
	"golang.org/x/sync/errgroup"
)

// Errors returned by plugin utility functions.
var (
	errHandshake     = errors.New("plugin handshake failed")
	errNoResponse    = errors.New("plugin disconnected before responding")
	errNotFile       = errors.New("plugin path is not a file")
	errWrongProtocol = errors.New("mismatch in plugin protocol info")
)

// A Plugin represents a plugin that acts as an RPP server and is run from this
// client.
type Plugin struct {
	name        string                       // user-friendly name for the plugin
	kind        string                       // whether this plugin is a task or a command
	flags       []string                     // command-line flags defined by the plugin if it's a command
	cmd         *exec.Cmd                    // command struct used to run the server
	nextID      atomic.Uint64                // next ID to use in RPP call
	stdin       *bufio.Writer                // stdin pipe of the plugin command
	stdout      *bufio.Reader                // buffered reader for the stdout of the plugin
	stderr      *bufio.Scanner               // stderr pipe of the plugin command
	writeLock   sync.Mutex                   // lock for writing to writer to serialize the messages
	pending     map[rpp.ID]chan *rpp.Message // read messages from the plugin waiting for processing
	pendingLock sync.Mutex                   // lock used with pending
	doneCh      chan error                   // channel to close when the plugin is done running
}

// New returns a pointer to a newly created Plugin.
func New(ctx context.Context, path string) (*Plugin, error) {
	if ok, err := pathname.IsFile(path); err != nil {
		return nil, fmt.Errorf("failed to check if %s is a file: %w", path, err)
	} else if !ok {
		return nil, fmt.Errorf("%w: %s", errNotFile, path)
	}

	c := exec.CommandContext(ctx, filepath.Clean(path))

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
		return nil, fmt.Errorf("failed to create standard error pipe for %s: %w", filepath.Base(path), err)
	}

	p := &Plugin{
		name: filepath.Base(
			path,
		), // the base name of the plugin is used until the real name is received
		kind:        "",
		flags:       nil,
		cmd:         c,
		nextID:      atomic.Uint64{},
		stdin:       bufio.NewWriter(stdin),
		stdout:      bufio.NewReader(stdout),
		stderr:      bufio.NewScanner(stderr),
		writeLock:   sync.Mutex{},
		pending:     make(map[rpp.ID]chan *rpp.Message),
		pendingLock: sync.Mutex{},
		doneCh:      nil, // this is initialized when the plugin has started
	}

	return p, nil
}

// Load creates the processes for the plugins, performs the handshakes with
// them, returns a slice of the valid plugins.
func Load(ctx context.Context, files []string) ([]*Plugin, error) {
	// TODO: Provide the config value for the create function.
	plugins, err := loadAll(ctx, files, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load the plugins: %w", err)
	}

	for _, p := range plugins {
		go func(p *Plugin) {
			if err := <-p.doneCh; err != nil {
				// TODO: Better logging or something.
				fmt.Fprintf(os.Stderr, "plugin %q quit unexpectedly: %v\n", p.name, err)
			}
		}(p)
	}

	return plugins, nil
}

// loadAll creates and starts all of the plugin processes and performs the
// handshake with them. If ignoreErrors is true, the function simply drops
// plugins that cause errors when starting or fail the handshake. Otherwise the
// function fails fast.
func loadAll(ctx context.Context, files []string, ignoreErrors bool) ([]*Plugin, error) {
	var (
		lock    sync.Mutex
		plugins []*Plugin
	)

	eg, egctx := errgroup.WithContext(ctx)

	// TODO: Print the errors to actual output if they are ignored.
	for _, f := range files {
		eg.Go(func() error {
			p, err := New(ctx, f)
			if err != nil {
				return fmt.Errorf("failed to create a new plugin for path %s; %w", f, err)
			}

			if err := p.start(); err != nil {
				if ignoreErrors {
					slog.Warn("failed to start plugin", "path", f, "err", err)

					return nil
				}

				return fmt.Errorf("failed to start plugin %s: %w", p.name, err)
			}

			if err := p.handshake(egctx); err != nil {
				if ignoreErrors {
					slog.Warn("handshake failed", "path", f, "err", err)

					return nil
				}

				return fmt.Errorf("handshake for plugin %s failed: %w", p.name, err)
			}

			// I'm not sure about using locks but it's simple and gets the job
			// done.
			lock.Lock()

			plugins = append(plugins, p)

			lock.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return plugins, nil
}

// call calls the given method in the plugin over RPP. It returns the message it
// got as response and possible errors. The message is nil on error.
func (p *Plugin) call(ctx context.Context, method string, params any) (*rpp.Message, error) {
	id := rpp.ID(p.nextID.Add(1))
	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	req := &rpp.Message{ //nolint:exhaustruct
		JSONRCP: rpp.JSONRCPVersion,
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	// A channel is created for each request. It receives a values in the read loop.
	ch := make(chan *rpp.Message, 1)

	p.pendingLock.Lock()

	p.pending[id] = ch

	p.pendingLock.Unlock()
	p.writeLock.Lock()

	err = rpp.Write(p.stdin, req)
	if err == nil {
		err = p.stdin.Flush()
	}

	p.writeLock.Unlock()

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

		return res, nil
	case <-ctx.Done():
		slog.Debug("context canceled during plugin call")
		p.cleanPending(id, ch)

		return nil, ctx.Err()
	}
}

// cleanPending safely cleans up the given ID entry from the pending message
// channels and closes the channel.
func (p *Plugin) cleanPending(id rpp.ID, ch chan *rpp.Message) {
	p.pendingLock.Lock()
	delete(p.pending, id)
	p.pendingLock.Unlock()
	close(ch)
}

// closeAll closes all of the pending message channels.
func (p *Plugin) closeAll() {
	p.pendingLock.Lock()

	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}

	p.pendingLock.Unlock()
}

// handshake performs the RPP handshake with the plugin and sets the relevant
// received values to p. If the handshake fails, the function returns an error.
func (p *Plugin) handshake(ctx context.Context) error {
	params := rpp.DefaultHandshakeParams()

	res, err := p.call(ctx, rpp.MethodHandshake, params)
	if err != nil {
		return fmt.Errorf("method call %q to plugin %s failed: %w", rpp.MethodHandshake, p.name, err)
	}

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

	return nil
}

// read runs the reading loop for the plugin. It listens to the standard output
// of the plugins and handles the messages when they come in.
func (p *Plugin) read() {
	for {
		msg, err := rpp.Read(p.stdout)
		if err != nil {
			// Error when reading or EOF.
			slog.Warn("error when reading plugin output", "err", err)
			p.closeAll()

			return
		}

		// The message is a notification.
		// TODO: Handle the notifications.
		if msg.ID == 0 {
		}

		// Otherwise the message is handled as a response.
		p.pendingLock.Lock()

		ch, ok := p.pending[msg.ID]
		if ok {
			// Each request should accepts exactly one response, therefore the
			// entry from pending is deleted when the matching response is
			// received.
			delete(p.pending, msg.ID)
		}

		p.pendingLock.Unlock()

		if ok {
			ch <- msg
			close(ch)
		}
	}
}

// readStderr runs a loop for reading the standard error output of the plugin.
func (p *Plugin) readStderr() {
	for p.stderr.Scan() {
		line := p.stderr.Text()

		fmt.Fprintf(os.Stderr, "[%s] %s\n", p.name, line)
	}

	if err := p.stderr.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading the stderr for %q: %v", p.name, err)
	}
}

func (p *Plugin) start() error {
	slog.Debug("executing plugin", "path", p.cmd.Path)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("plugin execution from %s failed: %w", p.cmd.Path, err)
	}

	slog.Info("started a plugin process", "path", p.cmd.Path, "pid", p.cmd.Process.Pid)

	p.doneCh = make(chan error, 1)

	go p.read()
	go p.readStderr()

	go func() {
		p.doneCh <- p.cmd.Wait()

		close(p.doneCh)
	}()

	return nil
}
