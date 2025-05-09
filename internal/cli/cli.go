// Package cli defines the command-line interface of Reginald.
package cli

import (
	"fmt"
	"os"
)

// programName is the canonical name of this program.
const programName = "Reginald"

// Run executes the given root command. It parses the command-line options,
// finds the correct subcommand to run, and executes it. It returns any error
// encountered during the run. The function panics if it is called with invalid
// configuration, e.g. with command other than the root command.
func Run(cmd *RootCommand) error {
	if cmd.HasParent() {
		panic("the CLI must be run using the root command")
	}

	args := os.Args[1:]

	// Ignore parsing errors as the parsing is continued with the options for
	// the subcommands.
	_ = cmd.StandardFlags().Parse(args)

	help, err := cmd.StandardFlags().GetBool("help")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--help': %w", err)
	}

	if help {
		fmt.Fprintln(os.Stdout, "HELP")

		return nil
	}

	version, err := cmd.StandardFlags().GetBool("version")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--version': %w", err)
	}

	if version {
		fmt.Fprintf(os.Stdout, "%s %s\n", programName, cmd.Version)

		return nil
	}

	return nil
}
