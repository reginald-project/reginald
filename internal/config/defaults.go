package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anttikivi/reginald/internal/pathname"
	"golang.org/x/term"
)

const (
	defaultEnvPrefix = "REGINALD" // prefix used for the environment variables.
	defaultFileName  = "reginald"
	defaultLogOutput = "~/.local/state/reginald.log"
)

// DefaultDirectory returns the default working directory for the program. It
// panics on errors.
func DefaultDirectory() string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get the current working directory: %v", err))
	}

	return pwd
}

// DefaultPluginsDir returns the plugins directory to use. It takes the environment
// variable for customizing the plugins directory and the platform into account.
func DefaultPluginsDir() (string, error) {
	name := strings.ToUpper(defaultEnvPrefix + "_PLUGIN_DIR")
	if env := os.Getenv(name); env != "" {
		path, err := pathname.Abs(env)
		if err != nil {
			return "", fmt.Errorf(
				"failed to convert plugins directory path %q to absolute path: %w",
				env,
				err,
			)
		}

		return path, nil
	}

	path, err := defaultPluginsDir()
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return filepath.Clean(path), nil
}

// defaultConfig returns the default configuration for the program. It does not
// include default values for fields that get their default values from
// [File].
func defaultConfig() *Config {
	return &Config{ //nolint:exhaustruct // the defaults are passed from the default file
		ConfigFile: "",
		Directory:  "",
	}
}

// defaultConfigFileValue returns the default values for the configuration
// options that can be set using a config file.
func defaultConfigFileValue() *File {
	pd, err := DefaultPluginsDir()
	if err != nil {
		panic(fmt.Sprintf("failed to create value for the default config file: %v", err))
	}

	return &File{
		Color: term.IsTerminal(int(os.Stdout.Fd())),
		Logging: LoggingConfig{
			Enabled: true,
			Format:  "json",
			Level:   slog.LevelInfo,
			Output:  defaultLogOutput,
		},
		PluginDir: pd,
		Quiet:     false,
		Tasks:     []map[string]any{},
		Verbose:   false,
	}
}
