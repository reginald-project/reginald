package cli

import (
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

func setupApply(cmd, _ *Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	return nil
}

func runApply(_ *Command, _ []string) error {
	slog.Info("running apply")

	return nil
}
