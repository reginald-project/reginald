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

package plugin

import (
	"log/slog"

	"github.com/reginald-project/reginald-sdk-go/api"
)

// A Command is the program representation of a plugin command that is defined
// in the manifest.
type Command struct {
	// Name is the name of the command as it should be written on
	// the command-line by the user.
	Name string

	// Usage is the one-line usage of the command that is shown to the user in
	// the help message.
	Usage string

	// Description is the description of the command that is shown to the user
	// in the help message.
	Description string

	// Aliases is a list of aliases for the command that can be used instead of
	// Name to run this command.
	Aliases []string

	// Config is a list of ConfigEntries that are used to define
	// the configuration of the command.
	Config []api.ConfigEntry `json:"config,omitempty"`

	// Commands is a list of subcommands that this command provides.
	Commands []*Command
}

// LogCmds is a helper type for logging a slice of commands.
type LogCmds []*Command

// LogValue implements [slog.LogValuer] for LogCmds. It formats the slice of
// commands as a group correctly for the different types of [slog.Handler] in
// use.
func (c LogCmds) LogValue() slog.Value {
	if len(c) == 0 {
		return slog.StringValue("<nil>")
	}

	attrs := make([]slog.Attr, len(c))
	for i, cmd := range c {
		attrs[i] = slog.Any(cmd.Name, cmd)
	}

	return slog.GroupValue(attrs...)
}

// LogValue implements [slog.LogValuer] for Command. It returns a group value
// for logging a Command.
func (c *Command) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", c.Name),
		slog.String("usage", c.Usage),
		slog.String("description", c.Description),
		slog.Any("aliases", c.Aliases),
		slog.Any("commands", LogCmds(c.Commands)),
	)
}

// newCommand creates the internal command representation for the given command
// manifest and its subcommands.
func newCommand(manifest *api.Command) *Command {
	var cmds []*Command

	if len(manifest.Commands) > 0 {
		cmds = make([]*Command, 0, len(manifest.Commands))

		for _, cmd := range manifest.Commands {
			cmds = append(cmds, newCommand(cmd))
		}
	}

	cmd := &Command{
		Name:        manifest.Name,
		Usage:       manifest.Usage,
		Description: manifest.Description,
		Aliases:     manifest.Aliases,
		Config:      manifest.Config,
		Commands:    cmds,
	}

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

	if manifest.Domain == "core" {
		cmds := make([]*Command, 0, len(manifest.Commands))

		for _, cmd := range manifest.Commands {
			cmds = append(cmds, newCommand(cmd))
		}

		return cmds
	}

	cmdInfo := &api.Command{
		Name:        manifest.Domain,
		Usage:       manifest.Domain + " [options]",
		Description: manifest.Description,
		Aliases:     nil,
		Config:      manifest.Config,
		Commands:    manifest.Commands,
	}

	cmds := make([]*Command, 1)
	cmds[0] = newCommand(cmdInfo)

	return cmds
}
