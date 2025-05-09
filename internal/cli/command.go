package cli

import (
	"strings"

	"github.com/spf13/pflag"
)

// A Command is a CLI command. All commands, including the root command and the
// subcommands, must be implemented as commands. A RootCommand can also be used
// for the root command.
type Command struct {
	// UsageLine is the one-line usage synopsis for the command. It should start
	// with the command name without including the parent commands.
	UsageLine string

	flags       *pflag.FlagSet // all of the command-line options
	globalFlags *pflag.FlagSet // options that are inherited by the subcommands
	parent      *Command       // parent command for this command
}

// A RootCommand is a special command that is reserved to be used as the root
// command of the program. It includes some additional information, e.g. the
// version number of the program and the standard options like '--version' and
// '--help'.
type RootCommand struct {
	Command

	// Version is the version number of the command. It
	Version string

	standardFlags *pflag.FlagSet // standard options in the root command
}

// Name returns the commands name.
func (c *Command) Name() string {
	n := c.UsageLine

	i := strings.Index(n, " ")
	if i != -1 {
		n = n[:i]
	}

	return n
}

// Flags returns the set of command-line options that contains all of the
// command-line options associated with this Command.
func (c *Command) Flags() *pflag.FlagSet {
	if c.flags == nil {
		c.flags = c.flagSet()
	}

	return c.flags
}

// GlobalFlags returns the set of command-line options of this command that are
// inherited by the subcommands.
func (c *Command) GlobalFlags() *pflag.FlagSet {
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

// StandardFlags returns the set of standard command-line options of this
// program. It can only be used by the root command, and it should contain the
// '--help' and '--version' options.
func (c *RootCommand) StandardFlags() *pflag.FlagSet {
	if c.HasParent() {
		panic("standard flags cannot be used on commands other than the root command")
	}

	if c.standardFlags == nil {
		c.standardFlags = c.flagSet()
	}

	return c.standardFlags
}

// flagSet returns a new flag set suitable to be used with Command.
func (c *Command) flagSet() *pflag.FlagSet {
	return pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
}

// mergeFlags merges the global options of this Command to the set of all
// options and adds the global options from parents.
func (c *Command) mergeFlags() {
	c.Root().GlobalFlags().AddFlagSet(pflag.CommandLine)
	c.VisitParents(func(p *Command) {
		c.GlobalFlags().AddFlagSet(p.GlobalFlags())
	})
	c.Flags().AddFlagSet(c.GlobalFlags())
}
