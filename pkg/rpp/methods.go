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

package rpp

import "github.com/reginald-project/reginald/pkg/logs"

// Standard method names used by the RPP.
const (
	MethodExit         = "exit"
	MethodHandshake    = "handshake"
	MethodInitialize   = "initialize"
	MethodLog          = "log"
	MethodRunCommand   = "runCommand"
	MethodRunTask      = "runTask"
	MethodSetupCommand = "setupCommand"
	MethodValidateTask = "validateTask"
	MethodShutdown     = "shutdown"
)

// Handshake is a helper type that contains the handshake information fields
// that are shared between the "handshake" method parameters and the response.
// These values must match in order to perform the handshake successfully.
// The valid values for the current implementation are provided as constants in
// this package.
type Handshake struct {
	// Protocol is the identifier of the protocol to use. It must be "rpp" for
	// the handshake to succeed.
	Protocol string `json:"protocol"`

	// ProtocolVersion is the version of the protocol to use. It must be 0 for
	// the handshake to succeed.
	ProtocolVersion int `json:"protocolVersion"`
}

// HandshakeParams are the parameters that the client passes when calling the
// "handshake" method on the server.
type HandshakeParams struct {
	Handshake
}

// HandshakeResult is the result struct the server returns when the handshake
// method is successful.
type HandshakeResult struct {
	Handshake

	// Name is the user-friendly name of the plugin that will be used in
	// the logs and in the user output. It must be unique and loading
	// the plugins will fail if two or more plugins have exactly the same name.
	// It must also be a valid config key if the plugin registers plugin-wide
	// config entries.
	Name string `json:"name"`

	// Version is the version of the plugin. It should be a valid version number
	// according to the semantic versioning 2.0.0 specification.
	Version string `json:"version"`

	// PluginConfigs contains the plugin-level config entries. If the name of
	// the plugin and the name of a command are the same, PluginConfigs takes
	// precedence over the configs defined by the command.
	PluginConfigs []ConfigEntry `json:"configs,omitempty"`

	// Commands contains the information on the command types this plugin
	// offers. If the plugin does not provide any commands, this can be either
	// nil or an empty list.
	Commands []CommandInfo `json:"commands,omitempty"`

	// Tasks contains the information on the task types this plugin offers. It
	// is a list of the provided task types. If the plugin does not provide any
	// tasks, this can be either nil or an empty list.
	Tasks []TaskInfo `json:"tasks,omitempty"`
}

// InitializeParams is the parameter type for the "initialize" method.
// The initialization is done after the handshake has succeeded, and it passes
// general configuration information as well as the plugin configuration.
// The plugin should validate its plugin-wide configuration and return an error
// if the configuration is invalid.
type InitializeParams struct {
	// Config contains the values of the plugin-wide configuration with
	// the values set from the configuration sources.
	Config []ConfigEntry `json:"config,omitempty"`

	// Logging contains the logging configuration by Reginald. The plugin should
	// aim to honor these settings in order to avoid sending log messages that
	// are not allowed by the configuration. Messages that are not allowed are
	// discarded by Reginald anyway.
	Logging LoggingConfig `json:"logging"`
}

// LoggingConfig is the logging configuration passed in to plugins during
// the initialize method call. It tells the client's settings for logging so
// that the plugin can adapt and not send unnecessary log messages.
type LoggingConfig struct {
	// Enabled tells whether logging is enabled at all.
	Enabled bool `json:"enabled"`

	// Level gives the selected logging level. For example, if the level is
	// set to [logs.LevelDebug], messages with the level "debug" and higher
	// are allowed.
	Level logs.Level `json:"level"`
}

// LogParams are the parameters passed with the "log" method. Reginald uses
// structured logging where the given message is one field of the log output and
// additional information can be given as Fields.
type LogParams struct {
	// Fields contains additional fields that should be included with the
	// message. Reginald automatically adds information about the plugin from
	// which the message came from.
	Fields map[string]any `json:"fields,omitempty"`

	// Message is the logging message.
	Message string `json:"msg"`

	// Level is the logging level of the message. It should have a string value
	// "trace", "debug", "info", "warn", or "error".
	Level logs.Level `json:"level"`
}

// RunCmdParams are the parameters passed when the client runs a command from
// a plugin.
type RunCmdParams struct {
	// Name is the name of the command that should be run.
	Name string `json:"name"`
}

// RunTaskParams are the parameters passed when the client runs a task from
// a plugin.
type RunTaskParams struct {
	// Type is the name of task type that is run.
	Type string `json:"type"`

	// Dir is the base directory of the program run.
	Dir string `json:"dir"`

	// Config contains the configuration options set for this task.
	Config []KeyValue `json:"config,omitempty"`
}

// SetupCmdParams are the parameters passed when the client runs a command setup
// from a plugin.
type SetupCmdParams struct {
	// Name is the name of the command that should be set up.
	Name string `json:"name"`

	// Args are the command-line arguments after parsing the commands and flags.
	// It should contain the positional arguments required by the command.
	Args []string `json:"args"`

	// Config contains the config values of the command with the values set from
	// the configuration sources.
	Config []ConfigEntry `json:"config,omitempty"`
}

// ValidateTaskParams are the parameters for calling the "validateTask" method
// in a plugin. The method checks if the given config values for a task are
// valid. The same method is run for both the default values and the actual
// parsed task configs.
type ValidateTaskParams struct {
	// Type is the name of task type for which the config is being validated.
	Type string `json:"type"`

	// Config contains the configuration options set for this task. These do not
	// contain the "type", "id", or "depends-on" fields as they are validated
	// internally by Reginald.
	Config []KeyValue `json:"config,omitempty"`
}

// DefaultHandshakeParams returns the default parameters used by the client in
// the handshake method call.
func DefaultHandshakeParams() HandshakeParams {
	return HandshakeParams{
		Handshake: Handshake{
			Protocol:        Name,
			ProtocolVersion: Version,
		},
	}
}
