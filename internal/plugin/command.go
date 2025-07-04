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

package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/reginald-project/reginald-sdk-go/api"
)

// A Command is the program representation of a plugin command that is defined
// in the manifest.
type Command struct {
	// Plugin is the plugin that this command is defined in.
	Plugin Plugin
	*api.Command

	// parent is the parent command of this command. It is nil for the root
	// commands.
	Parent *Command

	// Commands is a list of subcommands that this command provides.
	Commands []*Command
}

// logCmds is a helper type for logging a slice of commands.
type logCmds []*Command

// LogValue implements [slog.LogValuer] for Command. It returns a group value
// for logging a Command.
func (c *Command) LogValue() slog.Value {
	if c == nil {
		return slog.StringValue("<nil>")
	}

	return slog.GroupValue(
		slog.String("name", c.Name),
		slog.String("usage", c.Usage),
		slog.String("description", c.Description),
		slog.Any("aliases", c.Aliases),
		slog.Any("commands", logCmds(c.Commands)),
	)
}

// Names returns the full qualified name of the command as slice. The return
// value contains the names of all of the parents of the command and the command
// name.
func (c *Command) Names() []string {
	if c == nil {
		panic("calling Names on nil command")
	}

	var names []string

	parent := c
	for parent != nil {
		names = append([]string{parent.Name}, names...)
		parent = parent.Parent
	}

	return names
}

// Run runs the command by calling the correct plugin.
func (c *Command) Run(ctx context.Context, store *Store, cfg, pluginCfg api.KeyValues, tasks []TaskConfig) error {
	if c == nil {
		panic("calling Run on nil command")
	}

	if c.Plugin == nil {
		panic(fmt.Sprintf("command %q has nil plugin", c.Name))
	}

	if err := store.start(ctx, c.Plugin, tasks); err != nil {
		return err
	}

	names := c.Names()

	if c.Plugin.External() {
		names = names[1:]
	}

	name := strings.Join(names, ".")

	return callRunCommand(ctx, c.Plugin, name, cfg, pluginCfg)
}

// LogValue implements [slog.LogValuer] for logCmds. It formats the slice of
// commands as a group correctly for the different types of [slog.Handler] in
// use.
func (c logCmds) LogValue() slog.Value {
	if len(c) == 0 {
		return slog.StringValue("<nil>")
	}

	attrs := make([]slog.Attr, len(c))
	for i, cmd := range c {
		attrs[i] = slog.Any(cmd.Name, cmd)
	}

	return slog.GroupValue(attrs...)
}

// newCommand creates the internal command representation for the given command
// manifest and its subcommands.
func newCommand(plugin Plugin, manifest *api.Command) *Command {
	if manifest == nil {
		panic("creating command for nil manifest")
	}

	cmd := &Command{
		Command:  manifest,
		Commands: nil,
		Parent:   nil,
		Plugin:   plugin,
	}

	var cmds []*Command

	if len(manifest.Commands) > 0 {
		cmds = make([]*Command, len(manifest.Commands))

		for i, c := range manifest.Commands {
			newCmd := newCommand(plugin, c)
			newCmd.Parent = cmd
			cmds[i] = newCmd
		}
	}

	cmd.Commands = cmds

	return cmd
}

// newCommands creates the internal command representations for the given
// plugin. It panics if the plugin is nil.
func newCommands(plugin Plugin) []*Command {
	if plugin == nil {
		panic("creating commands for nil plugin")
	}

	manifest := plugin.Manifest()
	if manifest == nil || len(manifest.Commands) == 0 {
		return nil
	}

	if !plugin.External() {
		cmds := make([]*Command, len(manifest.Commands))

		for i, cmd := range manifest.Commands {
			cmds[i] = newCommand(plugin, cmd)
		}

		return cmds
	}

	cmdInfo := &api.Command{
		Name:        manifest.Domain,
		Usage:       manifest.Domain + " [command] [options]",
		Description: manifest.Description,
		Help:        manifest.Help,
		Manual:      "",
		Aliases:     nil,
		Config:      manifest.Config,
		Commands:    manifest.Commands,
		Args:        nil,
	}

	return []*Command{newCommand(plugin, cmdInfo)}
}
