// Package root defines the root command for Reginald.
package root

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/anttikivi/reginald/internal/cli/apply"
	"github.com/anttikivi/reginald/internal/config"
)

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New(version string) (*cli.RootCommand, error) {
	c := &cli.RootCommand{
		Command: cli.Command{
			UsageLine: name + " [--version] [-h | --help] <command> [<args>]",
			Setup:     setup,
			Run:       run,
		},
		Version: version,
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

	c.Add(apply.New())

	return c, nil
}

func setup(cmd, subcmd *cli.Command, _ []string) error {
	slog.Info("running setup", "cmd", cmd.Name())

	cfg, err := config.Parse(subcmd.Flags())
	if err != nil {
		return fmt.Errorf("failed to parse the config: %w", err)
	}

	fmt.Println(cfg)

	return nil
}

func run(_ *cli.Command, _ []string) error {
	fmt.Fprintln(os.Stdout, "HELP MESSAGE")

	return nil
}
