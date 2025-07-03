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
	"bytes"
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
	rootHelp    = "TODO"
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

// formatCommands wraps the given commands and their descriptions to the given
// width and pads each new line with spaces.
//
// This function is adapted from github.com/spf13/pflag.
// Copyright (c) 2012 Alex Ogier. All rights reserved.
// Copyright (c) 2012 The Go Authors. All rights reserved.
// See THIRD_PARTY_NOTICES for more information.
func formatCommands(cmds []*plugin.Command, indent, cols int) string {
	var buf bytes.Buffer

	lines := make([]string, 0, len(cmds))
	maxlen := 0

	for _, cmd := range cmds {
		line := strings.Repeat(" ", indent) + cmd.Name

		// This special character will be replaced with spacing once the correct
		// alignment is calculated
		line += "\x00"

		if len(line) > maxlen {
			maxlen = len(line)
		}

		line += cmd.Description
		lines = append(lines, line)
	}

	for _, line := range lines {
		sidx := strings.Index(line, "\x00")
		spacing := strings.Repeat(" ", maxlen-sidx)
		// maxlen + 2 comes from + 1 for the \x00 and + 1 for the (deliberate)
		// off-by-one in maxlen-sidx
		fmt.Fprintln(&buf, line[:sidx], spacing, wrap(maxlen+2, cols, line[sidx+1:])) //nolint:mnd
	}

	return buf.String()
}

// formatUsage wraps the given usage line to the given width and pads each new
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
func printHelp(cmd *plugin.Command, flagSet *flags.FlagSet, store *plugin.Store) {
	var sb strings.Builder

	width := min(max(terminal.Width(), minWidth), maxWidth)
	desc := description
	help := rootHelp
	cmds := store.Commands

	var (
		usage   string // don't calculate unless needed
		parents []string
	)

	if cmd != nil {
		desc = cmd.Description
		usage = cmd.Usage
		help = cmd.Help

		for parent := cmd.Parent; parent != nil; parent = parent.Parent {
			parents = append(parents, parent.Name)
		}

		parents = append(parents, Name)
		slices.Reverse(parents)

		if len(cmd.Commands) > 0 {
			cmds = cmd.Commands
		}
	} else {
		usage = defaultUsage()
	}

	sb.WriteString(text.Wrap(desc, width))
	sb.WriteByte('\n')
	sb.WriteString(formatUsage(usage, width, parents...))
	sb.WriteString("\n\n")
	sb.WriteString(text.Wrap(help, width))
	sb.WriteString("\nCommands:\n")
	sb.WriteString(formatCommands(cmds, 2, width)) //nolint:mnd
	sb.WriteString("\nOptions:\n")
	sb.WriteString(flagSet.FlagUsagesWrapped(width))

	terminal.Print(sb.String())
	terminal.Flush()
}

// runHelp runs the help command or flag by resolving the place of the command
// or the flag in the arguments list. It prints the help message of the command
// that was given before the flag.
func runHelp(cmd *plugin.Command, store *plugin.Store) error {
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

		if root == nil {
			continue
		}

		if arg == root.Name || slices.Contains(root.Aliases, arg) {
			found = root

			if err := addFlags(flagSet, found); err != nil {
				return err
			}
		}
	}

	printHelp(found, flagSet, store)

	return nil
}

// Wraps the string s to a maximum width w with leading indent i. The first line
// is not indented (this is assumed to be done by caller). Pass w == 0 to do no
// wrapping.
//
// This function is adapted from github.com/spf13/pflag.
// Copyright (c) 2012 Alex Ogier. All rights reserved.
// Copyright (c) 2012 The Go Authors. All rights reserved.
// See THIRD_PARTY_NOTICES for more information.
func wrap(i, w int, s string) string {
	if w == 0 {
		return strings.ReplaceAll(s, "\n", "\n"+strings.Repeat(" ", i))
	}

	// space between indent i and end of line width w into which
	// we should wrap the text.
	wrap := w - i

	var r, l string

	// Not enough space for sensible wrapping. Wrap as a block on
	// the next line instead.
	if wrap < 24 { //nolint:mnd
		i = 16
		wrap = w - i
		r += "\n" + strings.Repeat(" ", i)
	}

	// If still not enough space then don't even try to wrap.
	if wrap < 24 { //nolint:mnd
		return strings.ReplaceAll(s, "\n", r)
	}

	// Try to avoid short orphan words on the final line, by
	// allowing wrapN to go a bit over if that would fit in the
	// remainder of the line.
	slop := 5
	wrap -= slop

	// Handle first line, which is indented by the caller (or the
	// special case above)
	l, s = wrapN(wrap, slop, s)
	r += strings.ReplaceAll(l, "\n", "\n"+strings.Repeat(" ", i))

	// Now wrap the rest
	for s != "" {
		var t string

		t, s = wrapN(wrap, slop, s)
		r = r + "\n" + strings.Repeat(" ", i) + strings.ReplaceAll(t, "\n", "\n"+strings.Repeat(" ", i))
	}

	return r
}

// Splits the string s on whitespace into an initial substring up to i runes in
// length and the remainder. Will go slop over i if that encompasses the entire
// string (which allows the caller to avoid short orphan words on the final
// line).
//
// This function is adapted from github.com/spf13/pflag.
// Copyright (c) 2012 Alex Ogier. All rights reserved.
// Copyright (c) 2012 The Go Authors. All rights reserved.
// See THIRD_PARTY_NOTICES for more information.
func wrapN(i, slop int, s string) (string, string) {
	if i+slop > len(s) {
		return s, ""
	}

	w := strings.LastIndexAny(s[:i], " \t\n")
	if w <= 0 {
		return s, ""
	}

	nlPos := strings.LastIndex(s[:i], "\n")
	if nlPos > 0 && nlPos < w {
		return s[:nlPos], s[nlPos+1:]
	}

	return s[:w], s[w+1:]
}
