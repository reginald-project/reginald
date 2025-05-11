// Package root defines the root command for Reginald.
package root

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/cli/apply"
)

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New(version string) *cli.RootCommand {
	c := &cli.RootCommand{
		Command: cli.Command{
			UsageLine: name + " [--version] [-h | --help] <command> [<args>]",
			Setup:     setup,
			Run:       run,
		},
		Version: version,
	}

	c.GlobalFlags().Bool("version", false, "print the version information and exit")
	c.GlobalFlags().BoolP("help", "h", false, "show the help message and exit")

	c.Add(apply.New())

	return c
}

func setup(cmd *cli.Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	return nil
}

func run(_ *cli.Command, _ []string) error {
	fmt.Fprintln(os.Stdout, "HELP MESSAGE")

	return nil
}
