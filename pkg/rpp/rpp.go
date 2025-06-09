// Copyright 2025 Antti Kivi
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

// Package rpp defines helpers for using the RPPv0 (Reginald plugin protocol
// version 0) in Go programs.
package rpp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
)

// Constant values related to the RPP version currently implemented by this
// package.
const (
	ContentType    = "application/json-rpc" // default content type of messages
	JSONRCPVersion = "2.0"                  // JSON-RCP version the protocol uses
	Name           = "rpp"                  // protocol name to use in handshake
	Version        = 0                      // protocol version
)

// Error codes used for the protocol.
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// The different type values for config values and flags defined by the plugins.
const (
	ConfigBool   ConfigType = "bool"
	ConfigInt    ConfigType = "int"
	ConfigString ConfigType = "string"
)

// Errors returned by the RPP helper functions.
var (
	errInvalidConfig  = errors.New("invalid config value type")
	errInvalidFlagDef = errors.New("invalid flag definition")
	errConfigRead     = errors.New("reading config value failed")
	errZeroLength     = errors.New("content-length is zero")
)

// ConfigType is used as the type indicator of the fields that define the type
// of a config entry or a flag.
type ConfigType string

// A Message is the Go representation of a message using RPP. It includes all of
// the possible fields for a message. Thus, the values that are not used for a
// particular type of a message are omitted and the fields must be validated by
// the client and the server.
type Message struct {
	// JSONRCP is the JSON-RCP version used by the protocol and the message.
	// This must be exactly "2.0" or the client will reject the message.
	JSONRCP string `json:"jsonrpc"`

	// ID is the identifier established by the client and it must be included
	// for all messages except notification. It must be either an integer or
	// a string, and setting this field to nil is reserved for notifications and
	// special error cases as described by the protocols in use.
	ID any `json:"id,omitempty"`

	// Method is the name of the method to be invoked on the server. It must be
	// present in all of the requests and notifications that the client sends.
	Method string `json:"method,omitempty"`

	// Params are the parameters that are send with a method call. They are
	// encoded in this type as a raw JSON value as in the provided functions
	// the helpers used for reading the incoming messages handle the rest of
	// the fields and the plugin implementation must take care of the method
	// functionality according to the called method and the parameters.
	Params json.RawMessage `json:"params,omitempty"`

	// Result is the result of a method call as a raw JSON value. It must only
	// be present when the method call succeeded. Handling of the result is
	// similar to the handling of Params.
	Result json.RawMessage `json:"result,omitempty"`

	// Error is the error triggered by the invoked method, the error caused by
	// an invalid message etc. It must not be present on success. Handling of
	// the error is similar to the handling of Params.
	Error json.RawMessage `json:"error,omitempty"`
}

// An Error is the Go representation of a JSON-RCP error object using RPP.
type Error struct {
	// Data contains optional additional information about the error.
	Data any `json:"data,omitempty"`

	// Message is the error message.
	Message string `json:"message"`

	// Code is the error code that tells the error type. See the constant error
	// codes for the different supported values.
	Code int `json:"code"`
}

// CommandInfo contains information on a command that a plugin implements.
// CommandInfo is only used for discovering the plugin capabilities, and
// the actual command functionality is not implemented within this type.
type CommandInfo struct {
	// Name is the name of the command as it should be written by the user when
	// they run the command. It must not match any existing commands either
	// within Reginald or other plugins.
	Name string `json:"name"`

	// UsageLine is the one-line usage synopsis of the command.
	UsageLine string `json:"usage"`

	// Configs contains the config entries for this command.
	Configs []ConfigEntry `json:"configs,omitempty"`
}

// TaskInfo contains information on a task that a plugin implements. TaskInfo is
// only used for discovering the plugin capabilities, and the actual task
// functionality is not implemented within this type.
type TaskInfo struct {
	// Type is the name of the task type as it should be written by the user
	// when they specify it in, for example, their configuration. It must not
	// match any existing tasks either within Reginald or other plugins.
	Type string `json:"type"`

	// Configs contains the config entries for this task.
	Configs []ConfigEntry `json:"configs,omitempty"`
}

// A ConfigEntry is an entry in the config file that can also be set using
// an environment variable. As tasks are configured on a per-task basis,
// the config values in tasks cannot be set using environment variables.
// ConfigEntry can be in the plugin (under the plugin's name in the file), in
// a command (under the commands name in the file), or in a task (in a task
// entry in the file).
type ConfigEntry struct {
	// Key is the key of the ConfigEntry as it would be written in the config
	// file.
	Key string `json:"key"`

	// Value is the current value of the config entry as the type it should be
	// defined as. If this ConfigEntry is used in a result to a handshake, this
	// should be the default value of the ConfigEntry. When Reginald sends
	// the configuration data to the plugin at different steps after that, Value
	// contains the configured value of this ConfigEntry.
	Value any `json:"value"`

	// Type is a string representation of the type of the value that this config
	// entry holds. The possible values can be found in the protocol description
	// and in the constants of this package.
	Type ConfigType `json:"type"`

	// Flags contains the information on the possible command-line flag that is
	// associated with this ConfigEntry. Flag must be nil if the ConfigEntry has
	// no associated flag. Otherwise, its type must match [Flag]. If
	// the ConfigEntry is associated with a task, it must not have a flag.
	//
	// TODO: Make the difference between having an empty Flag object and having
	// a nil here clear.
	Flag any `json:"flag,omitempty"`

	// EnvName optionally defines a string to use in the environment variable
	// name instead of the automatic name of the variable that will be composed
	// using Key. It is appended after the prefix `REGINALD_` but if EnvName is
	// used to set the name of the environment variable, the name of the plugin
	// or the name of the command is not added to variable name automatically.
	EnvName string `json:"envOverride,omitempty"`

	// FlagOnly tells Reginald whether this config value should only be
	// controlled by a command-line flag. If this is set to true, Reginald won't
	// read the value of this config entry from the config file or from
	// environment variables.
	FlagOnly bool `json:"flagOnly,omitempty"`
}

// Flag is a field in [ConfigEntry] that describes the command-line flag
// associated with that ConfigValue. The default value and the value of Flag are
// given using ConfigValue.Value. Only a ConfigValue for a command or a plugin
// can have flags, and Reginald reports an error a flag is given for
// a ConfigValue for a task.
type Flag struct {
	// Name is the full name of the flag, used in the form of "--example". This
	// must be unique across Reginald and all of the flags currently in use by
	// the commands.
	//
	// If the name is omitted, the key for the config entry is used as the name
	// of the flag.
	Name string `json:"name,omitempty" mapstructure:"name,omitempty"`

	// Shorthand is the short one-letter name of the flag, used in the form of
	// "-e". This must be unique across Reginald and all of the flags currently
	// in use by the commands. The shorthand can be omitted if the flag
	// shouldn't have one.
	Shorthand string `json:"shorthand,omitempty" mapstructure:"shorthand,omitempty"`

	// Usage is the help description of this flag.
	Usage string `json:"usage,omitempty" mapstructure:"usage,omitempty"`

	// TODO: Add invert and remove IgnoreInConfig.
}

// NewConfigEntry creates a new ConfigValue and returns it. This function is
// primarily meant to be used outside of the handshake during the later method
// calls. It only assigns the Key, Value, and Type fields.
func NewConfigEntry(key string, value any) (ConfigEntry, error) {
	var t ConfigType

	switch value.(type) {
	case bool:
		t = ConfigBool
	case int, int64:
		t = ConfigInt
	case string:
		t = ConfigString
	default:
		return ConfigEntry{}, fmt.Errorf("%w: %[2]v (%[2]T) for %s", errInvalidConfig, value, key)
	}

	cfg := ConfigEntry{ //nolint:exhaustruct // rest are up to the caller
		Key:   key,
		Value: value,
		Type:  t,
	}

	return cfg, nil
}

// Int returns value of c as an int.
func (c *ConfigEntry) Int() (int, error) {
	if c.Type != ConfigInt {
		return 0, fmt.Errorf("%w: %q is not an int", errConfigRead, c.Key)
	}

	switch v := c.Value.(type) {
	case int:
		return v, nil
	case int64:
		// TODO: Might be unsafe.
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%w: invalid type %T", errConfigRead, v)
	}
}

// RealFlag resolves the real type for ConfigValue.Flag and returns a pointer to
// the Flag if it is set. If the flag is not set, it returns nil. As the flag
// might be decoded into a map when it is passed using JSON-RCP, RealFlag
// further decodes the map into Flag.
//
// As the Flag may inherit its name from ConfigValue, the function also sets
// the correct name for the Flag.
func (c *ConfigEntry) RealFlag() (*Flag, error) {
	switch v := c.Flag.(type) {
	case nil:
		return nil, nil //nolint:nilnil // TODO: See if sentinel error should be used.
	case Flag:
		f := v
		if f.Name == "" {
			// TODO: Make sure that the key is correctly formatted.
			f.Name = c.Key
		}

		return &f, nil
	case map[string]any:
		var flag Flag

		dc := &mapstructure.DecoderConfig{ //nolint:exhaustruct // use default values
			DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
			Result:     &flag,
		}

		d, err := mapstructure.NewDecoder(dc)
		if err != nil {
			return nil, fmt.Errorf("failed to create mapstructure decoder: %w", err)
		}

		if err := d.Decode(v); err != nil {
			return nil, fmt.Errorf("failed to decode flag: %w", err)
		}

		if flag.Name == "" {
			// TODO: Make sure that the key is correctly formatted.
			flag.Name = c.Key
		}

		return &flag, nil
	default:
		return nil, fmt.Errorf("%w: %T", errInvalidFlagDef, c.Flag)
	}
}

// Error returns the string representation of the error e.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("%s (%v)", e.Message, e.Data)
	}

	return e.Message
}

// Read reads one message from r.
func Read(r *bufio.Reader) (*Message, error) {
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

		// TODO: Disallow other headers.
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[strings.IndexByte(line, ':')+1:])

			if l, err = strconv.Atoi(v); err != nil {
				return nil, fmt.Errorf("bad Content-Length %q: %w", v, err)
			}
		}
	}

	if l <= 0 {
		return nil, fmt.Errorf("%w", errZeroLength)
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	var msg Message

	// TODO: Disallow unknown fields.
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, fmt.Errorf("failed to decode message from JSON: %w", err)
	}

	return &msg, nil
}

// Write writes an RPP message to the given writer.
func Write(w io.Writer, msg *Message) error {
	// TODO: Disallow unknown fields.
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal RPP message: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err = w.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write message header: %w", err)
	}

	if _, err = w.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// LogValue implements [slog.LogValuer] for message. It returns a group
// containing the fields of the Message, so that they appear together in the log
// output.
func (m *Message) LogValue() slog.Value {
	var attrs []slog.Attr

	attrs = append(attrs, slog.String("jsonrcp", m.JSONRCP))

	if m.ID != nil {
		attrs = append(attrs, slog.Attr{Key: "id", Value: IDLogValue(m.ID)})
	}

	attrs = append(attrs, slog.String("method", m.Method))

	if m.Params != nil {
		attrs = append(attrs, slog.String("params", string(m.Params)))
	}

	if m.Result != nil {
		attrs = append(attrs, slog.String("result", string(m.Result)))
	}

	if m.Error != nil {
		attrs = append(attrs, slog.String("error", string(m.Error)))
	}

	return slog.GroupValue(attrs...)
}

// IDLogValue return the [slog.Value] for the given message ID.
func IDLogValue(id any) slog.Value {
	// TODO: Find a safer way to convert the number types.
	switch v := id.(type) {
	case float64:
		u := int64(v)
		if float64(u) != v {
			return slog.StringValue(fmt.Sprintf("invalid ID type %T", v))
		}

		return slog.Int64Value(u)
	case string:
		return slog.StringValue(v)
	case *int:
		return slog.IntValue(*v)
	case *int64:
		return slog.Int64Value(*v)
	case *float64:
		u := int64(*v)
		if float64(u) != *v {
			return slog.StringValue(fmt.Sprintf("invalid ID type %T", v))
		}

		return slog.Int64Value(u)
	case *string:
		return slog.StringValue(*v)
	default:
		return slog.StringValue(fmt.Sprintf("invalid ID type %T", v))
	}
}
