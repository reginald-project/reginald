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

// Package plugin implements the plugin client of Reginald.
package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log"
	"github.com/reginald-project/reginald/internal/panichandler"
	"github.com/reginald-project/reginald/internal/terminal"
)

// A Plugin is a plugin that Reginald recognizes.
type Plugin interface {
	// External reports whether the plugin is not built-in.
	External() bool

	// Manifest returns the loaded manifest for the plugin.
	Manifest() *api.Manifest

	// call calls a method in the plugin. It unmarshals the result into result
	// if the method call is successful. Otherwise, it returns any error that
	// occurred or was returned in response.
	call(ctx context.Context, method string, params, result any) error

	// notify sends a notification request to the plugin.
	notify(ctx context.Context, method string, params any) error

	// start starts the execution of the plugin process.
	start(ctx context.Context) error
}

// A builtinPlugin is a built-in plugin provided by Reginald. It is implemented
// within the program and it must not use an external executable.
type builtinPlugin struct {
	// manifest is the manifest for this plugin.
	manifest *api.Manifest
}

// A connection handles the connection with the plugin client and the external
// plugin executable for an [externalPlugin].
type connection struct {
	stdin  io.WriteCloser // stdin of the process
	stdout io.ReadCloser  // stdout of the process
	stderr io.ReadCloser  // stderr of the process
	mu     sync.Mutex     // serializes writing
}

// An externalPlugin is an externalPlugin plugin that is not provided by
// the program itself. It implements the plugin client in Reginald for calling
// methods from the plugin executables.
type externalPlugin struct {
	// manifest is the manifest for this plugin.
	manifest *api.Manifest

	// conn holds the connection to cmd via the standard streams.
	conn io.ReadWriteCloser

	// cmd is the underlying command running the plugin process.
	cmd *exec.Cmd

	// queue transfers the responses from the read loop to the call function.
	queue *responseQueue

	// doneCh is closed when the plugin is done running.
	doneCh chan error

	// lastID is the ID that was last used in a method call. Even though
	// the protocol supports both strings and ints as the ID, we just default to
	// ints to make the client more reasonable.
	lastID atomic.Int64
}

// A responseQueue holds channels that transfer responses sent from the plugins
// and read by the plugin's reading loop to the plugin's call function. While
// not technically a queue, the name feels natural.
type responseQueue struct {
	// q holds channels waiting for responses from the plugins.
	q map[string]chan api.Response

	// mu locks the queue.
	mu sync.Mutex
}

// An rpcMessage is a helper that decodes an incoming message before the read
// loop determines its type.
type rpcMessage struct {
	JSONRCP string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	ID      *api.ID         `json:"id,omitempty"`
	Error   *api.Error      `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

// External reports whether the plugin is not built-in.
func (*builtinPlugin) External() bool {
	return false
}

// Manifest returns the loaded manifest for the plugin.
func (b *builtinPlugin) Manifest() *api.Manifest {
	return b.manifest
}

// Close closes the standard streams attached to the connection.
func (c *connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.stderr.Close(); err != nil {
		return fmt.Errorf("failed to close connection stderr: %w", err)
	}

	if err := c.stdin.Close(); err != nil {
		return fmt.Errorf("failed to close connection stdin: %w", err)
	}

	if err := c.stdout.Close(); err != nil {
		return fmt.Errorf("failed to close connection stdout: %w", err)
	}

	return nil
}

// Read reads up to len(p) bytes into p from the standard output attached to
// the connection.
func (c *connection) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err != nil {
		return n, fmt.Errorf("read from connection failed: %w", err)
	}

	return n, nil
}

// Write writes len(p) bytes from p to the standard input attached to
// the connection.
func (c *connection) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, err := c.stdin.Write(p)
	if err != nil {
		return n, fmt.Errorf("write to connection failed: %w", err)
	}

	return n, nil
}

// External reports whether the plugin is not built-in.
func (*externalPlugin) External() bool {
	return true
}

// Manifest returns the loaded manifest for the plugin.
func (e *externalPlugin) Manifest() *api.Manifest {
	return e.manifest
}

// call calls a method in the plugin. It unmarshals the result into result if
// the method call is successful. Otherwise, it returns any error that occurred
// or was returned in response.
func (b *builtinPlugin) call(ctx context.Context, method string, _, result any) error {
	log.Trace(ctx, "built-in method call", "plugin", b.manifest.Name, "method", method)

	switch method {
	case api.MethodHandshake:
		handshakeResult, ok := result.(*api.HandshakeResult)
		if !ok {
			panic(fmt.Sprintf("invalid result type for method %q: %[2]T (%[2]v)", method, result))
		}

		*handshakeResult = api.HandshakeResult{
			Name: b.manifest.Name,
			Handshake: api.Handshake{
				Protocol:        "reginald",
				ProtocolVersion: 0,
			},
		}
	default:
		panic("invalid method call: " + method)
	}

	return nil
}

// notify sends a notification request to the plugin.
func (b *builtinPlugin) notify(ctx context.Context, method string, _ any) error {
	log.Trace(ctx, "built-in notfication", "plugin", b.manifest.Name, "method", method)

	return nil
}

// start starts the execution of the plugin process.
func (b *builtinPlugin) start(ctx context.Context) error {
	log.Trace(ctx, "starting built-in plugin", "no-op", true, "plugin", b.manifest.Domain)

	return nil
}

// call calls a method in the plugin. It unmarshals the result into result if
// the method call is successful. Otherwise, it returns any error that occurred
// or was returned in response.
func (e *externalPlugin) call(ctx context.Context, method string, params, result any) error {
	id := e.lastID.Add(1) //nolint:varnamelen

	rpcID, err := api.NewID(id)
	if err != nil {
		return fmt.Errorf("failed to create ID: %w", err)
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	req := api.Request{
		JSONRPC: api.JSONRPCVersion,
		ID:      rpcID,
		Method:  method,
		Params:  rawParams,
	}

	log.Trace(
		ctx,
		"calling method",
		"plugin",
		e.manifest.Name,
		"method",
		method,
		"id",
		id,
		"rpcId",
		*rpcID,
		"params",
		params,
	)
	e.queue.add(rpcID)
	defer e.queue.close(rpcID)

	err = write(ctx, e.conn, req)
	if err != nil {
		return err
	}

	select {
	case res, ok := <-e.queue.channel(rpcID):
		if !ok {
			return fmt.Errorf("%w: plugin %q (method %q)", errNoResponse, e.manifest.Name, method)
		}

		log.Trace(ctx, "received response", "plugin", e.manifest.Name, "res", res)

		if res.Error != nil {
			return fmt.Errorf("plugin returned an error: %w", res.Error)
		}

		if err := json.Unmarshal(res.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}

		log.Trace(ctx, "method call successful", "plugin", e.manifest.Name, "method", method, "id", id)
	case <-ctx.Done():
		return fmt.Errorf("method call halted: %w", ctx.Err())
	}

	return nil
}

// kill kills the plugin process.
func (e *externalPlugin) kill(ctx context.Context) error {
	if e.cmd.Process != nil {
		log.Warn(ctx, "killing plugin process", "plugin", e.manifest.Name)

		if err := e.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process for plugin %q: %w", e.manifest.Name, err)
		}
	}

	e.queue.closeAll()

	if err := e.conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection to plugin %q: %w", e.manifest.Name, err)
	}

	return nil
}

// notification handles a notification request sent from the plugin.
func (e *externalPlugin) notification(ctx context.Context, req api.Request) error {
	switch req.Method {
	case api.MethodLog:
		var params api.LogParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return fmt.Errorf("failed to unmarshal log params: %w", err)
		}

		return handleLog(ctx, e, &params)
	default:
		return fmt.Errorf("%w: %s", errUnknownMethod, req.Method)
	}
}

// notify sends a notification request to the plugin.
func (e *externalPlugin) notify(ctx context.Context, method string, params any) error {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	req := api.Request{
		JSONRPC: api.JSONRPCVersion,
		ID:      nil,
		Method:  method,
		Params:  rawParams,
	}

	log.Trace(ctx, "sending notification", "plugin", e.manifest.Name, "method", method, "params", params)

	return write(ctx, e.conn, req)
}

// read runs the reading loop of the plugin. It listens to the connection with
// the plugin process for data through the standard output pipe and passes
// the messages either to the method callers or to the notification handler.
func (e *externalPlugin) read(ctx context.Context, handlePanic func()) {
	defer handlePanic()
	defer e.queue.closeAll()

	reader := bufio.NewReader(e.conn)
	done := false

	go func() {
		<-ctx.Done()

		done = true
	}()

	for !done {
		msg, err := read(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			log.Error(ctx, "error when reading from plugin", "plugin", e.manifest.Name, "err", err)

			return
		}

		if msg.JSONRCP != api.JSONRPCVersion {
			log.Error(
				ctx,
				"invalid JSON-RPC version",
				"plugin",
				e.manifest.Name,
				"want",
				api.JSONRPCVersion,
				"got",
				msg.JSONRCP,
			)

			return
		}

		if msg.ID == nil || msg.ID.Null {
			switch {
			case msg.Method == "":
				log.Error(ctx, "no method in notification", "plugin", e.manifest.Name, "msg", msg)

				return
			case msg.Error != nil:
				log.Error(ctx, "error in notification", "plugin", e.manifest.Name, "msg", msg)

				return
			case len(msg.Result) > 0:
				log.Error(ctx, "result in notification", "plugin", e.manifest.Name, "msg", msg)

				return
			}

			req := api.Request{
				JSONRPC: msg.JSONRCP,
				ID:      nil,
				Method:  msg.Method,
				Params:  msg.Params,
			}

			log.Trace(ctx, "received notification", "plugin", e.manifest.Name, "req", req)

			if err := e.notification(ctx, req); err != nil {
				log.Error(ctx, "error when handling notification from plugin", "plugin", e.manifest.Name, "err", err)

				return
			}

			continue
		}

		switch {
		case msg.Method != "":
			log.Error(ctx, "method in response", "plugin", e.manifest.Name, "msg", msg)

			return
		case msg.Params != nil:
			log.Error(ctx, "params in response", "plugin", e.manifest.Name, "msg", msg)

			return
		}

		ch := e.queue.channel(msg.ID)
		if ch == nil {
			log.Error(ctx, "response with ID that is not waiting", "plugin", e.manifest.Name, "msg", msg)

			return
		}

		res := api.Response{
			JSONRPC: msg.JSONRCP,
			ID:      *msg.ID,
			Error:   msg.Error,
			Result:  msg.Result,
		}

		ch <- res
	}
}

// readStderr runs the standard error stream reading loop of the plugin. It
// listens to the connection with the plugin process for data through
// the standard error pipe and handles the messages.
func (e *externalPlugin) readStderr(ctx context.Context, handlePanic func()) {
	defer handlePanic()

	conn, ok := e.conn.(*connection)
	if !ok {
		panic(fmt.Sprintf("connection for plugin %q is not *connection", e.manifest.Name))
	}

	scanner := bufio.NewScanner(conn.stderr)

	for scanner.Scan() {
		line := scanner.Text()

		log.Warn(ctx, "plugin printed to stderr", "plugin", e.manifest.Name, "output", line)
		terminal.Errorf("[%s] %s\n", e.manifest.Name, line)
	}

	if err := scanner.Err(); err != nil {
		log.Error(ctx, "error reading plugin stderr", "plugin", e.manifest.Name, "err", err)
	}
}

// start starts the execution of the plugin process.
func (e *externalPlugin) start(ctx context.Context) error {
	m := e.manifest

	if e.cmd != nil {
		panic(fmt.Sprintf("trying to restart process for plugin %q", e.manifest.Name))
	}

	exe := fspath.Path(m.Executable)

	if ok, err := exe.IsFile(); err != nil {
		return fmt.Errorf("failed to check if executable for %q is a file: %w", m.Name, err)
	} else if !ok {
		panic(fmt.Sprintf("executable for plugin %q at %s is not file", m.Name, exe))
	}

	// TODO: Add the mode for executing only trusted plugins.
	c := exec.CommandContext(ctx, string(exe.Clean())) // #nosec G204 -- sanitized earlier

	stdin, err := c.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for %s: %w", exe, err)
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		return fmt.Errorf(
			"failed to create stdout pipe for %s: %w", exe, err)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe for %s: %w", exe, err)
	}

	conn := &connection{
		mu:     sync.Mutex{},
		stderr: stderr,
		stdin:  stdin,
		stdout: stdout,
	}
	e.conn = conn
	e.cmd = c

	if err = e.cmd.Start(); err != nil {
		return fmt.Errorf("execution of %q (%s) failed: %w", m.Name, e.cmd.Path, err)
	}

	handlePanic := panichandler.WithStackTrace()

	go e.read(ctx, handlePanic)
	go e.readStderr(ctx, handlePanic)

	go func() {
		defer handlePanic()
		e.doneCh <- e.cmd.Wait()
		close(e.doneCh)
	}()

	return nil
}

func (q *responseQueue) add(id *api.ID) {
	if q.q == nil {
		panic("adding to nil responseQueue")
	}

	ch := make(chan api.Response, 1)

	q.mu.Lock()
	defer q.mu.Unlock()

	q.q[idToKey(id)] = ch
}

func (q *responseQueue) channel(id *api.ID) chan api.Response {
	key := idToKey(id)

	q.mu.Lock()
	defer q.mu.Unlock()

	ch, ok := q.q[key]
	if !ok {
		return nil
	}

	return ch
}

// close closes the channel matching the given ID and deletes the entry from
// the queue.
func (q *responseQueue) close(id *api.ID) {
	if q.q == nil {
		panic("closing in nil responseQueue")
	}

	key := idToKey(id)

	q.mu.Lock()
	defer q.mu.Unlock()

	ch, ok := q.q[key]
	if !ok {
		return
	}

	close(ch)
	delete(q.q, key)
}

// closeAll closes all of the channels from the queue.
func (q *responseQueue) closeAll() {
	if q.q == nil {
		panic("closing nil responseQueue")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	for id, ch := range q.q {
		close(ch)
		delete(q.q, id)
	}
}

// idToKey is a helper function that converts id into a string that can be used
// as a key in pending channels map of the plugin client.
func idToKey(id *api.ID) string {
	b, err := json.Marshal(id)
	if err != nil {
		panic(fmt.Sprintf("failed to convert ID %v to JSON-encoded value: %v", id, err))
	}

	return string(b)
}

// read reads a message from the plugin using the given reader.
func read(r *bufio.Reader) (*rpcMessage, error) {
	var l int

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read line: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		// TODO: Consider disallowing other headers.
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])

			if l, err = strconv.Atoi(v); err != nil {
				return nil, fmt.Errorf("bad Content-Length %q: %w", v, err)
			}
		}
	}

	if l <= 0 {
		return nil, fmt.Errorf("bad Content-Length %d: %w", l, errZeroLength)
	}

	buf := make([]byte, l)
	if n, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read RPC message: %w", err)
	} else if n != l {
		return nil, fmt.Errorf("failed to read RPC message: %w, want %d, got %d", errInvalidLength, l, n)
	}

	d := json.NewDecoder(bytes.NewReader(buf))
	d.DisallowUnknownFields()

	var msg *rpcMessage
	if err := d.Decode(&msg); err != nil {
		return nil, fmt.Errorf("failed to decode message from JSON: %w", err)
	}

	return msg, nil
}

func write(ctx context.Context, w io.Writer, req api.Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Trace(ctx, "writing data", "data", string(data))

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err = w.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write request header: %w", err)
	}

	if _, err = w.Write(data); err != nil {
		return fmt.Errorf("failed to write request data: %w", err)
	}

	return nil
}
