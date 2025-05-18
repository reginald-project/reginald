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
	defaultLogOutput = "~/.local/state/reginald.log"
	defaultDirName   = "reginald"
)

// PluginsDir returns the plugins directory to use. It takes the environment
// variable for customizing the plugins directory and the platform into account.
func PluginsDir() (string, error) {
	name := strings.ToUpper(envPrefix + "_PLUGINS_DIR")
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

// defaultConfigFile returns the default values for the configuration options
// that can be set using a config file.
func defaultConfigFile() *File {
	return &File{
		Color: term.IsTerminal(int(os.Stdout.Fd())),
		Logging: LoggingConfig{
			Enabled: true,
			Format:  "json",
			Level:   slog.LevelInfo,
			Output:  defaultLogOutput,
		},
		Quiet:   false,
		Tasks:   []map[string]any{},
		Verbose: false,
	}
}
