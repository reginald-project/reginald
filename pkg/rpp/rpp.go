// Package rpp defines helpers for using the RPPv0 (Reginald plugin protocol
// version 0) in Go programs.
package rpp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Constant values related to the RPP version currently implemented by this
// package.
const (
	ContentType = "application/json-rpc" // default content type of messages
	Name        = "rpp"                  // protocol name to use in handshake
	Version     = 0                      // protocol version
)

// A Message is the Go representation of a message using RPP. It includes all of
// the possible fields for a message. Thus, the values that are not used for a
// particular type of a message are omitted and the fields must be validated by
// the client and the server.
type Message struct {
	JSONRCP string          `json:"jsonrpc"`          // version of the JSON-RCP protocol, must be "2.0"
	ID      int             `json:"id,omitempty"`     // identifier established by the client
	Method  string          `json:"method,omitempty"` // name of the method to be invoked
	Params  json.RawMessage `json:"params,omitempty"` // params of the method call as raw encoded JSON value
	Result  any             `json:"result,omitempty"` // result of the invoked method, present only on success
	Error   *Error          `json:"error,omitempty"`  // error trigger by the invoked method, not present on success
}

// An Error is the Go representation of a JSON-RCP error object using RPP.
type Error struct {
	Code    int    `json:"code"`           // code of the error, tells the error type
	Message string `json:"message"`        // error message
	Data    any    `json:"data,omitempty"` // optional additional information on the error
}

// HandshakeParams are the parameters that the client passes when calling the
// "handshake" method on the server.
type HandshakeParams struct {
	Protocol        string `json:"protocol"`        // name of the protocol, must be "rpp"
	ProtocolVersion int    `json:"protocolVersion"` // protocol version of the client, must be 0
}

// HandshakeResult is the result struct the server returns when the handshake
// method is successful.
type HandshakeResult struct {
	Protocol        string // name of the protocol, must be "rpp"
	ProtocolVersion int    // protocol version of the server, must be 0
	Kind            string // what the plugin provides, either "command" or "task"
	Name            string // name of the plugin, must match the name of provided command or task
}

// Read reads one message from r.
func Read(r *bufio.Reader) (*Message, error) {
	var l int

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read line: %w", err)
		}

		line = strings.TrimSuffix(line, "\r\n")
		if line == "" {
			break
		}

		// TODO: Disallow other headers.
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])

			if l, err = strconv.Atoi(v); err != nil {
				return nil, fmt.Errorf("bad Content-Length %q: %w", v, err)
			}
		}
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	fmt.Fprintln(os.Stderr, string(buf))

	var msg Message

	// TODO: Disallow unknown fields.
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message from JSON: %w", err)
	}

	fmt.Fprintln(os.Stderr, "msg body:", msg)

	return &msg, nil
}

// Write writes an RPP message to the given writer.
func Write(w io.Writer, msg Message) error {
	// TODO: Disallow unknown fields.
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal RPP message: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	data = append([]byte(header), data...)

	fmt.Fprintln(os.Stdout, "Writing message:", string(data))

	if _, err = w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}
