package cli

import (
	"context"
	"log/slog"
)

// NewApply returns a new apply command.
func NewApply() *Command {
	c := &Command{ //nolint:exhaustruct
		UsageLine: "apply [options]",
		Setup:     setupApply,
		Run:       runApply,
	}

	return c
}

func setupApply(_ context.Context, cmd, _ *Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	return nil
}

func runApply(_ context.Context, _ *Command, _ []string) error {
	slog.Info("running apply")

	return nil
}
