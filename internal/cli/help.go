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
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/text"
	"github.com/spf13/pflag"
)

// Constants for the help message.
const (
	description = "Reginald is the personal workstation valet."
	maxWidth    = 80
	minWidth    = 40
	usagePrefix = "Usage: "
)

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
	parts = append(parts, "<command>", "[<args>]")

	return strings.Join(parts, " ")
}

// formatUsage wraps the given usage line to the given with and pads each new
// line with spaces.
func formatUsage(s string, width int, parents ...string) string {
	var (
		tokens []string
		sb     strings.Builder
		depth  int
	)

	i := strings.Index(s, " ")
	if i == -1 {
		i = len(s)
	}

	cmd := s[:i]
	args := s[i:]

	for _, r := range args {
		switch {
		case (r == ' ' || r == '\t' || r == '\n') && depth == 0:
			if sb.Len() > 0 {
				tokens = append(tokens, sb.String())
				sb.Reset()
			}
		default:
			switch r {
			case '[', '(':
				depth++
			case ']', ')':
				depth--
			}

			sb.WriteRune(r)
		}
	}

	if sb.Len() > 0 {
		tokens = append(tokens, sb.String())
	}

	var prefix string
	if len(parents) > 0 {
		prefix = usagePrefix + strings.Join(parents, " ") + " " + cmd
	} else {
		prefix = usagePrefix + cmd
	}

	indent := len(prefix) + 1

	var (
		lines []string
		cur   = prefix
		col   = len(prefix)
	)

	for _, p := range tokens {
		if col+1+len(p) > width {
			lines = append(lines, cur)
			cur = strings.Repeat(" ", indent) + p
			col = indent + len(p)
		} else {
			cur += " " + p
			col += 1 + len(p)
		}
	}

	lines = append(lines, cur)

	return strings.Join(lines, "\n")
}

// printHelp prints the help message for the given command.
func printHelp(cmd *plugin.Command, flagSet *flags.FlagSet) {
	var sb strings.Builder

	width := min(max(terminal.Width(), minWidth), maxWidth)
	desc := description

	var (
		usage   string // don't calculate unless needed
		parents []string
	)

	if cmd != nil {
		desc = cmd.Description
		usage = cmd.Usage

		for parent := cmd.Parent; parent != nil; parent = parent.Parent {
			parents = append(parents, parent.Name)
		}

		parents = append(parents, Name)
		slices.Reverse(parents)
	} else {
		usage = defaultUsage()
	}

	sb.WriteString(text.Wrap(desc, width))
	sb.WriteByte('\n')
	sb.WriteString(formatUsage(usage, width, parents...))
	sb.WriteString("\n\nOptions:\n")
	sb.WriteString(flagSet.FlagUsages())

	terminal.Print(sb.String())
	terminal.Flush()
}

// runHelp runs the help command or flag by resolving the place of the command
// or the flag in the arguments list. It prints the help message of the command
// that was given before the flag.
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
