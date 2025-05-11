// Package logging defines utilities for logging within Reginald. The program
// uses the [log/slog] package for logging, and this package contains the
// function for setting up the logging.
//
// At the first phase before parsing the configuration, logging is done using
// the bootstrap logger that is set as the default logger first. After the
// bootstrapping, the default logger should be replaced with the actual logger
// that is set up according to the user's configuration. The bootstrap logger is
// configured using environment variables if customizing it is needed.
package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Errors for logging.
var (
	errInvalidOutput = errors.New("given log output not supported")
)

// InitBootstrap initializes the bootstrap logger and sets it as the default
// logger in [log/slog]. It returns the encountered error.
func InitBootstrap() error {
	outputName := os.Getenv("REGINALD_BOOTSTRAP_OUTPUT")
	if outputName == "" {
		// TODO: The default should be file, but let's implement that later.
		outputName = "stderr"
	}

	var output io.Writer

	switch strings.ToLower(outputName) {
	case "stderr":
		output = os.Stderr
	case "stdout":
		output = os.Stdout
	default:
		return fmt.Errorf("failed to create the bootstrap logger: %w", errInvalidOutput)
	}

	handler := slog.NewTextHandler(output, &slog.HandlerOptions{ //nolint:exhaustruct
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	logger := slog.New(handler)

	slog.SetDefault(logger)

	return nil
}
