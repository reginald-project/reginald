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
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/plugin/runtimes"
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
	flagSet *flags.FlagSet  // flag set for the run
	args    []string        // positional arguments
	help    bool            // whether the help flag was set
	version bool            // whether the version flag was set
}

// Execute runs the CLI application and returns any errors from the run.
func Execute(ctx context.Context) error {
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
		return runHelp(info.cmd, info.store)
	}

	if info.version {
		runVersion(info.cmd)

		return nil
	}

	if err = runtimes.Resolve(ctx, info.store, info.cfg); err != nil {
		return &ExitError{
			Code: 1,
			err:  err,
		}
	}

	if err = info.store.Init(ctx, info.cfg.Tasks); err != nil {
		return &ExitError{
			Code: 1,
			err:  err,
		}
	}

	shutdownDone := false

	shutdown := func() {
		if shutdownDone {
			return
		}

		if err = info.store.ShutdownAll(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error when shutting down plugins: %v\n", err)
		}

		shutdownDone = true
	}
	defer shutdown()

	if err = run(ctx, info); err != nil {
		return &ExitError{
			Code: 1,
			err:  err,
		}
	}

	shutdown()

	return nil
}

// printVersion prints the program's version or, if the user specified
// the "--version" flag for a command from a plugin, the version of the plugin.
func printVersion(cmd *plugin.Command) {
	terminal.Printf("%s version %s (%s/%s)\n", Name, version.Version(), runtime.GOOS, runtime.GOARCH)

	if cmd != nil && cmd.Plugin.External() {
		manifest := cmd.Plugin.Manifest()
		terminal.Printf("Plugin %q version %s\n", manifest.Name, manifest.Version)
		terminal.Println()
		terminal.Printf(
			"%s is licensed under the Apache License, Version 2.0: <https://www.apache.org/licenses/LICENSE-2.0>\n",
			ProgramName,
		)
	} else {
		terminal.Println("Licensed under the Apache License, Version 2.0: <https://www.apache.org/licenses/LICENSE-2.0>") //nolint:lll
	}

	terminal.Flush()
}

// rootCommand returns the root command of the given command.
func rootCommand(cmd *plugin.Command) *plugin.Command {
	if cmd == nil {
		return nil
	}

	root := cmd

	for root.Parent != nil {
		root = root.Parent
	}

	return root
}

// run runs the requested command.
func run(ctx context.Context, info *runInfo) error {
	var (
		err       error
		cfg       api.KeyVal
		pluginCfg api.KeyValues
	)

	cfgs := info.cfg.Plugins

	i := slices.IndexFunc(cfgs, func(kv api.KeyVal) bool { return kv.Key == info.cmd.Plugin.Manifest().Domain })
	if i != -1 {
		pluginCfg, err = cfgs[i].Configs()
		if err != nil {
			return fmt.Errorf("failed to get config for %q: %w", info.cmd.Plugin.Manifest().Name, err)
		}
	}

	names := info.cmd.Names()

	for len(names) > 0 {
		s := names[0]

		var ok bool

		cfg, ok = cfgs.Get(s)
		if !ok {
			return fmt.Errorf("%w: %s", errCmdConfig, s)
		}

		cfgs, err = cfg.Configs()
		if err != nil {
			return fmt.Errorf("failed to get configs from KeyVal %q: %w", cfg.Key, err)
		}

		names = names[1:]
	}

	if err = info.cmd.Run(ctx, info.store, cfgs, pluginCfg, info.cfg.Tasks); err != nil {
		return fmt.Errorf("running command %q failed: %w", strings.Join(info.cmd.Names(), " "), err)
	}

	return nil
}

// runVersion runs the version command or flag by resolving the place of
// the command or the flag in the arguments list. It prints the version of
// the command that was given before the flag.
func runVersion(cmd *plugin.Command) {
	root := rootCommand(cmd)

	var found *plugin.Command

Loop:
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			break
		}

		if found != nil {
			for _, c := range found.Commands {
				if c.Name == arg || slices.Contains(c.Aliases, arg) {
					found = c

					continue Loop
				}
			}

			continue
		}

		if arg == root.Name || slices.Contains(root.Aliases, arg) {
			found = root
		}
	}

	printVersion(found)
}
