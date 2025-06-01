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
		Setup:     setupAttend,
		Run:       runAttend,
	}

	return c
}

func setupAttend(ctx context.Context, _ *Command, _ []string) error {
	logging.InfoContext(ctx, "setting up attend")

	return nil
}

func runAttend(ctx context.Context, _ *Command) error {
	logging.InfoContext(ctx, "running attend")

	return nil
}
