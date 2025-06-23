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
	"sort"
	"strings"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
	"github.com/spf13/pflag"
)

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
	usagePrefix = "Usage: "
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
		return runHelp(info.cmd)
	}

	if info.version {
		runVersion(info.cmd)

		return nil
	}

	return nil
}

// defaultUsage returns the default usage message for the program.
func defaultUsage() string { //nolint:gocognit // no need to split this up
	flagSet := newFlagSet()
	mutexGroups := flagSet.MutuallyExclusive()
	grouped := make(map[string]bool, 0)

	for _, group := range mutexGroups {
		for _, name := range group {
			grouped[name] = true
		}
	}

	var singles []string

	flagSet.VisitAll(func(f *pflag.Flag) {
		if grouped[f.Name] {
			return
		}

		value, _ := pflag.UnquoteUsage(f)
		s := "["

		if f.Shorthand != "" {
			s += "-" + f.Shorthand + " "

			if value != "" {
				if f.NoOptDefVal != "" {
					s += "[" + value + "]"
				} else {
					s += value
				}

				s += " "
			}

			s += "| "
		}

		s += "--" + f.Name

		if value != "" {
			if f.NoOptDefVal != "" {
				s += "[=" + value + "]"
			} else {
				s += "=" + value
			}
		}

		s += "]"

		singles = append(singles, s)
	})
	sort.Strings(singles)

	mutexParts := make([]string, 0, len(mutexGroups))

	for _, group := range mutexGroups {
		var elems []string

		for _, name := range group {
			f := flagSet.Lookup(name)
			if f == nil {
				panic(
					fmt.Sprintf(
						"failed to find flag %q marked as mutually exclusive when creating the usage message",
						name,
					),
				)
			}

			value, _ := pflag.UnquoteUsage(f)
			s := "--" + f.Name

			if value != "" {
				if f.NoOptDefVal != "" {
					s += "[=" + value + "]"
				} else {
					s += "=" + value
				}
			}

			elems = append(elems, s)

			continue
		}

		sort.Strings(elems)

		mutexParts = append(mutexParts,
			fmt.Sprintf("[%s]", strings.Join(elems, " | ")),
		)
	}

	usages := make([]string, 0, len(singles)+len(mutexParts))
	usages = append(usages, singles...)
	usages = append(usages, mutexParts...)

	sort.Strings(usages)

	parts := []string{Name}
	parts = append(parts, usages...)

	return strings.Join(parts, " ")
}

func printHelp(_ *plugin.Command, flagSet *flags.FlagSet) {
	fmt.Fprintf(os.Stderr, "%s\n", defaultUsage())
	flagSet.PrintDefaults()
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

func runHelp(cmd *plugin.Command) error {
	root := rootCommand(cmd)
	flagSet := newFlagSet()

	var found *plugin.Command

Loop:
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" {
			break
		}

		if found != nil {
			for _, c := range found.Commands {
				if c.Name == arg || slices.Contains(c.Aliases, arg) {
					found = c

					if err := addFlags(flagSet, found); err != nil {
						return err
					}

					continue Loop
				}
			}

			continue
		}

		if arg == root.Name || slices.Contains(root.Aliases, arg) {
			found = root

			if err := addFlags(flagSet, found); err != nil {
				return err
			}
		}
	}

	printHelp(found, flagSet)

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
