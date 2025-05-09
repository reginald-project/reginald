// Package root defines the root command for Reginald.
package root

import "github.com/anttikivi/reginald/internal/cli"

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New(version string) *cli.RootCommand {
	c := &cli.RootCommand{
		Command: cli.Command{UsageLine: name},
		Version: version,
	}

	c.StandardFlags().Bool("version", false, "print the version information and exit")
	c.StandardFlags().BoolP("help", "h", false, "show the help message and exit")

	return c
}
