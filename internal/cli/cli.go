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

// Package cli defines the command-line interface of Reginald.
package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/panichandler"
	"github.com/reginald-project/reginald/internal/plugin"
)

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
)

// A runInfo is the parsed information for the program run. It is returned from
// the bootstrapping function.
type runInfo struct {
	cfg     *config.Config //nolint:unused // TODO: Will be used soon.
	store   *plugin.Store  //nolint:unused // TODO: Will be used soon.
	args    []string       //nolint:unused // TODO: Will be used soon.
	help    bool           // whether the help flag was set
	version bool           // whether the version flag was set
}

// Run runs the CLI application and returns any errors from the run.
func Run() error {
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

	info, err := initialize(ctx)
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{
				Code: exitErr.Code,
				err:  err,
			}
		}

		return &ExitError{
			Code: 1,
			err:  err,
		}
	}

	if info.help {
		return nil
	}

	if info.version {
		return nil
	}

	return nil
}
