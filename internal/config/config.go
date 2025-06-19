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

// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"unicode"

	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/terminal"
)

// EnvPrefix is the prefix added to the names of the config values when reading
// them from environment variables.
const EnvPrefix = "REGINALD"

const (
	defaultFileName    = "reginald"
	defaultLogFileName = defaultFileName + ".log"
)

// Config is the parsed configuration of the program run. There should be only
// one effective Config per run.
//
// Config has a lock for locking it when it is being parsed and written to.
// After the parsing, Config should not be written to and, thus, the lock should
// no longer be used.
type Config struct {
	// sourceFile is path to the config file that was found and parsed.
	sourceFile fspath.Path

	// Directory is the "dotfiles" directory option. If it is set, Reginald
	// looks for all of the relative filenames from this directory. Most
	// absolute paths are still resolved relative to actual current working
	// directory of the program.
	Directory fspath.Path `mapstructure:"directory"`

	// PluginPaths is the directory where Reginald looks for the plugins.
	PluginPaths []fspath.Path `mapstructure:"plugin-paths"`

	// Defaults contains the default options set for tasks.
	Defaults TaskDefaults `mapstructure:"defaults"`

	// Plugins contains the rest of the config options which should only be
	// plugin-defined options.
	Plugins map[string]any `mapstructure:",remain"` //nolint:tagliatelle // linter doesn't know about "remain"

	// Tasks contains tasks and the configs for them as given in the config
	// file.
	Tasks []Task `mapstructure:"tasks"`

	// Logging contains the config values for logging.
	Logging logging.Config `flag:"log" mapstructure:"logging"`

	// Color tells whether colors should be enabled in the user output.
	Color terminal.ColorMode `mapstructure:"color"`

	// Quiet tells the program to suppress all other output than errors.
	Quiet bool `mapstructure:"quiet"`

	// Verbose tells the program to print more verbose output.
	Verbose bool `mapstructure:"verbose"`
}

// A Task is the configuration of a task instance.
type Task struct {
	// Type is the type of this task. It defines which task implementation is
	// called when this task is executed.
	Type string `mapstructure:"type"`

	// ID is the unique ID for this task. It must be unique. The ID must also be
	// different from the provided task types.
	ID string `mapstructure:"id,omitempty"`

	// Options contains the rest of the config options for the task.
	Options TaskOptions `mapstructure:",remain"` //nolint:tagliatelle // linter doesn't know about "remain"

	// Dependencies are the task IDs or types that this task depends on.
	Dependencies []string `mapstructure:"dependencies"`
}

// TaskDefaults is the type for the default config values set for the tasks.
type TaskDefaults map[string]any

// TaskOptions is the type for the config options in a task config entry.
type TaskOptions map[string]any

// DefaultConfig returns the default values for configuration. The function
// panics on errors.
func DefaultConfig() *Config {
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get current directory: %v", err))
	}

	logOutput, err := DefaultLogOutput()
	if err != nil {
		panic(fmt.Sprintf("failed to get the default log output: %v", err))
	}

	pluginPaths, err := DefaultPluginPaths()
	if err != nil {
		panic(fmt.Sprintf("failed to get default plugin directory: %v", err))
	}

	return &Config{
		Color:     terminal.ColorAuto,
		Defaults:  TaskDefaults{},
		Directory: fspath.Path(wd),
		Logging: logging.Config{
			Enabled: true,
			Format:  "json",
			Level:   logs.LevelInfo,
			Output:  logOutput.String(),
		},
		PluginPaths: pluginPaths,
		Quiet:       false,
		Tasks:       []Task{},
		Verbose:     false,
		Plugins:     map[string]any{},
	}
}

// File returns path to the config file that was used to parse the config.
func (c *Config) File() fspath.Path {
	return c.sourceFile
}

// HasFile reports whether the config was parsed from a file.
func (c *Config) HasFile() bool {
	return c.sourceFile != ""
}

// IsBool reports whether o has an entry with the given key that is a bool.
func (o TaskOptions) IsBool(key string) bool {
	v, ok := o[key]
	if !ok {
		return false
	}

	if _, ok := v.(bool); !ok {
		return false
	}

	return true
}

// DefaultDir returns the default working directory for Reginald.
func DefaultDir() (fspath.Path, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get the user home directory: %w", err)
	}

	path, err := fspath.NewAbs(home, ".dotfiles")
	if err != nil {
		return "", fmt.Errorf("failed to convert directory to absolute path: %w", err)
	}

	return path, nil
}

// DefaultLogOutput returns the default logging output file to use.
func DefaultLogOutput() (fspath.Path, error) {
	path, err := defaultPlatformLogFile()
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return path, nil
}

// DefaultPluginPaths returns the default plugins directory to use.
func DefaultPluginPaths() ([]fspath.Path, error) {
	paths, err := defaultPlatformPluginPaths()
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	for i, p := range paths {
		paths[i] = p.Clean()
	}

	return paths, nil
}

// FlagName returns the command-line flag name for the given Config field s.
// The field name should be given as you would write it in Go syntax, for
// example "Logging.Output".
//
// Flag name is primarily resolved from the "flag" tag in the struct tags for
// the field. The tag should be written as `flag:"<regular>,<inverted>"`where
// regular is the normal name of the flag that is used to either give the value
// or set the value as true. The inverted is available only for boolean types
// and it is used for getting the name of a flag that explicitly set the value
// of the field to false. The inverted name and the comma before it may be
// omitted.
//
// If the field has no "flag" tag, the flag name will be calculated from
// the field name. The function converts the field's name to lower case (and to
// "kebab-case") and adds the names of the parent fields before the field name
// separated with hyphen.
func FlagName(s string) string {
	return genFlagName(s, false)
}

// InvertedFlagName returns the command-line flag for the given Config field for
// a flag that explicitly sets the value of the boolean to false. The field name
// should be given as you would write it in Go syntax, for example
// "Logging.Output".
//
// Flag name is primarily resolved from the "flag" tag in the struct tags for
// the field. The tag should be written as `flag:"<regular>,<inverted>"`where
// regular is the normal name of the flag that is used to either give the value
// or set the value as true. The inverted is available only for boolean types
// and it is used for getting the name of a flag that explicitly set the value
// of the field to false. The inverted name and the comma before it may be
// omitted.
//
// If the field has no inverted flag name in the "flag" tag, this function will
// panic.
func InvertedFlagName(s string) string {
	return genFlagName(s, true)
}

// HasInvertedFlagName reports whether the given config value has an inverted
// flag name tag.
func HasInvertedFlagName(s string) bool {
	if s == "" {
		return false
	}

	cfg := Config{} //nolint:exhaustruct // used only for reflection
	fieldNames := strings.Split(s, ".")
	typ := reflect.TypeOf(cfg)

	for _, name := range fieldNames {
		f, ok := typ.FieldByName(name)
		if !ok {
			return false
		}

		if f.Type.Kind() == reflect.Struct {
			typ = f.Type

			continue
		}

		if f.Type.Kind() != reflect.Bool {
			return false
		}

		t := strings.ToLower(f.Tag.Get("flag"))
		tags := strings.FieldsFunc(t, func(r rune) bool {
			return r == ','
		})

		if len(tags) < 2 { //nolint:mnd // only the flag and the inverted flag are allowed
			return false
		}

		if tags[1] != "" {
			return true
		}
	}

	return false
}

// genFlagName resolves the flag name or the name of the inverted tag for
// the Config field. The process is documented with [FlagName].
func genFlagName(s string, invert bool) string {
	cfg := Config{} //nolint:exhaustruct // used only for reflection
	fieldNames := strings.Split(s, ".")
	typ := reflect.TypeOf(cfg)
	flagName := ""

	for _, name := range fieldNames {
		f, ok := typ.FieldByName(name)
		if !ok {
			panic(fmt.Sprintf("field in %q with name %q not found", typ.Name(), name))
		}

		t := strings.ToLower(f.Tag.Get("flag"))
		tags := strings.FieldsFunc(t, func(r rune) bool {
			return r == ','
		})

		if f.Type.Kind() != reflect.Bool && len(tags) > 1 {
			panic(fmt.Sprintf("field %q (%s) has invert flag tag: %q", f.Name, f.Type.Kind(), t))
		}

		if f.Type.Kind() != reflect.Struct && invert && len(tags) < 2 {
			panic(fmt.Sprintf("field %q has no invert flag tag: %q", f.Name, t))
		}

		if len(tags) > 2 { //nolint:mnd // only the flag and the inverted flag are allowed
			panic(fmt.Sprintf("field %q has invalid flag tag: %q", f.Name, t))
		}

		j := 0

		if invert && len(tags) > 1 {
			j = 1
		}

		if len(tags) > 0 && tags[j] != "" {
			// If the field has a "flag" tag, it overrides the whole tag thus
			// far.
			flagName = tags[j]
		} else {
			if flagName != "" {
				flagName += "-"
			}

			for i, r := range f.Name {
				if i > 0 && unicode.IsUpper(r) {
					flagName += "-"
				}

				flagName += string(r)
			}
		}

		if f.Type.Kind() == reflect.Struct {
			typ = f.Type
		}
	}

	return strings.ToLower(flagName)
}

// resolveFile looks up the possible paths for the configuration file and
// returns the first one that contains a file with a valid name. The returned
// path is absolute. If no configuration file is found, the function returns an
// empty string and an error.
func resolveFile(flagSet *flags.FlagSet) (fspath.Path, error) {
	var (
		err       error
		fileValue string
	)

	if env := os.Getenv(strings.ToUpper(EnvPrefix + "_CONFIG_FILE")); env != "" {
		fileValue = env
	}

	if flagSet.Changed("config") {
		fileValue, err = flagSet.GetString("config")
		if err != nil {
			return "", fmt.Errorf(
				"failed to get the value for command-line option --%s: %w",
				"config",
				err,
			)
		}
	}

	file := fspath.Path(fileValue)

	// Use the fileValue if it is an absolute path.
	if file.IsAbs() {
		var ok bool

		if ok, err = file.IsFile(); err != nil {
			return "", fmt.Errorf("%w", err)
		} else if ok {
			return file.Clean(), nil
		}
	}

	var wd fspath.Path

	if env := os.Getenv(strings.ToUpper(EnvPrefix + "_DIRECTORY")); env != "" {
		wd = fspath.Path(env)
	}

	flagName := FlagName("Directory")
	if flagSet.Changed(flagName) {
		wd, err = flagSet.GetPath(flagName)
		if err != nil {
			return "", fmt.Errorf(
				"failed to get the value for command-line option --%s: %w",
				flagName,
				err,
			)
		}
	}

	if wd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}

		wd = fspath.Path(cwd)
	}

	// Check if the config file f matches a file in the working directory.
	file = wd.Join(string(file))

	if ok, err := file.IsFile(); err != nil {
		return "", fmt.Errorf("%w", err)
	} else if ok {
		return file, nil
	}

	// If the config file flag is set but it didn't resolve, fail so that the
	// program doesn't use a config file from some other location by surprise.
	if fileValue != "" {
		return "", fmt.Errorf("%w: tried to resolve file with %q", errConfigFileNotFound, fileValue)
	}

	// TODO: Add more locations, at least the default location in the user home
	// directory.
	configDirs := []fspath.Path{
		wd,
	}
	configNames := []string{
		strings.ToLower(defaultFileName),
		"." + strings.ToLower(defaultFileName),
	}
	extensions := []string{
		"toml",
	}

	// This is crazy.
	for _, d := range configDirs {
		for _, n := range configNames {
			for _, e := range extensions {
				file = d.Join(fmt.Sprintf("%s.%s", n, e))
				if ok, err := file.IsFile(); err != nil {
					return "", fmt.Errorf("%w", err)
				} else if ok {
					return file, nil
				}
			}
		}
	}

	return "", fmt.Errorf("%w", errDefaultConfig)
}
