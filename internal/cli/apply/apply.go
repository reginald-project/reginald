// Package apply contains the apply subcommand for Reginald.
package apply

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/anttikivi/reginald/internal/cli"
)

// New returns a new apply subcommand.
func New() *cli.Command {
	c := &cli.Command{
		UsageLine: "apply [options]",
		Setup:     setup,
		Run:       run,
	}

	return c
}

func setup(cmd *cli.Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	return nil
}

func run(_ *cli.Command, _ []string) error {
	fmt.Fprintln(os.Stdout, "RUN APPLY")

	return nil
}
