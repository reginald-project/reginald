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
	"runtime"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
)

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
)

// A runInfo is the parsed information for the program run. It is returned from
// the bootstrapping function.
type runInfo struct {
	cmd     *plugin.Command // the command that was run
	cfg     *config.Config  // config for the run
	store   *plugin.Store   // loaded plugins
	args    []string        //nolint:unused // TODO: Will be used soon.
	help    bool            // whether the help flag was set
	version bool            // whether the version flag was set
}

// Run runs the CLI application and returns any errors from the run.
func Run(ctx context.Context) error {
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
		printVersion(info.cmd)

		return nil
	}

	return nil
}

// pirntVersion prints the program's version or, if the user specified
// the "--version" flag for a command from a plugin, the version of the plugin.
func printVersion(cmd *plugin.Command) {
	terminal.Printf(
		"%s version %s (%s/%s)\n",
		Name,
		version.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)

	if cmd != nil && cmd.Plugin.Manifest().Name != "builtin" {
		manifest := cmd.Plugin.Manifest()
		terminal.Printf("Plugin %q version %s\n", manifest.Name, manifest.Version)
		terminal.Println()
		terminal.Printf(
			"%s is licensed under the Apache License, Version 2.0: <https://www.apache.org/licenses/LICENSE-2.0>\n",
			ProgramName,
		)
	} else {
		terminal.Println("Licensed under the Apache License, Version 2.0: <https://www.apache.org/licenses/LICENSE-2.0>")
	}

	terminal.Flush()
}
