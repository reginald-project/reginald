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
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log"
)

// errRestart is returned if the program tries to start a plugin again.
var errRestart = errors.New("plugin already running")

// A Plugin is a plugin that Reginald recognizes.
type Plugin interface {
	// Call calls a method in the plugin. It unmarshals the result into result
	// if the method call is successful. Otherwise, it returns any error that
	// occured or was returned in response.
	Call(ctx context.Context, method string, params any, result any) error

	// Manifest returns the loaded manifest for the plugin.
	Manifest() *api.Manifest

	// Start starts the execution of the plugin process.
	Start(ctx context.Context) error
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

	// loaded tells whether the executable for this plugin is loaded and started
	// up.
	loaded bool
}

// Manifest returns the loaded manifest for the plugin.
func (b *builtinPlugin) Manifest() *api.Manifest {
	return b.manifest
}

// Call calls a method in the plugin. It unmarshals the result into result if
// the method call is successful. Otherwise, it returns any error that occured
// or was returned in response.
func (b *builtinPlugin) Call(ctx context.Context, method string, params any, result any) error {
	return nil
}

// Start starts the execution of the plugin process.
func (b *builtinPlugin) Start(ctx context.Context) error {
	log.Trace(ctx, "starting built-in plugin", "no-op", true, "plugin", b.manifest.Domain)

	return nil
}

// Close closes the standard streams attached to the connection.
func (c *connection) Close() error {
	return nil
}

// Read reads up to len(p) bytes into p from the standard output attached to
// the connection.
func (c *connection) Read(p []byte) (int, error) {
	return c.stdout.Read(p)
}

// Write writes len(p) bytes from p to the standard input attached to
// the connection.
func (c *connection) Write(p []byte) (int, error) {
	return c.stdin.Write(p)
}

// Call calls a method in the plugin. It unmarshals the result into result if
// the method call is successful. Otherwise, it returns any error that occured
// or was returned in response.
func (e *externalPlugin) Call(ctx context.Context, method string, params any, result any) error {
	return nil
}

// Manifest returns the loaded manifest for the plugin.
func (e *externalPlugin) Manifest() *api.Manifest {
	return e.manifest
}

// Start starts the execution of the plugin process.
func (e *externalPlugin) Start(ctx context.Context) error {
	m := e.manifest

	if e.loaded {
		return fmt.Errorf("failed to start %q: %w", m.Name, errRestart)
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
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
	e.conn = conn
	e.cmd = c

	if err = e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %q (%s): %w", m.Name, e.cmd.Path, err)
	}

	// TODO: Add read loops.

	return nil
}

// newBuiltin returns a new built-in plugin for the given manifest.
func newBuiltin(m *api.Manifest) *builtinPlugin {
	return &builtinPlugin{
		manifest: m,
	}
}

// newExternal returns a new external plugin for the given manifest.
func newExternal(m *api.Manifest) *externalPlugin {
	return &externalPlugin{
		manifest: m,
		conn:     nil,
		loaded:   false,
	}
}
