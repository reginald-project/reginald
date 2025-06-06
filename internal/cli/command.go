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

package cli

import (
	"context"
	"fmt"

	"github.com/anttikivi/reginald/internal/flags"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/spf13/pflag"
)

// A Command is a CLI command. All commands, including the root command and the
// subcommands, must be implemented as commands. A RootCommand can also be used
// for the root command.
type Command struct {
	// Name is the name of the command as it should be written by the user when
	// they run the command.
	Name string

	// UsageLine is the one-line usage synopsis for the command. It should start
	// with the command name without including the parent commands.
	UsageLine string

	// Aliases are the aliases for the command that can be used instead of
	// the real name of the command to run it. All of the aliases and command
	// names must be unique.
	Aliases []string

	// Setup runs the setup required for the Command.
	Setup func(ctx context.Context, cmd *Command, args []string) error

	// Runs runs the command. Before running the command, Setup function for it
	// is run.
	Run func(ctx context.Context, cmd *Command) error

	cli                    *CLI            // containing CLI struct
	plugin                 *plugins.Plugin // plugin that provided the command, nil for internal commands
	flags                  *flags.FlagSet  // all of the command-line options
	globalFlags            *flags.FlagSet  // options that are inherited by the subcommands
	parent                 *Command        // parent command of this command if it is a subcommand
	commands               []*Command      // list of subcommands
	mutuallyExclusiveFlags [][]string      // list of flag names that are marked as mutually exclusive
}

// Add adds the given command to the list of subcommands of c and marks c as the
// parent command of cmd.
func (c *Command) Add(cmd *Command) {
	if c == cmd {
		panic(fmt.Sprintf("failed to add the command %s as a subcommand of itself", cmd.Name))
	}

	cmd.parent = c

	if c.mutuallyExclusiveFlags != nil {
		if cmd.mutuallyExclusiveFlags == nil {
			cmd.mutuallyExclusiveFlags = [][]string{}
		}

		cmd.mutuallyExclusiveFlags = append(cmd.mutuallyExclusiveFlags, c.mutuallyExclusiveFlags...)
	}

	c.commands = append(c.commands, cmd)
}

// Lookup returns the subcommand for this command for the given name, if any.
// Otherwise it returns nil.
func (c *Command) Lookup(name string) *Command {
	for _, cmd := range c.commands {
		// TODO: Check for aliases.
		if cmd.Name == name {
			return cmd
		}
	}

	return nil
}

// MarkFlagsMutuallyExclusive marks two or more flags as mutually exclusive so
// that the program returns an error if the user tries to set them at the same
// time.
func (c *Command) MarkFlagsMutuallyExclusive(a ...string) {
	c.mergeFlags()

	if len(a) < 2 { //nolint:mnd // obvious
		panic("only one flag cannot be marked as mutually exclusive")
	}

	for _, s := range a {
		if f := c.Flags().Lookup(s); f == nil {
			panic(fmt.Sprintf("failed to find flag %q while marking it as mutually exclusive", s))
		}
	}

	if c.mutuallyExclusiveFlags == nil {
		c.mutuallyExclusiveFlags = [][]string{}
	}

	c.mutuallyExclusiveFlags = append(c.mutuallyExclusiveFlags, a)
}

// Flags returns the set of command-line options that contains all of the
// command-line options associated with this Command.
func (c *Command) Flags() *flags.FlagSet {
	if c.flags == nil {
		c.flags = c.flagSet()
	}

	return c.flags
}

// GlobalFlags returns the set of command-line options of this command that are
// inherited by the subcommands.
func (c *Command) GlobalFlags() *flags.FlagSet {
	if c.globalFlags == nil {
		c.globalFlags = c.flagSet()
	}

	return c.globalFlags
}

// HasParent tells if this command has a parent, i.e. it is a subcommand.
func (c *Command) HasParent() bool {
	return c.parent != nil
}

// Root returns the root command for this command.
func (c *Command) Root() *Command {
	if c.HasParent() {
		return c.parent.Root()
	}

	return c
}

// VisitParents executes the function fn on all of the command's parents.
func (c *Command) VisitParents(fn func(*Command)) {
	if c.HasParent() {
		fn(c.parent)
		c.parent.VisitParents(fn)
	}
}

// flagSet returns a new flag set suitable to be used with Command.
func (c *Command) flagSet() *flags.FlagSet {
	return flags.NewFlagSet(c.Name, pflag.ContinueOnError)
}

// mergeFlags merges the global options of this Command to the set of all
// options and adds the global options from parents.

// mergeFlags merges all of the global command-line flags from its parent Commands to its flag set.
func (c *Command) mergeFlags() {
	c.VisitParents(func(p *Command) {
		c.GlobalFlags().AddFlagSet(p.GlobalFlags())
	})
	c.Flags().AddFlagSet(c.GlobalFlags())
}
