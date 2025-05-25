package cli

import (
	"context"

	"github.com/anttikivi/reginald/internal/logging"
)

// NewApply returns a new apply command.
func NewApply() *Command {
	c := &Command{ //nolint:exhaustruct
		Name:      "apply",
		UsageLine: "apply [options]",
		Setup:     setupApply,
		Run:       runApply,
	}

	return c
}

func setupApply(_ context.Context, cmd, _ *Command, _ []string) error {
	logging.Info("running setup", "cmd", cmd.Name)

	return nil
}

func runApply(_ context.Context, _ *Command, _ []string) error {
	logging.Info("running apply")

	return nil
}
