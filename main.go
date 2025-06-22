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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/reginald-project/reginald/internal/cli"
	"github.com/reginald-project/reginald/internal/panichandler"
)

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

	err := cli.Run(ctx)
	if err != nil {
		var successErr *cli.SuccessError
		if errors.As(err, &successErr) {
			return 0
		}

		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}

		return 1
	}

	return 0
}
