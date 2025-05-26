// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/anttikivi/reginald/internal/flags"
	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/pkg/task"
	"github.com/spf13/afero"
)

// EnvPrefix is the prefix added to the names of the config values when reading
// them from environment variables.
const EnvPrefix = "REGINALD" // prefix used for the environment variables.

// Possible values for [ColorMode].
const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

const (
	defaultFileName  = "reginald"
	defaultLogOutput = "~/.local/state/reginald.log"
)

// errColorMode is returned when an invalid value is parsed into [ColorMode].
var errColorMode = errors.New("invalid color mode")

// ColorMode represent the color output setting of the program.
type ColorMode int

// Config is the parsed configuration of the program run. There should be only
// one effective Config per run.
//
// Config has a lock for locking it when it is being parsed and written to.
// After the parsing, Config should not be written to and, thus, the lock should
// no longer be used.
type Config struct {
	// Color tells whether colors should be enabled in the user output.
	Color ColorMode `default:"auto" mapstructure:"color"`

	// Logging contains the config values for logging.
	Logging logging.Config `mapstructure:"logging"`

	// PluginDir is the directory where Reginald looks for the plugins.
	PluginDir fspath.Path `mapstructure:"plugin-dir"`

	// Quiet tells the program to suppress all other output than errors.
	Quiet bool `mapstructure:"quiet"`

	// Tasks contains tasks and the configs for them as given in the config
	// file.
	Tasks []task.Config `mapstructure:"tasks"`

	// Verbose tells the program to print more verbose output.
	Verbose bool `mapstructure:"verbose"`

	// Plugins contains the rest of the config options which should only be
	// plugin-defined options.
	Plugins map[string]any `mapstructure:",remain"`
}

// String returns the string representation of c.
func (c ColorMode) String() string {
	switch c {
	case ColorAlways:
		return "always"
	case ColorAuto:
		return "auto"
	case ColorNever:
		return "never"
	default:
		return "invalid"
	}
}

// Set sets the value of c from the given string s.
func (c *ColorMode) Set(s string) error {
	switch s = strings.ToLower(s); s {
	case "true", "always", "yes", "1":
		*c = ColorAlways
	case "false", "never", "no", "0":
		*c = ColorNever
	case "auto", "":
		*c = ColorAuto
	default:
		return fmt.Errorf("%w: %q", errColorMode, s)
	}

	return nil
}

// Type returns type of c as a string for command-line flags.
func (c *ColorMode) Type() string {
	return "ColorMode"
}

// MarshalJSON encodes c as a JSON value.
func (c ColorMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// UnmarshalJSON assign the value from the given JSON representation to c.
func (c *ColorMode) UnmarshalJSON(data []byte) error {
	var (
		err error
		s   string
	)

	if err = json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to unmarshal ColorMode: %w", err)
	}

	if err = c.Set(s); err != nil {
		return fmt.Errorf("failed to set ColorMode: %w", err)
	}

	return nil
}

// MarshalText encodes c in a textual form.
func (c ColorMode) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

// UnmarshalText assigns the value from the given textual representation to c.
func (c *ColorMode) UnmarshalText(data []byte) error {
	if err := c.Set(string(data)); err != nil {
		return fmt.Errorf("failed to set ColorMode: %w", err)
	}

	return nil
}

// DefaultPluginsDir returns the plugins directory to use. It takes the environment
// variable for customizing the plugins directory and the platform into account.
func DefaultPluginsDir() (fspath.Path, error) {
	path, err := defaultPluginsDir()
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return path.Clean(), nil
}

// resolveFile looks up the possible paths for the configuration file and
// returns the first one that contains a file with a valid name. The returned
// path is absolute. If no configuration file is found, the function returns an
// empty string and an error.
func resolveFile(fs afero.Fs, flagSet *flags.FlagSet) (fspath.Path, error) {
	var (
		err       error
		fileValue string
	)

	if env := os.Getenv(EnvPrefix + "_CONFIG_FILE"); env != "" {
		fileValue = env
	}

	if flagSet.Changed("config") {
		fileValue, err = flagSet.GetString("config")
		if err != nil {
			return "", fmt.Errorf(
				"failed to get the value for command-line option '--config': %w",
				err,
			)
		}
	}

	file := fspath.Path(fileValue)

	// Use the fileValue if it is an absolute path.
	if file.IsAbs() {
		if ok, err := file.IsFile(fs); err != nil {
			return "", fmt.Errorf("%w", err)
		} else if ok {
			return file.Clean(), nil
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	// Check if the config file f matches a file in the working directory.
	file = fspath.New(wd, string(file))

	if ok, err := file.IsFile(fs); err != nil {
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
	configDirs := []string{
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
				file = fspath.New(d, fmt.Sprintf("%s.%s", n, e))
				if ok, err := file.IsFile(fs); err != nil {
					return "", fmt.Errorf("%w", err)
				} else if ok {
					return file, nil
				}
			}
		}
	}

	return "", fmt.Errorf("%w", errConfigFileNotFound)
}
