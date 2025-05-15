package config

import "log/slog"

const defaultLogOutput = "~/.local/state/reginald.log"

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
		Logging: LoggingConfig{
			Enabled: true,
			Format:  "json",
			Level:   slog.LevelInfo,
			Output:  defaultLogOutput,
		},
		PluginDir: "~/.local/share/reginald/plugins",
		Quiet:     false,
		Tasks:     []map[string]any{},
		Verbose:   false,
	}
}
