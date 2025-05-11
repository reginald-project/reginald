// Package cli defines the command-line interface of Reginald.
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

// programName is the canonical name of this program.
const programName = "Reginald"

// Run executes the given root command. It parses the command-line options,
// finds the correct subcommand to run, and executes it. It returns any error
// encountered during the run. The function panics if it is called with invalid
// configuration, e.g. with command other than the root command.
func Run(cmd *RootCommand) error {
	if cmd.HasParent() {
		panic("the CLI must be run using the root command")
	}

	args := os.Args

	slog.Debug("starting to parse the command-line arguments", "args", args)

	c, args, flags := findSubcommand(&cmd.Command, args)

	args = append(args, flags...)

	c.mergeFlags()

	// TODO: Move checking the errors to a later time when the plugin system is
	// in place. It should be possible to define subcommands and flags for them
	// using the plugins.
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse command-line arguments: %w", err)
	}

	help, err := cmd.GlobalFlags().GetBool("help")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--help': %w", err)
	}

	if help {
		fmt.Fprintln(os.Stdout, "HELP MESSAGE")

		return nil
	}

	version, err := cmd.GlobalFlags().GetBool("version")
	if err != nil {
		return fmt.Errorf("failed to get the value for command-line option '--version': %w", err)
	}

	if version {
		fmt.Fprintf(os.Stdout, "%s %s\n", programName, cmd.Version)

		return nil
	}

	// setupCmd

	return nil
}

// findSubcommand finds the subcommand to run from the tree starting at cmd. It
// returns the final subcommand, the arguments without the subcommands and flags
// (positional command-line arguments), and command-line flags.
func findSubcommand(cmd *Command, args []string) (*Command, []string, []string) {
	if len(args) <= 1 {
		return cmd, args, []string{}
	}

	flags := []string{}
	c := cmd

	for len(args) >= 1 {
		if len(args) > 1 {
			args, flags = collectFlags(c, args[1:], flags)
		}

		if len(args) >= 1 {
			next := c.Lookup(args[0])

			if next == nil {
				break
			}

			c = next
		}
	}

	slog.Debug("found subcommand", "cmd", c.Name(), "args", args, "flags", flags)

	return c, args, flags
}

// collectFlags removes all of the known flags from the arguments list and
// appends them to flags. It returns the non-flag arguments as the first return
// value and the appended flags as the second return value. It does not check
// for any errors; all of the arguments that might look like flags but are not
// found in the flag set of c are treated as regular command-line arguments. If
// the user has run the program correctly, this function should return the next
// subcommand as the first element of the argument slice.
func collectFlags(c *Command, args, flags []string) ([]string, []string) {
	slog.Debug("collecting flags", "args", args, "flags", flags)

	if len(args) == 0 {
		return args, flags
	}

	c.mergeFlags()

	rest := []string{}
	fs := c.Flags()

	// TODO: This is probably way more inefficient than it needs to be, but it
	// gets the work done for now.
Loop:
	for len(args) > 0 {
		s := args[0]
		args = args[1:]

		slog.Debug("checking argument", "arg", s)

		switch {
		case s == "--":
			// Stop parsing at "--".
			break Loop
		case strings.HasPrefix(s, "-") && strings.Contains(s, "="):
			// All of the cases with "=": "--flag=value", "-f=value", and
			// "-abf=value".
			if hasFlag(fs, s) {
				flags = append(flags, s)
			} else {
				rest = append(rest, s)
			}
		case strings.HasPrefix(s, "--") && !hasNoOptDefVal(s[2:], fs):
			// The '--flag arg' case.
			fallthrough
		case strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "--") && !shortHasNoOptDefVal(s[len(s)-1:], fs):
			// The '-f arg' and '-abcf arg' cases. Only the last flag in can
			// have a argument, so other ones aren't checked for the default
			// value.
			if hasFlag(fs, s) {
				if len(args) == 0 {
					flags = append(flags, s)
				} else {
					flags = append(flags, s, args[0])
					args = args[1:]
				}
			} else {
				rest = append(rest, s)
			}
		case strings.HasPrefix(s, "-") && len(s) >= 2:
			// Rest of the flags.
			if hasFlag(fs, s) {
				flags = append(flags, s)
			} else {
				rest = append(rest, s)
			}
		default:
			rest = append(rest, s)
		}
	}

	rest = append(rest, args...)

	slog.Debug("collected flags", "rest", rest, "flags", flags)

	return rest, flags
}

// hasFlag checks whether the given flag s is in fs. The whole flag string must
// be included. The function checks by looking up the shorthands if the string
// starts with only one hyphen. If s contains a combination of shorthands, the
// function will check for all of them.
func hasFlag(fs *pflag.FlagSet, s string) bool {
	if strings.HasPrefix(s, "--") {
		if strings.Contains(s, "=") {
			return fs.Lookup(s[2:strings.Index(s, "=")]) != nil
		}

		return fs.Lookup(s[2:]) != nil
	}

	if strings.HasPrefix(s, "-") {
		if len(s) == 2 { //nolint:mnd
			return fs.ShorthandLookup(s[1:]) != nil
		}

		if strings.Index(s, "=") == 2 { //nolint:mnd
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
func hasNoOptDefVal(name string, fs *pflag.FlagSet) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}

	return f.NoOptDefVal != ""
}

// shortHasNoOptDefVal checks if the flag for the given shorthand has a
// NoOptDefVal set.
func shortHasNoOptDefVal(name string, fs *pflag.FlagSet) bool {
	f := fs.ShorthandLookup(name[:1])
	if f == nil {
		return false
	}

	return f.NoOptDefVal != ""
}
