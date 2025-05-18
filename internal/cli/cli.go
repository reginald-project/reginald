// Package cli defines the command-line interface of Reginald.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anttikivi/go-semver"
	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/spf13/pflag"
	"golang.org/x/term"
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
	UsageLine string          // one-line synopsis of the program
	Version   *semver.Version // version number of the program

	cfg                    *config.Config    // parsed config of the run
	commands               []*Command        // list of subcommands
	flags                  *pflag.FlagSet    // global command-line flags
	mutuallyExclusiveFlags [][]string        // list of flag names that are marked as mutually exclusive
	plugins                []*plugins.Plugin // loaded plugins
}

// Program-related constants.
const (
	ProgramName = "Reginald" // canonical name for the program
	Name        = "reginald" // name of the command that's run
)

// errMutuallyExclusive is returned when the user sets two mutually exclusive
// flags from the same group at the same time.
var errMutuallyExclusive = errors.New("two mutually exclusive flags set at the same time")

// New creates a new CLI and returns it. It panics on errors.
func New(v string) *CLI {
	cli := &CLI{
		UsageLine:              Name + " [--version] [-h | --help] <command> [<args>]",
		Version:                semver.MustParse(v),
		cfg:                    nil,
		commands:               []*Command{},
		flags:                  pflag.NewFlagSet(Name, pflag.ContinueOnError),
		mutuallyExclusiveFlags: [][]string{},
		plugins:                []*plugins.Plugin{},
	}

	cli.flags.Bool("version", false, "print the version information and exit")
	cli.flags.BoolP("help", "h", false, "show the help message and exit")

	pwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get the current working directory: %v", err))
	}

	cli.flags.StringP(
		"directory",
		"C",
		pwd,
		fmt.Sprintf(
			"run as if %s was started in `<path>` instead of the current working directory",
			ProgramName,
		),
	)
	cli.flags.StringP(
		"config",
		"c",
		"",
		"use `<path>` as the configuration file instead of resolving it from the standard locations",
	)

	cli.flags.BoolP("verbose", "v", false, "make "+ProgramName+" print more output during the run")
	cli.flags.BoolP(
		"quiet",
		"q",
		false,
		"make "+ProgramName+" print only error messages during the run",
	)
	cli.markFlagsMutuallyExclusive("quiet", "verbose")

	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))

	cli.flags.Bool("color", isTerminal, "enable colors in the output")
	cli.flags.Bool("no-color", !isTerminal, "disable colors in the output")
	cli.markFlagsMutuallyExclusive("color", "no-color")

	if err := cli.flags.MarkHidden("no-color"); err != nil {
		panic(fmt.Sprintf("failed to mark --no-color hidden: %v", err))
	}

	cli.flags.Bool("logging", false, "enable logging")
	cli.flags.Bool("no-logging", false, "disable logging")
	cli.markFlagsMutuallyExclusive("logging", "no-logging")

	if err := cli.flags.MarkHidden("no-logging"); err != nil {
		panic(fmt.Sprintf("failed to mark --no-logging hidden: %v", err))
	}

	cli.add(NewApply())

	return cli
}

// Execute executes the CLI. It parses the command-line options, finds the
// correct command to run, and executes it. An error is returned on user errors.
// The function panics if it is called with invalid program configuration.
func (c *CLI) Execute(ctx context.Context) error {
	args := os.Args

	// Matches merging flags for commands.
	c.flags.AddFlagSet(pflag.CommandLine)
	slog.Debug("starting to parse the command-line arguments", "args", args)

	cmd, args := c.findSubcommand(args)

	var flagSet *pflag.FlagSet

	if cmd == nil {
		flagSet = c.flags
	} else {
		cmd.mergeFlags()
		flagSet = cmd.Flags()
	}

	// TODO: Move checking the errors to a later time when the plugin system is
	// in place. It should be possible to define subcommands and flags for them
	// using the plugins.
	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("failed to parse command-line arguments: %w", err)
	}

	help, err := flagSet.GetBool("help")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--help': %w", err)
	}

	if help {
		fmt.Fprintln(os.Stdout, "HELP MESSAGE")

		return nil
	}

	version, err := flagSet.GetBool("version")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--version': %w", err)
	}

	if version {
		fmt.Fprintf(os.Stdout, "%s %v\n", ProgramName, c.Version)

		return nil
	}

	if err = c.checkMutuallyExclusiveFlags(cmd); err != nil {
		return fmt.Errorf("%w", err)
	}

	c.cfg, err = c.parseConfig(flagSet)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	// Initialize the output streams for user output.
	iostreams.Streams = iostreams.New(c.cfg.Quiet, c.cfg.Verbose, c.cfg.Color)

	if err := logging.Init(c.cfg.Logging); err != nil {
		return fmt.Errorf("failed to init the logger: %w", err)
	}

	slog.Debug("logging initialized")

	if err = c.loadPlugins(ctx); err != nil {
		return fmt.Errorf("failed to resolve plugins: %w", err)
	}

	if err = c.run(cmd, args); err != nil {
		return fmt.Errorf("%w", err)
	}

	// End by shutting down the plugins.
	timeoutCtx, cancel := context.WithTimeout(ctx, plugins.DefaultPluginShutdownTimeout)
	defer cancel()

	if err = plugins.ShutdownAll(timeoutCtx, c.plugins); err != nil {
		return fmt.Errorf("failed to shut down plugins: %w", err)
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

// checkMutuallyExclusiveFlags checks if two flags marked as mutually exclusive
// are set at the same time by the user. The function returns an error if two
// mutually exclusive flags are set.
func (c *CLI) checkMutuallyExclusiveFlags(cmd *Command) error {
	var (
		fs                     *pflag.FlagSet
		mutuallyExclusiveFlags [][]string
	)

	if cmd == nil {
		fs = c.flags
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
func (c *CLI) findSubcommand(args []string) (*Command, []string) {
	if len(args) <= 1 {
		return nil, args
	}

	var cmd *Command

	fs := c.flags
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

	if len(args) > 0 && cmd != nil && args[0] == cmd.Name() {
		args = args[1:]
	}

	if cmd == nil {
		slog.Debug("no command found", "cmd", os.Args[0], "args", args, "flags", flags)
	} else {
		slog.Debug("found subcommand", "cmd", cmd.Name(), "args", args, "flags", flags)
	}

	args = append(args, flags...)

	return cmd, args
}

// lookup returns the command from this CLI for the given name, if any.
// Otherwise it returns nil.
func (c *CLI) lookup(name string) *Command {
	for _, cmd := range c.commands {
		// TODO: Check for aliases.
		if cmd.Name() == name {
			return cmd
		}
	}

	return nil
}

// markFlagsMutuallyExclusive marks two or more flags as mutually exclusive so
// that the program returns an error if the user tries to set them at the same
// time.
func (c *CLI) markFlagsMutuallyExclusive(a ...string) {
	if len(a) < 2 { //nolint:mnd
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

// parseConfig parses the configuration from the configuration files,
// environment variables, and command-line flags. It returns a pointer to the
// configuration and any errors encountered.
func (c *CLI) parseConfig(fs *pflag.FlagSet) (*config.Config, error) {
	slog.Info("parsing config")

	cfg, err := config.Parse(fs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the config: %w", err)
	}

	slog.Info("config parsed", "config", cfg)

	return cfg, nil
}

// loadPlugins finds and executes all of the plugins in the plugins directory
// found in the configuration in c. It sets plugins in c to a slice of pointers
// to the found and executed plugins.
func (c *CLI) loadPlugins(ctx context.Context) error {
	var pluginFiles []string

	entries, err := os.ReadDir(c.cfg.PluginDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory %s: %w", c.cfg.PluginDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !entry.Type().IsRegular() {
			continue
		}

		path := filepath.Join(c.cfg.PluginDir, entry.Name())

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("failed to check the file info for %s: %w", path, err)
		}

		if info.Mode()&0o111 == 0 {
			slog.Debug("plugin file is not executable", "path", path)

			continue
		}

		if strings.HasPrefix(entry.Name(), Name+"-") {
			pluginFiles = append(pluginFiles, path)
		}
	}

	slog.Debug("performed the plugin lookup", "plugins", pluginFiles)

	if c.plugins, err = plugins.Load(ctx, pluginFiles); err != nil {
		return fmt.Errorf("failed to load the plugins: %w", err)
	}

	return nil
}

// run runs the setup and execution of the resolved command.
func (c *CLI) run(cmd *Command, args []string) error {
	if cmd == nil {
		return nil
	}

	if err := setup(cmd, cmd, args); err != nil {
		return fmt.Errorf("%w", err)
	}

	if err := cmd.Run(cmd, args); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// setup runs [Command.Setup] for all of the commands, starting from the root
// command. It exits on the first error it encounters.
func setup(c, subcmd *Command, args []string) error {
	if c.HasParent() {
		if err := setup(c.parent, subcmd, args); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	if err := c.Setup(c, subcmd, args); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
