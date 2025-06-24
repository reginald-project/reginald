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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/debugging"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/log"
	"github.com/reginald-project/reginald/internal/log/logger"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
	"github.com/spf13/pflag"
)

// errInvalidArgs is the error returned when the arguments are invalid.
var errInvalidArgs = errors.New("invalid arguments")

// addFlags adds the flags from the given command to the flag set.
func addFlags(flagSet *flags.FlagSet, cmd *plugin.Command) error {
	for _, cfg := range cmd.Config {
		if err := flagSet.AddPluginFlag(&cfg); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	return nil
}

// initialize initializes the program run by creating the logger and output
// streams, loading the plugin information, and parsing the command-line
// arguments.
func initialize(ctx context.Context) (*runInfo, error) {
	if err := logger.InitBootstrap(); err != nil {
		return nil, fmt.Errorf("failed to init bootstrap logger: %w", err)
	}

	log.Debug(ctx, "bootstrap logger initialized")
	log.Info(
		ctx,
		"initializing Reginald",
		"version",
		version.Version(),
		"commit",
		version.Revision(),
	)

	strictErr := &strictError{
		errs: nil,
	}

	cfg, err := initConfig(ctx)
	if err != nil {
		var fileErr *config.FileError
		if !errors.As(err, &fileErr) {
			return nil, &ExitError{
				Code: 1,
				err:  err,
			}
		}

		strictErr.errs = append(strictErr.errs, fileErr)
	}

	if err = initOut(ctx, cfg); err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  err,
		}
	}

	// Just to be sure.
	debugging.SetDebug(cfg.Debug)

	log.Info(ctx, "executing Reginald", "version", version.Version())

	var pathErrs plugin.PathErrors

	store, err := initPlugins(ctx, cfg)
	if err != nil {
		if !errors.As(err, &pathErrs) {
			return nil, &ExitError{
				Code: 1,
				err:  err,
			}
		}

		strictErr.errs = append(strictErr.errs, err)
	}

	if len(strictErr.errs) > 0 && cfg.Strict {
		return nil, &ExitError{
			Code: 1,
			err:  strictErr,
		}
	}

	info := &runInfo{
		cmd:     nil,
		cfg:     cfg,
		store:   store,
		args:    nil,
		help:    false,
		version: false,
	}

	if err = parseArgs(ctx, info); err != nil {
		return nil, &ExitError{
			Code: 1,
			err:  err,
		}
	}

	// Best to skip printing if "--help" or "--version" was used.
	if info.help || info.version {
		return info, nil
	}

	switch {
	case cfg.HasFile():
		// no-op
	case !cfg.Interactive:
		terminal.Warnln("No config file was found")
	case !terminal.Confirm(ctx, "No config file was found. Continue?", true):
		return nil, &SuccessError{}
	}

	switch {
	case pathErrs == nil:
		// no-op
	case !cfg.Interactive:
		terminal.Warnln("Plugin directory not found")
	case !terminal.Confirm(ctx, "Plugin directory not found. Continue?", true):
		return nil, &SuccessError{}
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
		case strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "--") && !hasShortNoOptDefVal(s[len(s)-1:], flagSet):
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

// hasShortNoOptDefVal checks if the flag for the given shorthand has a
// NoOptDefVal set.
func hasShortNoOptDefVal(name string, fs *flags.FlagSet) bool {
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
		log.Warn(ctx, "initial flag parsing yielded an error", "err", err.Error())
	}

	var fileErr *config.FileError

	cfg, err := config.Parse(ctx, flagSet)
	if err != nil {
		if !errors.As(err, &fileErr) {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	log.Info(ctx, "built-in config values parsed and applied", "cfg", cfg)

	if fileErr != nil {
		return cfg, fileErr
	}

	return cfg, nil
}

// initOut initializes the output streams and the logging for the program.
func initOut(ctx context.Context, cfg *config.Config) error {
	terminal.Default().Init(cfg.Quiet, cfg.Verbose, cfg.Interactive, cfg.Color)

	if err := logger.Init(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}

	log.Debug(ctx, "logging initialized")

	return nil
}

// initPlugins looks up the plugin manifests and creates a new plugin store
// instance from them.
func initPlugins(ctx context.Context, cfg *config.Config) (*plugin.Store, error) {
	var pathErrs plugin.PathErrors

	manifests, err := plugin.Search(ctx, cfg.Directory, cfg.PluginPaths)
	if err != nil {
		if !errors.As(err, &pathErrs) {
			return nil, fmt.Errorf("failed to search for plugins: %w", err)
		}

		log.Error(ctx, "failed to search for plugins", "err", pathErrs)
	}

	store := plugin.NewStore(manifests)

	log.Debug(ctx, "created plugins", "store", store)

	if len(pathErrs) > 0 {
		return store, pathErrs
	}

	return store, nil
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
	flagSet.Bool("strict", defaults.Strict, "enable strict mode", "")
	flagSet.MarkMutuallyExclusive("interactive", "strict")

	colorMode := defaults.Color

	flagSet.Var(&colorMode, config.FlagName("Color"), "set the `<mode>` for color output", "")

	logName := config.FlagName("Logging.Enabled")
	noLogName := config.InvertedFlagName("Logging.Enabled")
	hiddenLogFlag := logName

	flagSet.Bool(logName, defaults.Logging.Enabled, "enable logging", "")
	flagSet.Bool(noLogName, !defaults.Logging.Enabled, "disable logging", "")
	flagSet.MarkMutuallyExclusive(logName, noLogName)

	if err := flagSet.MarkHidden(hiddenLogFlag); err != nil {
		panic(fmt.Sprintf("failed to mark --%s hidden: %v", hiddenLogFlag, err))
	}

	debugFlagSet := debugging.FlagSet()

	flagSet.AddFlagSet(debugFlagSet)

	return flagSet
}

// parseArgs parses the command-line arguments and modifies the run info and
// the config in it according to them. The function creates a new flag set for
// the root command, finds the subcommand for the command-line arguments, and
// sets the flags from the subcommand to the flag set. The remaining arguments
// are stored in the run info.
func parseArgs(ctx context.Context, info *runInfo) error {
	// There is no need to remove the first element of the arguments slice as
	// findSubcommand takes care of that.
	info.args = os.Args

	// Make sure that `CommandLine` is not used.
	pflag.CommandLine.VisitAll(func(f *pflag.Flag) {
		panic(fmt.Sprintf("flag %q is set in the CommandLine flag set", f.Name))
	})
	log.Debug(ctx, "parsing command-line arguments", "args", info.args)

	flagSet := newFlagSet()
	if err := parseCommands(ctx, flagSet, info); err != nil {
		return err
	}

	log.Debug(ctx, "commands parsed", "cmd", info.cmd, "args", info.args)

	if err := flagSet.Parse(info.args); err != nil {
		return fmt.Errorf("failed to parse the command-line arguments: %w", err)
	}

	info.args = flagSet.Args()

	if err := flagSet.CheckMutuallyExclusive(); err != nil {
		return fmt.Errorf("%w", err)
	}

	log.Trace(ctx, "flags parsed", "args", info.args)

	if err := validateArgs(info); err != nil {
		return err
	}

	if err := config.Validate(info.cfg, info.store); err != nil {
		return fmt.Errorf("%w", err)
	}

	// if err := config.ApplyPlugins(ctx); err != nil {
	// 	return fmt.Errorf("failed to apply config values: %w", err)
	// }
	config.ApplyPlugins(ctx)
	log.Info(ctx, "full config parsed and applied", "cfg", info.cfg, "args", info.args)

	var err error

	if info.help, err = flagSet.GetBool("help"); err != nil {
		return fmt.Errorf("failed to get value for --help: %w", err)
	}

	if info.version, err = flagSet.GetBool("version"); err != nil {
		return fmt.Errorf("failed to get value for --version: %w", err)
	}

	if info.cmd == nil && !info.version {
		info.help = true
	}

	if info.cmd != nil && info.cmd.Name == "version" {
		info.version = true
	}

	return nil
}

// parseCommands finds the subcommand to run from the command tree starting at
// root command. It sets the arguments and the command to run in the run info.
// The function adds the flags from the subcommand to the flag set. The flag set
// is modified in-place.
func parseCommands(ctx context.Context, flagSet *flags.FlagSet, info *runInfo) error {
	if len(info.args) == 0 {
		panic("no command-line arguments")
	}

	if len(info.args) == 1 {
		info.args = info.args[1:]

		return nil
	}

	flagsFound := []string{}
	info.args = info.args[1:]

	for len(info.args) >= 1 {
		log.Trace(ctx, "checking args", "cmd", info.cmd, "args", info.args, "flags", flagsFound)

		if len(info.args) > 1 {
			info.args, flagsFound = collectFlags(flagSet, info.args, flagsFound)

			log.Trace(ctx, "collected flags", "args", info.args, "flags", flagsFound)
		}

		if len(info.args) >= 1 {
			next := info.store.Command(info.cmd, info.args[0])

			log.Trace(ctx, "next command", "cmd", next)

			if next == nil {
				break
			}

			info.cmd = next
			info.args = info.args[1:]

			if err := addFlags(flagSet, info.cmd); err != nil {
				// TODO: This should be handled better.
				log.Error(ctx, "failed to add flags from commands", "err", err)

				return err
			}
		}
	}

	info.args = append(info.args, flagsFound...)

	return nil
}

// validateArgs validates the command-line arguments according to
// the specifications given by the plugins.
func validateArgs(info *runInfo) error {
	if info.cmd == nil {
		if len(info.args) > 0 {
			return fmt.Errorf("%w: unknown command: %q", errInvalidArgs, info.args[0])
		}

		return nil
	}

	spec := info.cmd.Args
	if spec == nil {
		if len(info.args) > 0 {
			return fmt.Errorf("%w: unknown argument: %q", errInvalidArgs, info.args[0])
		}

		return nil
	}

	if len(info.args) < spec.Min {
		return fmt.Errorf(
			"%w: command %q requires at least %d argument(s), got %d",
			errInvalidArgs,
			info.cmd.Name,
			spec.Min,
			len(info.args),
		)
	}

	if spec.Max != -1 && len(info.args) > spec.Max {
		return fmt.Errorf(
			"%w: command %q accepts at most %d argument(s), got %d",
			errInvalidArgs,
			info.cmd.Name,
			spec.Max,
			len(info.args),
		)
	}

	return nil
}
