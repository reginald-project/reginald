// Package main is the entry point for Reginald, the personal workstation valet.
// TODO: Add a comment describing the actual command when there is something to
// describe.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/panichandler"
	"github.com/anttikivi/reginald/pkg/version"
)

func main() {
	code := run()
	if code != 0 {
		os.Exit(code)
	}
}

func run() int {
	defer panichandler.Handle()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up canceling the context on certain signals so the plugins are
	// killed.
	sigc := make(chan os.Signal, 1)

	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	handlePanic := panichandler.WithStackTrace()
	go func() {
		defer handlePanic()
		<-sigc
		cancel()
	}()

	if err := logging.InitBootstrap(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		return 1
	}

	slog.DebugContext(ctx, "bootstrap logger initialized")
	slog.InfoContext(
		ctx,
		"bootstrapping Reginald",
		"version",
		version.Version(),
		"commit",
		version.BuildCommit(),
		"buildTime",
		version.BuildTime(),
	)

	c := cli.New()
	if err := c.Execute(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		return 1
	}

	if err := c.DeferredErr(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		return 1
	}

	return 0
}
