// Copyright 2025 Antti Kivi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main is the entry point for Reginald, the personal workstation valet.
// TODO: Add a comment describing the actual command when there is something to
// describe.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/panichandler"
	"github.com/anttikivi/reginald/internal/plugins"
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

	logging.DebugContext(ctx, "bootstrap logger initialized")
	logging.InfoContext(
		ctx,
		"bootstrapping Reginald",
		"version",
		version.Version(),
		"commit",
		version.Revision(),
	)

	if err := runCLI(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		return 1
	}

	return 0
}

func runCLI(ctx context.Context) error {
	c := cli.New()
	if ok, err := c.Initialize(ctx); err != nil {
		return fmt.Errorf("%w", err)
	} else if !ok {
		return nil
	}

	iostreams.Streams = iostreams.New(c.Cfg.Quiet, c.Cfg.Verbose, c.Cfg.Color)

	if err := logging.Init(c.Cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	logging.DebugContext(ctx, "logging initialized")
	logging.InfoContext(ctx, "executing Reginald", "version", version.Version())

	if err := c.LoadPlugins(ctx); err != nil {
		return fmt.Errorf("failed to resolve plugins: %w", err)
	}

	// We want to aim for a clean plugin shutdown in all cases, so the shut down
	// should be run in all cases where the plugins have been initialized.
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, plugins.DefaultShutdownTimeout)
		defer cancel()

		if err := plugins.ShutdownAll(timeoutCtx, c.Plugins); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to shut down plugins: %v\n", err)
		}
	}()

	if err := c.Execute(ctx); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
