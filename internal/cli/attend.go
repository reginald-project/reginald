package cli

import (
	"context"

	"github.com/anttikivi/reginald/internal/logging"
)

// NewAttend returns a new apply command.
func NewAttend() *Command {
	c := &Command{ //nolint:exhaustruct // private fields need zero values
		Name: "attend",
		Aliases: []string{
			"apply",
			"tend",
		},
		UsageLine: "attend [options]",
		Run:       runAttend,
	}

	return c
}

func runAttend(ctx context.Context, _ *Command) error {
	logging.InfoContext(ctx, "running attend")

	return nil
}
