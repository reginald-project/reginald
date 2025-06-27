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

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/reginald-project/reginald-sdk-go/api"
)

// Errors returned by the server functions.
var (
	errInvalidLength = errors.New("number of bytes read does not match")
	errInvalidParams = errors.New("invalid params")
	errUnknownMethod = errors.New("unknown method")
	errZeroLength    = errors.New("Content-Length is zero")
)

// A Server is a Reginald plugin server implementation.
type Server struct {
	jsonRPCVersion string
	protocol       string
	ServerOpts
	commands        []*Command //nolint:unused // TODO: Implement commands.
	protocolVersion int
	exit            bool
	shutdown        bool
}

// ServerOpts are the options for creating a plugin server.
type ServerOpts struct {
	Name        string
	Version     string
	Domain      string
	Description string
	Help        string
	Executable  string
	Config      []api.ConfigEntry
}

// A Command is a command that can be run by the plugin.
type Command struct {
	Name        string
	Usage       string
	Description string
	Help        string
	Args        *api.Arguments
	Aliases     []string
	Config      []api.ConfigEntry
	Commands    []*Command
}

// NewServer creates a new plugin server. A plugin manifest can be generated
// automatically for plugins that use NewServer to create their plugn server.
func NewServer(opts *ServerOpts, impls ...any) *Server {
	var cmds []*Command

	for _, impl := range impls {
		switch v := impl.(type) {
		case *Command:
			cmds = append(cmds, v)
		case Command:
			cmds = append(cmds, &v)
		default:
			panic(fmt.Sprintf("invalid plugin functionality type: %[1]T (%[1]v)", impl))
		}
	}

	if opts.Name == "" {
		panic("creating plugin server with no name")
	}

	if opts.Executable == "" {
		panic("creating plugin server with no executable name")
	}

	return &Server{
		ServerOpts:      *opts,
		commands:        cmds,
		exit:            false,
		jsonRPCVersion:  api.JSONRPCVersion,
		protocol:        api.Protocol,
		protocolVersion: api.ProtocolVersion,
		shutdown:        false,
	}
}

// Run starts the plugin server and waits for requests from the client. It reads
// the requests from the standard input and sends the responses to the standard
// output. Run exits when the server is shut down first by calling the method
// "shutdown" and then sending the exit notification. The server reports normal
// errors by sending them as resposes to the client. If the server encounters
// some other error, it stops the run and returns the error.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)

	for !s.exit {
		req, err := read(reader)
		if err != nil {
			return err
		}

		if s.shutdown && req.Method != api.MethodExit {
			err = s.respondError(*req.ID, &api.Error{
				Code:    api.CodeMethodNotFound,
				Message: "method not found",
				Data:    fmt.Sprintf("method %q not available when shutting down", req.Method),
			})
			if err != nil {
				return fmt.Errorf("failed to send error response: %w", err)
			}
		}

		if req.ID != nil && !req.ID.Null {
			err = s.method(req)
			if err != nil {
				return err
			}

			continue
		}

		if err = s.notification(req); err != nil {
			// TODO: Should we send a log notification here?
			continue
		}
	}

	return nil
}

// method runs the method in the request and sends a response to it. If method
// returns an error, it means that the function itself has failed and that
// the server should notify the client about that. The server should not,
// however, send a response outside of method.
func (s *Server) method(req api.Request) error {
	if req.ID == nil || req.ID.Null {
		panic(fmt.Sprintf("method runner received request with a nil ID: %+v", req))
	}

	var methodFunc func(params json.RawMessage) (any, error)

	switch req.Method {
	case api.MethodHandshake:
		methodFunc = s.methodHandshake
	case api.MethodShutdown:
		methodFunc = s.methodShutdown
	default:
		err := s.respondError(*req.ID, &api.Error{
			Code:    api.CodeMethodNotFound,
			Message: "method not found",
			Data:    req.Method,
		})
		if err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}
	}

	result, err := methodFunc(req.Params)
	if err != nil {
		var rpcErr *api.Error
		if errors.As(err, &rpcErr) {
			err = s.respondError(*req.ID, rpcErr)
		}

		return err
	}

	err = s.respond(*req.ID, result)
	if err != nil {
		return err
	}

	return nil
}

// methodExit runs the "exit" method.
func (s *Server) methodExit(params json.RawMessage) error {
	if !isNull(params) {
		return fmt.Errorf("%w: %q expects null params", errInvalidParams, "exit")
	}

	s.exit = true

	return nil
}

// methodHandshake runs the "handshake" method.
func (s *Server) methodHandshake(params json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(params))
	d.DisallowUnknownFields()

	var handshakeParams api.HandshakeParams
	if err := d.Decode(&handshakeParams); err != nil {
		return nil, &api.Error{
			Code:    api.CodeInvalidParams,
			Message: "invalid params",
			Data:    fmt.Errorf("failed to unmarshal handshake params: %w", err),
		}
	}

	if handshakeParams.Protocol != s.protocol {
		return nil, &api.Error{
			Code:    api.CodeHandshakeError,
			Message: "handshake error",
			Data:    fmt.Sprintf("expected protocol to be %q, got %q", s.protocol, handshakeParams.Protocol),
		}
	}

	if handshakeParams.ProtocolVersion != s.protocolVersion {
		return nil, &api.Error{
			Code:    api.CodeHandshakeError,
			Message: "handshake error",
			Data: fmt.Sprintf(
				"expected protocol to be %d, got %d",
				s.protocolVersion,
				handshakeParams.ProtocolVersion,
			),
		}
	}

	return api.HandshakeResult{
		Name: s.Name,
		Handshake: api.Handshake{
			Protocol:        s.protocol,
			ProtocolVersion: s.protocolVersion,
		},
	}, nil
}

// methodShutdown runs the "shutdown" method.
func (s *Server) methodShutdown(params json.RawMessage) (any, error) {
	if !isNull(params) {
		return nil, &api.Error{
			Code:    api.CodeInvalidParams,
			Message: "invalid params",
			Data:    fmt.Sprintf("method \"shutdown\" requires no params, received: %+v", params),
		}
	}

	s.shutdown = true

	return true, nil
}

// notification runs the method in the notification request. No response is sent
// to the client.
func (s *Server) notification(req api.Request) error {
	var methodFunc func(params json.RawMessage) error

	switch req.Method {
	case api.MethodExit:
		methodFunc = s.methodExit
	default:
		return fmt.Errorf("%w: notification %q", errUnknownMethod, req.Method)
	}

	err := methodFunc(req.Params)
	if err != nil {
		return err
	}

	return nil
}

// respond sends successful response to the client.
func (s *Server) respond(id api.ID, result any) error {
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	err = write(api.Response{
		JSONRPC: s.jsonRPCVersion,
		ID:      id,
		Error:   nil,
		Result:  rawResult,
	})
	if err != nil {
		return err
	}

	return nil
}

// respondError sends an error response to the client.
func (s *Server) respondError(id api.ID, rpcErr *api.Error) error {
	err := write(api.Response{
		JSONRPC: s.jsonRPCVersion,
		ID:      id,
		Error:   rpcErr,
		Result:  nil,
	})
	if err != nil {
		return err
	}

	return nil
}

// isNull is a helper function for telling if the given raw JSON message is
// either omitted or equal to null. That is, the function can check if the given
// message field given as raw message is null.
func isNull(p json.RawMessage) bool {
	if len(p) == 0 {
		return true
	}

	return bytes.Equal(bytes.TrimSpace(p), []byte("null"))
}

// read reads a request sent to the plugin using the given reader.
func read(r *bufio.Reader) (api.Request, error) {
	var l int

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return api.Request{}, fmt.Errorf("failed to read line: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		// TODO: Consider disallowing other headers.
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])

			if l, err = strconv.Atoi(v); err != nil {
				return api.Request{}, fmt.Errorf("bad Content-Length %q: %w", v, err)
			}
		}
	}

	if l <= 0 {
		return api.Request{}, fmt.Errorf("bad Content-Length %d: %w", l, errZeroLength)
	}

	buf := make([]byte, l)
	if n, err := io.ReadFull(r, buf); err != nil {
		return api.Request{}, fmt.Errorf("failed to read RPC message: %w", err)
	} else if n != l {
		return api.Request{}, fmt.Errorf("failed to read RPC message: %w, want %d, got %d", errInvalidLength, l, n)
	}

	d := json.NewDecoder(bytes.NewReader(buf))
	d.DisallowUnknownFields()

	var req api.Request
	if err := d.Decode(&req); err != nil {
		return api.Request{}, fmt.Errorf("failed to decode message from JSON: %w", err)
	}

	return req, nil
}

// write writes a response to the standard output that will be sent to
// the client.
func write(res api.Response) error {
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err = os.Stdout.WriteString(header); err != nil {
		return fmt.Errorf("failed to write response header: %w", err)
	}

	if _, err = os.Stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write response data: %w", err)
	}

	return nil
}
