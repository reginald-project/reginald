// Package plugins implements an RPP client in Reginald to run plugins.
package plugins

import (
	"bufio"
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
	in          *bufio.Writer                // stdin pipe of the plugin command
	out         *bufio.Reader                // buffered reader for the stdout of the plugin
	writeLock   sync.Mutex                   // lock for writing to writer to serialize the messages
	pending     map[rpp.ID]chan *rpp.Message // read messages from the plugin waiting for processing
	pendingLock sync.Mutex                   // lock used with pending
}

// New returns a pointer to a newly created Plugin.
func New(path string) (*Plugin, error) {
	if ok, err := pathname.IsFile(path); err != nil {
		return nil, fmt.Errorf("failed to check if %s is a file: %w", path, err)
	} else if !ok {
		return nil, fmt.Errorf("%w: %s", errNotFile, path)
	}

	c := exec.Command(filepath.Clean(path))

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

	// TODO: Capture the standard error output.
	c.Stderr = os.Stderr
	p := &Plugin{
		name: filepath.Base(
			path,
		), // the base name of the plugin is used until the real name is received
		kind:        "",
		flags:       nil,
		cmd:         c,
		nextID:      atomic.Uint64{},
		in:          bufio.NewWriter(stdin),
		out:         bufio.NewReader(stdout),
		writeLock:   sync.Mutex{},
		pending:     make(map[rpp.ID]chan *rpp.Message),
		pendingLock: sync.Mutex{},
	}

	return p, nil
}

// Load creates the processes for the plugins, performs the handshakes with
// them, returns a slice of the valid plugins.
func Load(files []string) ([]*Plugin, error) {
	// TODO: Provide the config value for the create function.
	plugins, err := loadAll(files, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load the plugins: %w", err)
	}

	return plugins, nil
}

// loadAll creates and starts all of the plugin processes and performs the
// handshake with them. If ignoreErrors is true, the function simply drops
// plugins that cause errors when starting or fail the handshake. Otherwise the
// function fails fast.
func loadAll(files []string, ignoreErrors bool) ([]*Plugin, error) {
	var (
		lock    sync.Mutex
		plugins []*Plugin
	)

	// TODO: Add a context for cancellation.
	eg := errgroup.Group{}

	// TODO: Print the errors to actual output if they are ignored.
	for _, f := range files {
		eg.Go(func() error {
			p, err := New(f)
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

			if err := p.handshake(); err != nil {
				if ignoreErrors {
					slog.Warn("handshake failed", "path", f, "err", err)

					return nil
				}

				return fmt.Errorf("handshake for plugin %s failed: %w", p.name, err)
			}

			// TODO: Should I use a context to be able to cancel?

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
func (p *Plugin) call(method string, params any) (*rpp.Message, error) {
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

	err = rpp.Write(p.in, req)
	if err == nil {
		err = p.in.Flush()
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

	res, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("%w: %s (method %s)", errNoResponse, p.name, method)
	}

	return res, nil

	// TODO: Use contexts for better control.
	// select {
	// case res, ok := <-ch:
	// 	if !ok {
	// 		return nil, fmt.Errorf("%w: %s (method %s)", errNoResponse, p.name, method)
	// 	}
	// 	return res, nil
	// case <-ctx.Done():
	// 	p.cleanPending(id, ch)
	// 	return nil, ctx.Err()
	// }
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
func (p *Plugin) handshake() error {
	params := rpp.DefaultHandshakeParams()

	res, err := p.call(rpp.Handshake, params)
	if err != nil {
		return fmt.Errorf("method call %q to plugin %s failed: %w", rpp.Handshake, p.name, err)
	}

	var result rpp.HandshakeResult
	if err = json.Unmarshal(res.Result, &result); err != nil {
		return fmt.Errorf(
			"failed to unmarshal result for the %q method call to %s: %w",
			rpp.Handshake,
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
		msg, err := rpp.Read(p.out)
		if err != nil {
			// Error when reading or EOF.
			// TODO: Print the error or something.
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

func (p *Plugin) start() error {
	slog.Debug("executing plugin", "path", p.cmd.Path)

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("plugin execution from %s failed: %w", p.cmd.Path, err)
	}

	slog.Info("started a plugin process", "path", p.cmd.Path, "pid", p.cmd.Process.Pid)

	go p.read()

	return nil
}
