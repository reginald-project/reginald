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
	"os"

	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/spf13/pflag"
)

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

// IsDebug reports whether the program should run in debug mode. It should only
// be called during the bootstrapping of the program, and if the information
// required later, the fully parsed config should be used instead. The function
// ignores all errors and return the default value for the debug mode if it
// fails to parse the value.
func IsDebug(ctx context.Context) bool {
	flagSet := FlagSet()
	flagSet.AddFlagSet(configFlagSet())
	_ = flagSet.Parse(os.Args[1:])       //nolint:errcheck // ignore errors
	cfg, _ := config.Parse(ctx, flagSet) //nolint:errcheck // ignore errors

	return cfg.Debug
}

// configFlagSet returns the flags that are required for fully determining
// whether the program should run in debug mode. Essentially, this returns
// a flag set with the "--config" and "--directory" flags as the config file
// needs to be checked in order to see if the debug mode is enabled.
func configFlagSet() *flags.FlagSet {
	flagSet := flags.NewFlagSet("", pflag.ContinueOnError)
	defaults := config.DefaultConfig()

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
