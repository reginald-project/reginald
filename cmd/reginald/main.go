// Package main is the entry point for Reginald, the personal workstation valet.
// TODO: Add a comment describing the actual command when there is something to
// describe.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/cli/root"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/version"
)

func main() {
	if err := logging.InitBootstrap(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	slog.Debug("bootstrap logger initialized")
	slog.Info("bootstrapping Reginald", "version", version.Version)

	rootCmd, err := root.New(version.Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := cli.Run(rootCmd); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
