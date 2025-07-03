// Copyright 2025 The Reginald Authors
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

/*
Reginald is the personal workstation valet.

TODO: Add a comment describing the actual command when there is something to
describe.
*/
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/reginald-project/reginald/internal/cli"
	"github.com/reginald-project/reginald/internal/panichandler"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
)

//go:embed version
var versionFile string

func init() { //nolint:gochecknoinits // initializes the version information
	version.Init(versionFile)
}

func main() {
	code := run()
	if code != 0 {
		os.Exit(code)
	}
}

// run runs the CLI and returns the exit code.
func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	panichandler.SetCancel(cancel)

	defer panichandler.Handle()

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

	// Discard logs until the config is parsed.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	terminal.Set(terminal.New(ctx))

	var wg sync.WaitGroup

	cleanupCh := make(chan error, 1)
	handleCleanupPanic := panichandler.WithStackTrace()

	wg.Add(1)

	go func() {
		defer wg.Done()
		defer handleCleanupPanic()
		<-ctx.Done()

		if err := terminal.Default().Close(); err != nil {
			cleanupCh <- err

			return
		}

		cleanupCh <- nil
	}()

	exitCode := 0

	if err := cli.Execute(ctx); err != nil {
		var successErr *cli.SuccessError
		if !errors.As(err, &successErr) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)

			var exitErr *cli.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.Code
			} else {
				exitCode = 1
			}
		}
	}

	cancel()
	wg.Wait()

	if err := <-cleanupCh; err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, readline.ErrInterrupt) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

		exitCode = 1
	}

	return exitCode
}
