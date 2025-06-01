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
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/anttikivi/reginald/internal/tasks"
	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/anttikivi/reginald/pkg/version"
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
//
// NOTE: This struct creates some duplications as some of the functionality from
// the commands must be copied to the CLI. I still find the model where we have
// one CLI struct instead of a CLI and a separate root command much simpler to
// handle.
type CLI struct {
	UsageLine string // one-line synopsis of the program

	args                   []string          // command-line arguments after parsing
	cmd                    *Command          // command to run
	cfg                    *config.Config    // parsed config of the run
	commands               []*Command        // list of subcommands
	pluginCommands         []*Command        // commands received from plugins
	allCommands            []*Command        // internal and plugin subcommands combined
	tasks                  []*tasks.Task     // list of the task instances according to the config
	allFlags               *flags.FlagSet    // own and plugin command-line flags combined
	flags                  *flags.FlagSet    // global command-line flags
	pluginFlags            *flags.FlagSet    // plugin-wide flags
	mutuallyExclusiveFlags [][]string        // list of flag names that are marked as mutually exclusive
	plugins                []*plugins.Plugin // loaded plugins
	deferredErr            error             // error returned by the plugin shutdown not captured by the return value
}

// New creates a new CLI and returns it. It panics on errors.
func New() *CLI {
	cli := &CLI{
		UsageLine:              Name + " [--version] [-h | --help] <command> [<args>]",
		args:                   []string{},
		cmd:                    nil,
		cfg:                    nil,
		commands:               []*Command{},
		pluginCommands:         []*Command{},
		allCommands:            []*Command{},
		tasks:                  []*tasks.Task{},
		allFlags:               flags.NewFlagSet(Name, pflag.ContinueOnError),
		flags:                  flags.NewFlagSet(Name, pflag.ContinueOnError),
		pluginFlags:            flags.NewFlagSet(Name, pflag.ContinueOnError),
		mutuallyExclusiveFlags: [][]string{},
		plugins:                []*plugins.Plugin{},
		deferredErr:            nil,
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

// DeferredErr returns the error from the CLI that was set during cleaning up
// the execution.
func (c *CLI) DeferredErr() error {
	return c.deferredErr
}

// Execute executes the CLI. It parses the command-line options, finds the
// correct command to run, and executes it. An error is returned on user errors.
// The function panics if it is called with invalid program configuration.
//
//nolint:cyclop,funlen // one function to rule them all
func (c *CLI) Execute(ctx context.Context) error {
	var flagSet *flags.FlagSet

	args := os.Args
	flagSet = flags.NewFlagSet(c.flags.Name(), pflag.ContinueOnError)

	flagSet.AddFlagSet(c.flags)

	// Ignore errors for now as we want to get all of the flags from plugins
	// first.
	_ = flagSet.Parse(args)

	// TODO: Help should be implemented for all commands.
	helpSet, err := flagSet.GetBool("help")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--help': %w", err)
	}

	if helpSet {
		if err = printHelp(); err != nil {
			return fmt.Errorf("failed to print the usage info: %w", err)
		}

		return nil
	}

	versionSet, err := flagSet.GetBool("version")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--version': %w", err)
	}

	if versionSet {
		if err = printVersion(); err != nil {
			return fmt.Errorf("failed to print the version info: %w", err)
		}

		return nil
	}

	c.cfg, err = config.Parse(ctx, flagSet)
	if err != nil {
		return fmt.Errorf("failed to parse the config: %w", err)
	}

	// Initialize the output streams for user output.
	iostreams.Streams = iostreams.New(c.cfg.Quiet, c.cfg.Verbose, c.cfg.Color)

	if err := logging.Init(c.cfg.Logging); err != nil {
		return fmt.Errorf("failed to init the logger: %w", err)
	}

	logging.DebugContext(ctx, "logging initialized")
	logging.InfoContext(ctx, "running Reginald", "version", version.Version())

	if err := c.loadPlugins(ctx); err != nil {
		return fmt.Errorf("failed to resolve plugins: %w", err)
	}

	// We want to aim for a clean plugin shutdown in all cases, so the shut down
	// should be run in all cases where the plugins have been initialized.
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(ctx, plugins.DefaultShutdownTimeout)
		defer cancel()

		if err := plugins.ShutdownAll(timeoutCtx, c.plugins); err != nil {
			c.deferredErr = fmt.Errorf("failed to shut down plugins: %w", err)
		}
	}()

	if err := c.addPluginCommands(); err != nil {
		return fmt.Errorf("failed to add plugin commands: %w", err)
	}

	// Reset the arguments for parsing them when all of the plugins and
	// the correct subcommand has been loaded.
	args = os.Args

	// Make sure that `CommandLine` is not used.
	pflag.CommandLine.VisitAll(func(f *pflag.Flag) {
		panic(fmt.Sprintf("flag %q is set in the CommandLine flag set", f.Name))
	})
	logging.DebugContext(ctx, "parsing command-line arguments", "args", args)

	c.cmd, args = c.findSubcommand(ctx, args)

	if c.cmd == nil {
		flagSet = c.allFlags
	} else {
		c.cmd.mergeFlags()
		flagSet = c.cmd.Flags()
	}

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("failed to parse command-line arguments: %w", err)
	}

	c.args = flagSet.Args()

	if err := c.checkMutuallyExclusiveFlags(c.cmd); err != nil {
		return fmt.Errorf("%w", err)
	}

	valueParser := &config.ValueParser{
		Cfg:      c.cfg,
		FlagSet:  flagSet,
		Plugins:  c.plugins,
		Value:    reflect.ValueOf(c.cfg).Elem(),
		Field:    reflect.StructField{}, //nolint:exhaustruct // zero value wanted
		Plugin:   nil,
		FullName: "",
		EnvName:  config.EnvPrefix,
		EnvValue: "",
		FlagName: "",
	}
	if err := valueParser.ApplyOverrides(ctx); err != nil {
		return fmt.Errorf("failed to apply config values: %w", err)
	}

	logging.DebugContext(ctx, "full config parsed", "cfg", c.cfg)

	if err = plugins.Initialize(ctx, c.plugins, c.cfg.Plugins); err != nil {
		return fmt.Errorf("failed to initialize plugins: %w", err)
	}

	// TODO: The "root command" should do something useful like print the help.
	if c.cmd == nil {
		return nil
	}

	if err = c.setup(ctx, args); err != nil {
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

// add adds the given command to the list of commands of c and marks c as the
// CLI of cmd.
func (c *CLI) add(cmd *Command) {
	cmd.cli = c

	if cmd.mutuallyExclusiveFlags == nil {
		cmd.mutuallyExclusiveFlags = [][]string{}
	}

	cmd.mutuallyExclusiveFlags = append(cmd.mutuallyExclusiveFlags, c.mutuallyExclusiveFlags...)

	c.commands = append(c.commands, cmd)
}

// addPluginCmd adds the given command to the list of plugin commands of c and
// marks c as the CLI of cmd.
func (c *CLI) addPluginCmd(cmd *Command) error {
	if slices.ContainsFunc(c.commands, func(e *Command) bool {
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

// loadPlugins finds and executes all of the plugins in the plugins directory
// found in the configuration in c. It sets plugins in c to a slice of pointers
// to the found and executed plugins.
func (c *CLI) loadPlugins(ctx context.Context) error {
	var pluginFiles []fspath.Path

	dir := c.cfg.PluginDir

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

		info, err := os.Stat(string(path))
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

	if c.plugins, err = plugins.Load(ctx, pluginFiles); err != nil {
		return fmt.Errorf("failed to load the plugins: %w", err)
	}

	return nil
}

// addPluginCommands adds the commands from the loaded plugins to c.
func (c *CLI) addPluginCommands() error { //nolint:gocognit // no problem
	for _, plugin := range c.plugins {
		for _, cv := range plugin.PluginConfigs {
			if err := c.pluginFlags.AddPluginFlag(cv); err != nil {
				return fmt.Errorf("failed to add flag from plugin %q: %w", plugin.Name, err)
			}
		}

		for _, info := range plugin.Commands {
			cmd := &Command{ //nolint:exhaustruct // private fields have zero values
				Name:      info.Name,
				Aliases:   []string{}, // TODO: Add alias support or at least think about it.
				UsageLine: info.UsageLine,
				Setup: func(ctx context.Context, cmd *Command, _ []string) error {
					var values []rpp.ConfigValue

					if c, ok := cmd.cli.cfg.Plugins[cmd.Name].(map[string]any); ok {
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
			}

			for _, cv := range info.Configs {
				if err := cmd.Flags().AddPluginFlag(cv); err != nil {
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

	c.allCommands = append(c.allCommands, c.commands...)
	c.allCommands = append(c.allCommands, c.pluginCommands...)

	c.allFlags.AddFlagSet(c.flags)
	c.allFlags.AddFlagSet(c.pluginFlags)

	return nil
}

// checkMutuallyExclusiveFlags checks if two flags marked as mutually exclusive
// are set at the same time by the user. The function returns an error if two
// mutually exclusive flags are set.
func (c *CLI) checkMutuallyExclusiveFlags(cmd *Command) error {
	var (
		fs                     *flags.FlagSet
		mutuallyExclusiveFlags [][]string
	)

	if cmd == nil {
		fs = c.allFlags
		mutuallyExclusiveFlags = c.mutuallyExclusiveFlags
	} else {
		fs = cmd.Flags()
		mutuallyExclusiveFlags = cmd.mutuallyExclusiveFlags
	}

	if !fs.Parsed() {
		panic("checkMutuallyExclusiveFlags called before the flags were parsed")
	}

	for _, a := range mutuallyExclusiveFlags {
		var set string

		for _, s := range a {
			f := fs.Lookup(s)
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

	fs := c.allFlags
	flags := []string{}

	for len(args) >= 1 {
		if len(args) > 1 {
			args, flags = collectFlags(fs, args[1:], flags)
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
			flags,
		)
	} else {
		logging.TraceContext(ctx, "found subcommand", "cmd", cmd.Name, "args", args, "flags", flags)
	}

	args = append(args, flags...)

	return cmd, args
}

// lookup returns the command from this CLI for the given name, if any.
// Otherwise it returns nil.
func (c *CLI) lookup(name string) *Command {
	if c.allCommands == nil {
		panic("called CLI function lookup before initializing all of the list of all commands")
	}

	for _, cmd := range c.allCommands {
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

// setupCommands runs [Command.Setup] for all of the commands, starting from the
// root command. It exits on the first error it encounters.
func (c *CLI) setup(ctx context.Context, args []string) error {
	cmd := c.cmd
	cmdStack := make([]*Command, 0)
	cmdStack = append(cmdStack, cmd)

	for cmd.HasParent() {
		cmd := cmd.parent
		cmdStack = append(cmdStack, cmd)
	}

	for _, cmd := range slices.Backward(cmdStack) {
		if cmd.Setup != nil {
			if err := cmd.Setup(ctx, cmd, args); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
	}

	return nil
}
