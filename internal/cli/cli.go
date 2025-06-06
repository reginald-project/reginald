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
	"reflect"
	"slices"
	"strings"

	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/flags"
	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/anttikivi/reginald/internal/tasks"
	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/spf13/pflag"
)

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
)

// Errors returned by the CLI commands.
var (
	errDuplicateCommand  = errors.New("duplicate command")
	errMutuallyExclusive = errors.New("two mutually exclusive flags set at the same time")
)

// A CLI is the command-line interface that runs the program. It handles
// subcommands, global command-line flags, and the program execution. The "root
// command" of the CLI is represented by the CLI itself and should not a
// separate [Command] within the CLI.
type CLI struct {
	// UsageLine is the one-line synopsis of the program.
	UsageLine string

	// Cfg is the instance of [config.Config] that is used for this run.
	Cfg *config.Config

	// Plugins is a list of all of the currently loaded Plugins.
	Plugins []*plugins.Plugin

	// cmd is the subcommand that is run.
	cmd *Command

	// builtinCommands contains the subcommands of the CLI that are not from
	// plugins.
	builtinCommands []*Command // list of subcommands

	// pluginCommands contains the subcommands that are defined by plugins.
	pluginCommands []*Command

	// commands is the list of all subcommands combined.
	commands []*Command

	// TODO: tasks is a list of tasks instances according to the config.
	tasks []*tasks.Task //nolint:unused // TODO: Will be used soon.

	// flagSet is the flag set that contains all of the flags that are supported
	// by the current subcommand that is run. The flags are combined from
	// the global flags from this CLI, the global flags of the parent commands
	// of the subcommand, and the flags of the subcommand itself.
	flagSet *flags.FlagSet

	// flags is the flag set that registers the global command-line flags of
	// this CLI. It should be noted that all of the flags registered to the CLI
	// are in fact global.
	flags *flags.FlagSet

	// mutuallyExclusiveFlags is the list of flag names that are marked as
	// mutually exclusive. Each element of the slice is a slice that contains
	// the full names of the mutually exclusive flags in that group.
	mutuallyExclusiveFlags [][]string
}

// New creates a new CLI and returns it. It panics on errors.
func New() *CLI {
	cli := &CLI{
		UsageLine:              Name + " [--version] [-h | --help] <command> [<args>]",
		Cfg:                    nil,
		Plugins:                []*plugins.Plugin{},
		cmd:                    nil,
		builtinCommands:        []*Command{},
		pluginCommands:         []*Command{},
		commands:               []*Command{},
		tasks:                  []*tasks.Task{},
		flagSet:                nil,
		flags:                  flags.NewFlagSet(Name, pflag.ContinueOnError),
		mutuallyExclusiveFlags: [][]string{},
	}

	defaults := config.DefaultConfig()

	cli.flags.Bool("version", false, "print the version information and exit", "")
	cli.flags.BoolP("help", "h", false, "show the help message and exit", "")

	cli.flags.StringP(
		"config",
		"c",
		"",
		"use `<path>` as the configuration file instead of resolving it from the standard locations",
		"",
	)
	cli.flags.PathP(
		config.FlagName("Directory"),
		"C",
		defaults.Directory,
		"use `<path>` as the \"dotfiles\" directory so that Reginald looks for the config file and the files for linking from there", //nolint:lll
		"",
	)
	cli.flags.PathP(
		config.FlagName("PluginDir"),
		"p",
		defaults.PluginDir,
		"search for plugins from `<path>`",
		"",
	)

	verboseName := config.FlagName("Verbose")
	quietName := config.FlagName("Quiet")

	cli.flags.BoolP(
		verboseName,
		"v",
		defaults.Verbose,
		"make "+ProgramName+" print more output during the run",
		"",
	)
	cli.flags.BoolP(
		quietName,
		"q",
		defaults.Quiet,
		"make "+ProgramName+" print only error messages during the run",
		"",
	)
	cli.markFlagsMutuallyExclusive(quietName, verboseName)

	colorMode := defaults.Color

	cli.flags.Var(&colorMode, config.FlagName("Color"), "enable colors in the output", "")

	logName := config.FlagName("Logging.Enabled")
	noLogName := config.InvertedFlagName("Logging.Enabled")
	hiddenLogFlag := logName

	cli.flags.Bool(logName, defaults.Logging.Enabled, "enable logging", "")
	cli.flags.Bool(noLogName, !defaults.Logging.Enabled, "disable logging", "")
	cli.markFlagsMutuallyExclusive(logName, noLogName)

	if err := cli.flags.MarkHidden(hiddenLogFlag); err != nil {
		panic(fmt.Sprintf("failed to mark --%s hidden: %v", hiddenLogFlag, err))
	}

	cli.add(NewAttend())

	return cli
}

// Execute executes the CLI. It parses the command-line options, finds the
// correct command to run, and executes it. An error is returned on user errors.
// The function panics if it is called with invalid program configuration.
func (c *CLI) Execute(ctx context.Context) error {
	if err := c.addPluginCommands(); err != nil {
		return fmt.Errorf("failed to add plugin commands: %w", err)
	}

	c.commands = append(c.commands, c.builtinCommands...)
	c.commands = append(c.commands, c.pluginCommands...)

	if err := c.parseArgs(ctx); err != nil {
		return fmt.Errorf("failed to parse command-line arguments: %w", err)
	}

	ok, err := c.shortCircuitPlugin()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if !ok {
		return nil
	}

	valueParser := &config.ValueParser{
		Cfg:      c.Cfg,
		FlagSet:  c.flagSet,
		Plugins:  c.Plugins,
		Value:    reflect.ValueOf(c.Cfg).Elem(),
		Field:    reflect.StructField{}, //nolint:exhaustruct // zero value wanted
		Plugin:   nil,
		FullName: "",
		EnvName:  config.EnvPrefix,
		EnvValue: "",
		FlagName: "",
	}
	if err = valueParser.ApplyOverrides(ctx); err != nil {
		return fmt.Errorf("failed to apply config values: %w", err)
	}

	logging.DebugContext(ctx, "full config parsed", "cfg", c.Cfg)

	if err = plugins.Initialize(ctx, c.Plugins, c.Cfg.Plugins); err != nil {
		return fmt.Errorf("failed to initialize plugins: %w", err)
	}

	// TODO: The "root command" should do something useful like print the help.
	if c.cmd == nil {
		return nil
	}

	if err = c.setup(ctx); err != nil {
		return fmt.Errorf("%w", err)
	}

	if c.cmd.Run == nil {
		panic(fmt.Sprintf("command %q has no Run function", c.cmd.Name))
	}

	if err := c.cmd.Run(ctx, c.cmd); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// Initialize initializes the CLI by checking if the "--help" or "--version"
// flags are set without any other arguments and by doing the first round of
// configuration parsing. As the program should not continue its execution if
// the "--help" or "--version" flags are invoked here, Initialize return false
// if the execution should not continue. Otherwise, the first return value is
// true.
func (c *CLI) Initialize(ctx context.Context) (bool, error) {
	// Create a temporary flag set for the initialization.
	flagSet := flags.NewFlagSet(c.flags.Name(), pflag.ContinueOnError)

	flagSet.AddFlagSet(c.flags)

	// Ignore errors for now as we want to get all of the flags from plugins
	// first.
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		logging.DebugContext(ctx, "initial flag parsing yielded an error", "err", err.Error())
	}

	var ok bool

	ok, err = c.shortCircuit(flagSet)
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	if !ok {
		return false, nil
	}

	c.Cfg, err = config.Parse(ctx, flagSet)
	if err != nil {
		return false, fmt.Errorf("failed to parse the config: %w", err)
	}

	return true, nil
}

// LoadPlugins finds and executes all of the plugins in the plugins directory
// found in the configuration in c. It sets plugins in c to a slice of pointers
// to the found and executed plugins.
func (c *CLI) LoadPlugins(ctx context.Context) error {
	var pluginFiles []fspath.Path

	dir := c.Cfg.PluginDir

	entries, err := dir.ReadDir()
	if err != nil {
		return fmt.Errorf("failed to read plugins directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		path := dir.Join(entry.Name())

		if entry.IsDir() {
			logging.TraceContext(ctx, "plugin file is a directory", "path", path)

			continue
		}

		var info os.FileInfo

		info, err = os.Stat(string(path))
		if err != nil {
			return fmt.Errorf("failed to check the file info for %s: %w", path, err)
		}

		if info.Mode()&0o111 == 0 {
			logging.TraceContext(ctx, "plugin file is not executable", "path", path)

			continue
		}

		if strings.HasPrefix(entry.Name(), Name+"-") {
			pluginFiles = append(pluginFiles, path)
		}
	}

	logging.DebugContext(ctx, "performed the plugin lookup", "plugins", pluginFiles)

	if c.Plugins, err = plugins.Load(ctx, pluginFiles); err != nil {
		return fmt.Errorf("failed to load the plugins: %w", err)
	}

	return nil
}

// add adds the given command to the list of commands of c and marks c as the
// CLI of cmd.
func (c *CLI) add(cmd *Command) {
	cmd.cli = c

	if cmd.mutuallyExclusiveFlags == nil {
		cmd.mutuallyExclusiveFlags = [][]string{}
	}

	cmd.mutuallyExclusiveFlags = append(cmd.mutuallyExclusiveFlags, c.mutuallyExclusiveFlags...)
	cmd.plugin = nil // ensure that internal commands do not have a plugin

	c.builtinCommands = append(c.builtinCommands, cmd)
}

// addPluginCmd adds the given command to the list of plugin commands of c and
// marks c as the CLI of cmd.
func (c *CLI) addPluginCmd(cmd *Command) error {
	if slices.ContainsFunc(c.builtinCommands, func(e *Command) bool {
		return e.Name == cmd.Name
	}) {
		return fmt.Errorf("%w: %s", errDuplicateCommand, cmd.Name)
	}

	cmd.cli = c

	if cmd.mutuallyExclusiveFlags == nil {
		cmd.mutuallyExclusiveFlags = [][]string{}
	}

	cmd.mutuallyExclusiveFlags = append(cmd.mutuallyExclusiveFlags, c.mutuallyExclusiveFlags...)

	c.pluginCommands = append(c.pluginCommands, cmd)

	return nil
}

// addPluginCommands adds the commands from the loaded plugins to c.
func (c *CLI) addPluginCommands() error { //nolint:gocognit // no problem
	for _, plugin := range c.Plugins {
		if err := c.registerPluginFlags(plugin); err != nil {
			return fmt.Errorf("failed to add plugin-wide flags from %q: %w", plugin.Name, err)
		}

		for _, info := range plugin.Commands {
			cmd := &Command{ //nolint:exhaustruct // private fields have zero values
				Name:      info.Name,
				Aliases:   []string{}, // TODO: Add alias support or at least think about it.
				UsageLine: info.UsageLine,
				Setup: func(ctx context.Context, cmd *Command, _ []string) error {
					var values []rpp.ConfigValue

					if c, ok := cmd.cli.Cfg.Plugins[cmd.Name].(map[string]any); ok {
						for k, v := range c {
							cfgVal, err := rpp.NewConfigValue(k, v)
							if err != nil {
								return fmt.Errorf("%w", err)
							}

							values = append(values, cfgVal)
						}
					}

					// TODO: Pass in the args.
					if err := plugin.SetupCmd(ctx, cmd.Name, values); err != nil {
						return fmt.Errorf(
							"failed to run setup for command %q from plugin %q: %w",
							cmd.Name,
							plugin.Name,
							err,
						)
					}

					return nil
				},
				Run: func(ctx context.Context, cmd *Command) error {
					if err := plugin.RunCmd(ctx, cmd.Name); err != nil {
						return fmt.Errorf(
							"failed to run command %q from plugin %q: %w",
							cmd.Name,
							plugin.Name,
							err,
						)
					}

					return nil
				},
				plugin: plugin,
			}

			for _, cv := range info.Configs {
				if err := cmd.Flags().AddPluginFlag(&cv); err != nil {
					return fmt.Errorf(
						"failed to add flag from plugin %q and command %q: %w",
						plugin.Name,
						info.Name,
						err,
					)
				}
			}

			if err := c.addPluginCmd(cmd); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
	}

	return nil
}

// registerPluginFlags adds the plugin-wide flags from the configuration of
// the given plugin to flags in c. Plugin-wide flags are treated as global
// flags.
func (c *CLI) registerPluginFlags(plugin *plugins.Plugin) error {
	for _, cv := range plugin.PluginConfigs {
		if err := c.flags.AddPluginFlag(&cv); err != nil {
			return fmt.Errorf("failed to add flag from plugin %q: %w", plugin.Name, err)
		}
	}

	return nil
}

// checkMutuallyExclusiveFlags checks if two flags marked as mutually exclusive
// are set at the same time by the user. The function returns an error if two
// mutually exclusive flags are set.
func (c *CLI) checkMutuallyExclusiveFlags(cmd *Command) error {
	mutuallyExclusiveFlags := [][]string{}
	mutuallyExclusiveFlags = append(mutuallyExclusiveFlags, c.mutuallyExclusiveFlags...)

	for cmd != nil {
		mutuallyExclusiveFlags = append(mutuallyExclusiveFlags, cmd.mutuallyExclusiveFlags...)

		if cmd.HasParent() {
			cmd = cmd.parent
		} else {
			cmd = nil
		}
	}

	if !c.flagSet.Parsed() {
		panic("checkMutuallyExclusiveFlags called before the flags were parsed")
	}

	for _, a := range mutuallyExclusiveFlags {
		var set string

		for _, s := range a {
			f := c.flagSet.Lookup(s)
			if f == nil {
				panic("nil flag in the set of mutually exclusive flags: " + s)
			}

			if f.Changed {
				if set != "" {
					return fmt.Errorf(
						"%w: --%s and --%s (or their shorthands)",
						errMutuallyExclusive,
						set,
						s,
					)
				}

				set = s
			}
		}
	}

	return nil
}

// findSubcommand finds the subcommand to run from the command tree starting at
// c. It returns the final command and the command-line arguments, and
// command-line flags. If no subcommand is found (i.e. the root command should
// be run), this function returns nil as the first return value.
func (c *CLI) findSubcommand(ctx context.Context, args []string) (*Command, []string) {
	if len(args) <= 1 {
		return nil, args
	}

	var cmd *Command

	fs := c.flags
	flagsFound := []string{}

	for len(args) >= 1 {
		if len(args) > 1 {
			args, flagsFound = collectFlags(fs, args[1:], flagsFound)
		}

		if len(args) >= 1 {
			var next *Command

			if cmd == nil {
				next = c.lookup(args[0])
			} else {
				next = cmd.Lookup(args[0])
			}

			if next == nil {
				break
			}

			cmd = next
			fs = c.mergedFlagSet(cmd)
		}
	}

	if len(args) > 0 && cmd != nil && args[0] == cmd.Name {
		args = args[1:]
	}

	if cmd == nil {
		logging.TraceContext(
			ctx,
			"no command found",
			"cmd",
			os.Args[0],
			"args",
			args,
			"flags",
			flagsFound,
		)
	} else {
		logging.TraceContext(ctx, "found subcommand", "cmd", cmd.Name, "args", args, "flags", flagsFound)
	}

	args = append(args, flagsFound...)

	return cmd, args
}

// lookup returns the command from this CLI for the given name, if any.
// Otherwise it returns nil.
func (c *CLI) lookup(name string) *Command {
	if c.commands == nil {
		panic("called CLI function lookup before initializing all of the list of all commands")
	}

	for _, cmd := range c.commands {
		if strings.EqualFold(cmd.Name, name) {
			return cmd
		}

		for _, a := range cmd.Aliases {
			if strings.EqualFold(a, name) {
				return cmd
			}
		}
	}

	return nil
}

// markFlagsMutuallyExclusive marks two or more flags as mutually exclusive so
// that the program returns an error if the user tries to set them at the same
// time.
func (c *CLI) markFlagsMutuallyExclusive(a ...string) {
	if len(a) < 2 { //nolint:mnd // obvious
		panic("only one flag cannot be marked as mutually exclusive")
	}

	for _, s := range a {
		if f := c.flags.Lookup(s); f == nil {
			panic(fmt.Sprintf("failed to find flag %q while marking it as mutually exclusive", s))
		}
	}

	if c.mutuallyExclusiveFlags == nil {
		panic(
			"mutually exclusive flags of the CLI should have been initialized when the struct was created",
		)
	}

	c.mutuallyExclusiveFlags = append(c.mutuallyExclusiveFlags, a)
}

// mergedFlagSet allocates a new flag set and merges the command-line flags from
// the given command, the global flags from its parents, and the flags from c to
// it.
func (c *CLI) mergedFlagSet(cmd *Command) *flags.FlagSet {
	flagSet := flags.NewFlagSet(c.flags.Name(), pflag.ContinueOnError)

	flagSet.AddFlagSet(c.flags)

	if cmd != nil {
		cmd.mergeFlags()
		flagSet.AddFlagSet(cmd.Flags())
	}

	return flagSet
}

// parseArgs parses the command-line arguments and sets the main flag set and
// the command to run to c according to them. After the function run, flagSet in
// c will always have at least all of the global flags from c and additionally
// the effective global flags and the local flags for the subcommand that was
// found, if any. However, cmd in c might be left nil if no subcommand was found
// from the command-line arguments.
func (c *CLI) parseArgs(ctx context.Context) error {
	// There is no need to remove the first element of the arguments slice as
	// findSubcommand takes care of that.
	args := os.Args

	// Make sure that `CommandLine` is not used.
	pflag.CommandLine.VisitAll(func(f *pflag.Flag) {
		panic(fmt.Sprintf("flag %q is set in the CommandLine flag set", f.Name))
	})
	logging.DebugContext(ctx, "parsing command-line arguments", "args", args)

	c.cmd, args = c.findSubcommand(ctx, args)
	c.flagSet = c.mergedFlagSet(c.cmd)

	if err := c.flagSet.Parse(args); err != nil {
		return fmt.Errorf("failed to parse command-line arguments: %w", err)
	}

	logging.DebugContext(ctx, "command-line arguments parsed", "args", c.flagSet.Args())

	if err := c.checkMutuallyExclusiveFlags(c.cmd); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// setupCommands runs [Command.Setup] for all of the commands, starting from the
// root command. It exits on the first error it encounters.
func (c *CLI) setup(ctx context.Context) error {
	cmd := c.cmd
	cmdStack := make([]*Command, 0)
	cmdStack = append(cmdStack, cmd)

	for cmd.HasParent() {
		cmd = cmd.parent
		cmdStack = append(cmdStack, cmd)
	}

	for _, cmd := range slices.Backward(cmdStack) {
		if cmd.Setup != nil {
			if err := cmd.Setup(ctx, cmd, c.flagSet.Args()); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
	}

	return nil
}

// shortCircuit checks if the program run can be short-circuited. This means
// that the program was run using the "--help" or the "--version" flag and there
// are no other arguments left after the initial parsing of the command-line
// flags. If there are more arguments, the meaning of the flags might change
// and, thus, the program should continue running.
//
// If the program should short-circuit, shortCircuit returns false. Otherwise,
// it returns true and the execution should continue.
func (c *CLI) shortCircuit(flagSet *flags.FlagSet) (bool, error) {
	if len(flagSet.Args()) > 0 {
		return true, nil
	}

	helpSet, err := flagSet.GetBool("help")
	if err != nil {
		return false, fmt.Errorf(
			"failed to get the value for command-line option '--help': %w",
			err,
		)
	}

	if helpSet {
		if err = printHelp(c); err != nil {
			return false, fmt.Errorf("failed to print the usage info: %w", err)
		}

		return false, nil
	}

	versionSet, err := flagSet.GetBool("version")
	if err != nil {
		return false, fmt.Errorf(
			"failed to get the value for command-line option '--version': %w",
			err,
		)
	}

	if versionSet {
		if err = printVersion(nil); err != nil {
			return false, fmt.Errorf("failed to print the version info: %w", err)
		}

		return false, nil
	}

	return true, nil
}

// shortCircuitPlugin checks if the program should display the help or
// the version info after the plugins have loaded. If either of those options
// are set, the function does the operation for the currently selected command.
// For help this means that the subcommands help message is displayed and for
// version that the version information of the program is displayed if
// the subcommand is not from a plugin. If it is from a plugin, that plugin's
// version is displayed instead.
//
// If the program should short-circuit, shortCircuitPlugin returns false.
// Otherwise, it returns true and the execution should continue.
func (c *CLI) shortCircuitPlugin() (bool, error) {
	if len(c.flagSet.Args()) > 0 {
		return true, nil
	}

	// TODO: Help should be implemented for all commands.
	helpSet, err := c.flagSet.GetBool("help")
	if err != nil {
		return false, fmt.Errorf(
			"failed to get the value for command-line option '--help': %w",
			err,
		)
	}

	if helpSet {
		if err = printHelp(c); err != nil {
			return false, fmt.Errorf("failed to print the usage info: %w", err)
		}

		return false, nil
	}

	versionSet, err := c.flagSet.GetBool("version")
	if err != nil {
		return false, fmt.Errorf(
			"failed to get the value for command-line option '--version': %w",
			err,
		)
	}

	if versionSet {
		if err = printVersion(c.cmd); err != nil {
			return false, fmt.Errorf("failed to print the version info: %w", err)
		}

		return false, nil
	}

	return true, nil
}
