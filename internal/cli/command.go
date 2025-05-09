package cli

import (
	"strings"

	"github.com/spf13/pflag"
)

// A Command is a CLI command. All commands, including the root command and the
// subcommands, must be implemented as commands.
type Command struct {
	// UsageLine is the one-line usage synopsis for the command. It should start
	// with the command name without including the parent commands.
	UsageLine string

	globalFlags *pflag.FlagSet // options that are inherited by the subcommands
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

// GlobalFlags returns the set of command-line options of this command that are
// inherited by the subcommands.
func (c *Command) GlobalFlags() *pflag.FlagSet {
	if c.globalFlags == nil {
		c.globalFlags = c.flagSet()
	}

	return c.globalFlags
}

// flagSet returns a new flag set suitable to be used with Command.
func (c *Command) flagSet() *pflag.FlagSet {
	return pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
}
