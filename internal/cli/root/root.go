// Package root defines the root command for Reginald.
package root

import "github.com/anttikivi/reginald/internal/cli"

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New() *cli.Command {
	c := &cli.Command{
		UsageLine: name,
	}

	c.GlobalFlags().Bool("version", false, "print the version information and exit")

	return c
}
