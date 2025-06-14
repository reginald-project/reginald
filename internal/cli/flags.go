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

package cli

import (
	"strings"

	"github.com/reginald-project/reginald/internal/flags"
)

// collectFlags removes all of the known flags from the arguments list and
// appends them to flags. It returns the non-flag arguments as the first return
// value and the appended flags as the second return value. It does not check
// for any errors; all of the arguments that might look like flags but are not
// found in the flag set are treated as regular command-line arguments. If the
// user has run the program correctly, this function should return the next
// subcommand as the first element of the argument slice.
func collectFlags(fs *flags.FlagSet, args, collected []string) ([]string, []string) {
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
			if hasFlag(fs, s) {
				collected = append(collected, s)
			} else {
				rest = append(rest, s)
			}
		case strings.HasPrefix(s, "--") && !hasNoOptDefVal(s[2:], fs):
			// The '--flag arg' case.
			fallthrough //nolint:gocritic // this is much clearer with an empty fallthrough
		case strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "--") && !shortHasNoOptDefVal(s[len(s)-1:], fs):
			// The '-f arg' and '-abcf arg' cases. Only the last flag in can
			// have a argument, so other ones aren't checked for the default
			// value.
			if hasFlag(fs, s) {
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
			if hasFlag(fs, s) {
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

// shortHasNoOptDefVal checks if the flag for the given shorthand has a
// NoOptDefVal set.
func shortHasNoOptDefVal(name string, fs *flags.FlagSet) bool {
	f := fs.ShorthandLookup(name[:1])
	if f == nil {
		return false
	}

	return f.NoOptDefVal != ""
}
