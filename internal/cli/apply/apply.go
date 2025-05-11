// Package apply contains the apply subcommand for Reginald.
package apply

import "github.com/anttikivi/reginald/internal/cli"

// New returns a new apply subcommand.
func New() *cli.Command {
	c := &cli.Command{
		UsageLine: "apply [options]",
	}

	return c
}
