// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anttikivi/reginald/internal/cli"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
)

// Config the parsed configuration of the program run. There should be only one
// effective Config per run.
type Config struct {
	Logging LoggingConfig // logging config values
}

// LoggingConfig is type of the logging configuration in Config.
type LoggingConfig struct {
	Enabled bool       // whether logging is enabled
	Format  string     // format of the logs, "json" or "text"
	Level   slog.Level // logging level
	Output  string     // destination of the logs
}

// errConfigFileNotFound is returned when a configuration file is not found.
var errConfigFileNotFound = errors.New("config file not found")

// Parse parses the configuration according to the configuration given with fs.
// The FlagSet fs should be a flag set that contains all of the flags for the
// program as the function uses the flags to override values from the
// configuration file. The function returns a pointer to the parsed
// configuration and any errors it encounters.
//
// The function also resolves the configuration file according to the standard
// paths for the file or according the flags. The relevant flags are
// `--directory` and `--config`.
func Parse(fs *pflag.FlagSet) (*Config, error) {
	wd, err := fs.GetString("directory")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get the value for command-line option '--directory': %w",
			err,
		)
	}

	filename, err := fs.GetString("config")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get the value for command-line option '--config': %w",
			err,
		)
	}

	configFile, err := resolveFile(wd, filename)
	if err != nil {
		return nil, fmt.Errorf("searching for config file failed: %w", err)
	}

	data, err := os.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := defaultConfig()
	if err = toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the config file: %w", err)
	}

	return config, nil
}

// resolveFile looks up the possible paths for the configuration file and
// returns the first one that contains a file with a valid name. The returned
// path is absolute. If no configuration file is found, the function returns an
// empty string and an error.
func resolveFile(wd, f string) (string, error) {
	// Use the config file f if it is an absolute path.
	if f != "" && filepath.IsAbs(f) {
		if ok, err := checkFile(f); err != nil {
			return "", fmt.Errorf("%w", err)
		} else if ok {
			return filepath.Clean(f), nil
		}
	}

	if !filepath.IsAbs(wd) {
		realWD, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get the current working directory: %w", err)
		}

		wd = filepath.Join(realWD, wd)
	}

	// Check if the config file f matches a file in the working directory.
	if f != "" {
		f = filepath.Join(wd, f)
		if ok, err := checkFile(f); err != nil {
			return "", fmt.Errorf("%w", err)
		} else if ok {
			return f, nil
		}
	}

	configDirs := []string{
		wd,
	}
	configNames := []string{
		strings.ToLower(cli.ProgramName),
		"." + strings.ToLower(cli.ProgramName),
	}
	extensions := []string{
		"toml",
	}

	// This is crazy.
	for _, d := range configDirs {
		for _, n := range configNames {
			for _, e := range extensions {
				f = filepath.Join(d, fmt.Sprintf("%s.%s", n, e))
				if ok, err := checkFile(f); err != nil {
					return "", fmt.Errorf("%w", err)
				} else if ok {
					return f, nil
				}
			}
		}
	}

	return "", fmt.Errorf("%w", errConfigFileNotFound)
}

// checkFile checks it the given file exists. It returns an error if there is an
// error other than [fs.ErrNotExist].
func checkFile(f string) (bool, error) {
	if _, err := os.Stat(f); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("%w", err)
	}

	return true, nil
}

// defaultConfig returns the default configuration for the program.
func defaultConfig() *Config {
	return &Config{
		Logging: LoggingConfig{
			Enabled: true,
			Format:  "text",
			Level:   slog.LevelDebug,
			Output:  "stdout",
		},
	}
}
