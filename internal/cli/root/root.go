// Package root defines the root command for Reginald.
package root

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anttikivi/go-semver"
	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/cli/apply"
	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugin"
)

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New(version string) (*cli.RootCommand, error) {
	c := &cli.RootCommand{ //nolint:varnamelen
		Command: cli.Command{
			UsageLine: name + " [--version] [-h | --help] <command> [<args>]",
			Setup:     setup,
			Run:       run,
		},
		Version: semver.MustParse(version),
	}

	c.GlobalFlags().Bool("version", false, "print the version information and exit")
	c.GlobalFlags().BoolP("help", "h", false, "show the help message and exit")

	pwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get the current working directory: %w", err)
	}

	c.GlobalFlags().StringP(
		"directory",
		"C",
		pwd,
		fmt.Sprintf(
			"run as if %s was started in `<path>` instead of the current working directory",
			cli.ProgramName,
		),
	)
	c.GlobalFlags().StringP(
		"config",
		"c",
		"",
		"use `<path>` as the configuration file instead of resolving it from the standard locations",
	)

	c.GlobalFlags().BoolP(
		"verbose",
		"v",
		false,
		"make "+cli.ProgramName+" print more output during the run",
	)
	c.GlobalFlags().BoolP(
		"quiet",
		"q",
		false,
		"make "+cli.ProgramName+" print only error messages during the run",
	)
	c.MarkFlagsMutuallyExclusive("quiet", "verbose")

	c.GlobalFlags().Bool("logging", false, "enable logging")
	c.GlobalFlags().Bool("no-logging", false, "disable logging")
	c.MarkFlagsMutuallyExclusive("logging", "no-logging")

	if err := c.GlobalFlags().MarkHidden("no-logging"); err != nil {
		panic(fmt.Sprintf("failed to mark --no-logging hidden: %v", err))
	}

	c.Add(apply.New())

	return c, nil
}

func setup(cmd, subcmd *cli.Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	cfg, err := config.Parse(subcmd.Flags())
	if err != nil {
		return fmt.Errorf("failed to parse the config: %w", err)
	}

	slog.Info("config parsed", "config", cfg)

	if err := logging.Init(cfg.Logging); err != nil {
		return fmt.Errorf("failed to init the logger: %w", err)
	}

	slog.Debug("logging initialized")

	pluginFiles, err := lookUpPlugins(cfg.PluginDir)
	if err != nil {
		return fmt.Errorf("failed to look up plugins: %w", err)
	}

	slog.Debug("performed the plugin lookup", "plugins", pluginFiles)

	plugin.ResolvePlugins(pluginFiles)

	return nil
}

func run(_ *cli.Command, _ []string) error {
	fmt.Fprintln(os.Stdout, "HELP MESSAGE")

	return nil
}

// lookUpPlugins looks up the plugin files (files starting with "reginald-") in
// the plugins directory. It returns a list of the absolute paths.
func lookUpPlugins(dir string) ([]string, error) {
	var plugins []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			if strings.HasPrefix(entry.Name(), name+"-") {
				plugins = append(plugins, filepath.Join(dir, entry.Name()))
			}
		}
	}

	return plugins, nil
}
