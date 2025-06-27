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

// Package debugging contains debugging utilities for Reginald.
package debugging

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/spf13/pflag"
)

// enabled tells whether debugging is enabled.
var enabled = false //nolint:gochecknoglobals // can be set at compile time

// FlagSet returns a flag set with the debugging flags.
func FlagSet() *flags.FlagSet {
	flagSet := flags.NewFlagSet("", pflag.ContinueOnError)
	debugFlag := config.FlagName("Debug")

	flagSet.Bool(debugFlag, config.DefaultConfig().Debug, "print debug output", "")

	if err := flagSet.MarkHidden(debugFlag); err != nil {
		panic(fmt.Sprintf("failed to mark --%s hidden: %v", debugFlag, err))
	}

	return flagSet
}

// Init initializes the debugging module.
func Init(ctx context.Context) {
	// If the debug mode is already set, it is possible that it is done during
	// the build. That shouldn't be overwritten.
	if enabled {
		return
	}

	flagSet := FlagSet()
	flagSet.AddFlagSet(configFlagSet())
	flagSet.SetOutput(io.Discard)
	_ = flagSet.Parse(os.Args[1:])       //nolint:errcheck // ignore errors
	cfg, _ := config.Parse(ctx, flagSet) //nolint:errcheck // ignore errors
	enabled = cfg.Debug
}

// IsDebug reports whether the program should run in debug mode.
func IsDebug() bool {
	return enabled
}

// SetDebug sets the debugging mode to b if it is not already set.
func SetDebug(b bool) {
	// If the debug mode is already set, it is possible that it is done during
	// the build. That shouldn't be overwritten.
	if enabled {
		enabled = b
	}
}

// configFlagSet returns the flags that are required for fully determining
// whether the program should run in debug mode. Essentially, this returns
// a flag set with the "--config" and "--directory" flags as the config file
// needs to be checked in order to see if the debug mode is enabled.
func configFlagSet() *flags.FlagSet {
	flagSet := flags.NewFlagSet("", pflag.ContinueOnError)
	defaults := config.DefaultConfig()

	flagSet.BoolP("help", "h", false, "", "")
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

	return flagSet
}
