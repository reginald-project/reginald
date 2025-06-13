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

// Package cli defines the command-line interface of Reginald.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/panichandler"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/version"
	"github.com/spf13/pflag"
)

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
)

// Global errors returned by the commands.
var (
	ErrUnknownArg = errors.New("unknown command-line argument")
)

// Errors returned by the CLI commands.
var (
	errDuplicateCommand  = errors.New("duplicate command")
	errMutuallyExclusive = errors.New("two mutually exclusive flags set at the same time")
)

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

	bootStreams := terminal.NewIO(false, false, terminal.ColorNever)
	defer bootStreams.Close()

	if err := logging.InitBootstrap(bootStreams); err != nil {
		return &ExitError{
			Code: 1,
			err:  err,
		}
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

	cfg, err := initConfig(ctx)
	if err != nil {
		return &ExitError{
			Code: 1,
			err:  err,
		}
	}

	terminal.SetStreams(terminal.NewIO(cfg.Quiet, cfg.Verbose, cfg.Color))
	defer terminal.Streams().Close()

	if err := logging.Init(cfg.Logging); err != nil {
		return &ExitError{
			Code: 1,
			err:  fmt.Errorf("failed to initialize logging: %w", err),
		}
	}

	logging.DebugContext(ctx, "logging initialized")
	logging.InfoContext(ctx, "executing Reginald", "version", version.Version())

	_, err = initPlugins(ctx, cfg)
	if err != nil {
		return &ExitError{
			Code: 1,
			err:  fmt.Errorf("failed to init plugins: %w", err),
		}
	}

	return nil
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
		logging.DebugContext(ctx, "initial flag parsing yielded an error", "err", err.Error())
	}

	cfg, err := config.Parse(ctx, flagSet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the config: %w", err)
	}

	return cfg, nil
}

func initPlugins(ctx context.Context, cfg *config.Config) (*plugin.Store, error) {
	manifests, err := plugin.Search(ctx, cfg.Directory, cfg.PluginPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to search for plugins: %w", err)
	}

	plugins := plugin.NewStore(manifests)

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
