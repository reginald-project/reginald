package cli

import (
	"context"

	"github.com/anttikivi/reginald/internal/logging"
)

// NewApply returns a new apply command.
func NewApply() *Command {
	c := &Command{ //nolint:exhaustruct // private fields need zero values
		Name:      "apply",
		UsageLine: "apply [options]",
		Run:       runApply,
	}

	return c
}

func runApply(ctx context.Context, _ *Command) error {
	logging.InfoContext(ctx, "running apply")

	return nil
}
