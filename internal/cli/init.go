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

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
	"github.com/spf13/pflag"
)

// addFlags adds the flags from the given command to the flag set.
func addFlags(flagSet *flags.FlagSet, cmd *plugin.Command) error {
	for _, cfg := range cmd.Config {
		if err := flagSet.AddPluginFlag(&cfg); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	return nil
}

// bootstrap initializes the program run by creating the logger and output
// streams, loading the plugin information, and parsing the command-line
// arguments.
func bootstrap(ctx context.Context) (*runInfo, error) {
	streams := terminal.NewIO(ctx, false, false, false, terminal.ColorNever)
	defer streams.Close()

	if err := logging.InitBootstrap(streams); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logging.Debug(ctx, "bootstrap logger initialized")
	logging.Info(
		ctx,
		"bootstrapping Reginald",
		"version",
		version.Version(),
		"commit",
		version.Revision(),
	)

	cfg, err := initConfig(ctx)
	if err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  err,
		}
	}

	if err = initOut(ctx, cfg); err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  err,
		}
	}

	defer terminal.Streams().Close()

	logging.Info(ctx, "executing Reginald", "version", version.Version())

	if !cfg.HasFile() {
		if !terminal.Confirm("No config file was found. Continue?", true) {
			return nil, &SuccessError{}
		}

		if !cfg.Interactive {
			terminal.Warnln("No config file was found")
		}
	}

	plugins, err := initPlugins(ctx, cfg)
	if err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  fmt.Errorf("%w", err),
		}
	}

	err = parseArgs(ctx, cfg, plugins)
	if err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  fmt.Errorf("failed to parse the command-line arguments: %w", err),
		}
	}

	info := &runInfo{
		cfg:     cfg,
		plugins: plugins,
		args:    nil,
	}

	return info, nil
}

// collectFlags removes all of the known flags from the arguments list and
// appends them to flags. It returns the non-flag arguments as the first return
// value and the appended flags as the second return value. It does not check
// for any errors; all of the arguments that might look like flags but are not
// found in the flag set are treated as regular command-line arguments. If the
// user has run the program correctly, this function should return the next
// subcommand as the first element of the argument slice.
func collectFlags(flagSet *flags.FlagSet, args, collected []string) ([]string, []string) {
	if len(args) == 0 {
		return args, collected
	}

	rest := []string{}

	// TODO: This is probably way more inefficient than it needs to be, but it
	// gets the work done for now.
Loop:
	for len(args) > 0 {
		s := args[0]
		args = args[1:]

		switch {
		case s == "--":
			// Stop parsing at "--".
			break Loop
		case strings.HasPrefix(s, "-") && strings.Contains(s, "="):
			// All of the cases with "=": "--flag=value", "-f=value", and
			// "-abf=value".
			if hasFlag(flagSet, s) {
				collected = append(collected, s)
			} else {
				rest = append(rest, s)
			}
		case strings.HasPrefix(s, "--") && !hasNoOptDefVal(s[2:], flagSet):
			// The '--flag arg' case.
			fallthrough //nolint:gocritic // this is much clearer with an empty fallthrough
		case strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "--") && !shortHasNoOptDefVal(s[len(s)-1:], flagSet):
			// The '-f arg' and '-abcf arg' cases. Only the last flag in can
			// have a argument, so other ones aren't checked for the default
			// value.
			if hasFlag(flagSet, s) {
				if len(args) == 0 {
					collected = append(collected, s)
				} else {
					collected = append(collected, s, args[0])
					args = args[1:]
				}
			} else {
				rest = append(rest, s)
			}
		case strings.HasPrefix(s, "-") && len(s) >= 2:
			// Rest of the flags.
			if hasFlag(flagSet, s) {
				collected = append(collected, s)
			} else {
				rest = append(rest, s)
			}
		default:
			rest = append(rest, s)
		}
	}

	rest = append(rest, args...)

	return rest, collected
}

// findSubcommand finds the subcommand to run from the command tree starting at
// root command. It returns the names of the commands in order as the first
// value. The resulting slice of names can be used to get the subcommand from
// the plugin store. The slice does not include the root command. The rest of
// the command-line arguments remaining after the parsing are returned as
// the second return value. If no subcommand is found (i.e. the root command
// should be run), this function returns nil as the first return value.
//
// The function adds the flags from the subcommand to the flag set. The flag set
// is modified in-place.
func findSubcommands(
	ctx context.Context,
	flagSet *flags.FlagSet,
	store *plugin.Store,
	args []string,
) (*plugin.Command, []string) {
	if len(args) <= 1 {
		return nil, args
	}

	var cmd *plugin.Command

	flagsFound := []string{}

	for len(args) >= 1 {
		logging.Trace(ctx, "checking args", "cmd", cmd, "args", args, "flags", flagsFound)

		if len(args) > 1 {
			args, flagsFound = collectFlags(flagSet, args[1:], flagsFound)

			logging.Trace(ctx, "collected flags", "args", args, "flags", flagsFound)
		}

		if len(args) >= 1 {
			next := store.Command(cmd, args[0])

			logging.Trace(ctx, "next command", "cmd", next)

			if next == nil {
				break
			}

			cmd = next

			if err := addFlags(flagSet, cmd); err != nil {
				// TODO: This should be handled better.
				logging.Error(ctx, "failed to add flags from commands", "err", err)
				terminal.Errorf("Failed to add flags from commands: %v", err)

				return nil, nil
			}
		}
	}

	args = append(args, flagsFound...)

	return cmd, args
}

// hasFlag checks whether the given flag s is in fs. The whole flag string must
// be included. The function checks by looking up the shorthands if the string
// starts with only one hyphen. If s contains a combination of shorthands, the
// function will check for all of them.
func hasFlag(fs *flags.FlagSet, s string) bool {
	if strings.HasPrefix(s, "--") {
		if strings.Contains(s, "=") {
			return fs.Lookup(s[2:strings.Index(s, "=")]) != nil
		}

		return fs.Lookup(s[2:]) != nil
	}

	if strings.HasPrefix(s, "-") {
		if len(s) == 2 { //nolint:mnd // obvious
			return fs.ShorthandLookup(s[1:]) != nil
		}

		if strings.Index(s, "=") == 2 { //nolint:mnd // obvious
			return fs.ShorthandLookup(s[1:2]) != nil
		}

		for i := 1; i < len(s) && s[i] != '='; i++ {
			f := fs.ShorthandLookup(s[i : i+1])

			if f == nil {
				return false
			}
		}

		return true
	}

	return false
}

// hasNoOptDefVal checks if the given flag has a NoOptDefVal set.
func hasNoOptDefVal(name string, fs *flags.FlagSet) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}

	return f.NoOptDefVal != ""
}

// shortHasNoOptDefVal checks if the flag for the given shorthand has a
// NoOptDefVal set.
func shortHasNoOptDefVal(name string, fs *flags.FlagSet) bool {
	f := fs.ShorthandLookup(name[:1])
	if f == nil {
		return false
	}

	return f.NoOptDefVal != ""
}

// initConfig creates the initial config instance by locating the config file
// and parsing it with the basic set of flags provided by the CLI.
func initConfig(ctx context.Context) (*config.Config, error) {
	// Create a temporary flag set for the initialization.
	flagSet := newFlagSet()

	// Ignore errors for now as we want to get all of the flags from plugins
	// first.
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		logging.Debug(ctx, "initial flag parsing yielded an error", "err", err.Error())
	}

	cfg, err := config.Parse(ctx, flagSet)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return cfg, nil
}

// initOut initializes the output streams and the logging for the program.
func initOut(ctx context.Context, cfg *config.Config) error {
	terminal.SetStreams(terminal.NewIO(ctx, cfg.Quiet, cfg.Verbose, cfg.Interactive, cfg.Color))

	if err := logging.Init(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	logging.Debug(ctx, "logging initialized")

	return nil
}

// initPlugins looks up the plugin manifests and creates a new plugin store
// instance from them.
func initPlugins(ctx context.Context, cfg *config.Config) (*plugin.Store, error) {
	manifests, err := plugin.Search(ctx, cfg.Directory, cfg.PluginPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to search for plugins: %w", err)
	}

	plugins := plugin.NewStore(manifests)

	logging.Debug(ctx, "created plugins", "store", plugins)

	return plugins, nil
}

// newFlagSet creates a [flags.FlagSet] that contains the command-line flags for
// the root command of the program. The function panics on errors.
func newFlagSet() *flags.FlagSet {
	flagSet := flags.NewFlagSet(ProgramName, pflag.ContinueOnError)
	defaults := config.DefaultConfig()

	flagSet.Bool("version", false, "print the version information and exit", "")
	flagSet.BoolP("help", "h", false, "show the help message and exit", "")

	flagSet.StringP(
		"config",
		"c",
		"",
		"use `<path>` as the configuration file instead of resolving it from the standard locations",
		"",
	)
	flagSet.PathP(
		config.FlagName("Directory"),
		"C",
		defaults.Directory,
		"use `<path>` as the \"dotfiles\" directory so that Reginald looks for the config file and the files for linking from there", //nolint:lll
		"",
	)
	flagSet.PathSliceP(
		config.FlagName("PluginPaths"),
		"p",
		defaults.PluginPaths,
		"search for plugins from `<path>`",
		"",
	)

	verboseName := config.FlagName("Verbose")
	quietName := config.FlagName("Quiet")

	flagSet.BoolP(
		verboseName,
		"v",
		defaults.Verbose,
		"make "+ProgramName+" print more output during the run",
		"",
	)
	flagSet.BoolP(
		quietName,
		"q",
		defaults.Quiet,
		"make "+ProgramName+" print only error messages during the run",
		"",
	)
	flagSet.MarkMutuallyExclusive(quietName, verboseName)

	flagSet.BoolP(
		config.FlagName("Interactive"),
		"i",
		defaults.Interactive,
		"run in interactive mode",
		"",
	)

	colorMode := defaults.Color

	flagSet.Var(&colorMode, config.FlagName("Color"), "enable colors in the output", "")

	logName := config.FlagName("Logging.Enabled")
	noLogName := config.InvertedFlagName("Logging.Enabled")
	hiddenLogFlag := logName

	flagSet.Bool(logName, defaults.Logging.Enabled, "enable logging", "")
	flagSet.Bool(noLogName, !defaults.Logging.Enabled, "disable logging", "")
	flagSet.MarkMutuallyExclusive(logName, noLogName)

	if err := flagSet.MarkHidden(hiddenLogFlag); err != nil {
		panic(fmt.Sprintf("failed to mark --%s hidden: %v", hiddenLogFlag, err))
	}

	return flagSet
}

// parseArgs parses the command-line arguments and modifies the config according
// to them. The function creates a new flag set for the root command, finds
// the subcommand for the command-line arguments, and sets the flags from
// the subcommand to the flag set.
func parseArgs(ctx context.Context, cfg *config.Config, plugins *plugin.Store) error {
	// There is no need to remove the first element of the arguments slice as
	// findSubcommand takes care of that.
	args := os.Args

	// Make sure that `CommandLine` is not used.
	pflag.CommandLine.VisitAll(func(f *pflag.Flag) {
		panic(fmt.Sprintf("flag %q is set in the CommandLine flag set", f.Name))
	})
	logging.Debug(ctx, "parsing command-line arguments", "args", args)

	flagSet := newFlagSet()
	cmds, remain := findSubcommands(ctx, flagSet, plugins, args)

	logging.Debug(ctx, "command-line arguments parsed", "cmd", cmds, "args", remain)

	if err := flagSet.Parse(remain); err != nil {
		return fmt.Errorf("failed to parse the command-line arguments: %w", err)
	}

	if err := flagSet.CheckMutuallyExclusive(); err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := config.Validate(cfg, plugins); err != nil {
		return fmt.Errorf("%w", err)
	}

	// if err := config.ApplyPlugins(ctx); err != nil {
	// 	return fmt.Errorf("failed to apply config values: %w", err)
	// }
	config.ApplyPlugins(ctx)
	logging.Debug(ctx, "config parsed", "cfg", cfg, "args", flagSet.Args())

	return nil
}
