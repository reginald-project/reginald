// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"log/slog"
)

// Config the parsed configuration of the program run. There should be only one
// effective Config per run.
type Config struct {
	ConfigFile string        // path to the config file
	Directory  string        // path to the directory passed in with '-C'
	Logging    LoggingConfig // logging config values
	Quiet      bool          // whether only errors are output
	Verbose    bool          // whether verbose output is enabled
}

// LoggingConfig is type of the logging configuration in Config.
type LoggingConfig struct {
	Enabled bool       // whether logging is enabled
	Format  string     // format of the logs, "json" or "text"
	Level   slog.Level // logging level
	Output  string     // destination of the logs
}

// A File is a struct that the represents the structure of a valid configuration
// file. It is a subset of [Config]. Some of the configuration values may not
// be set using the file so the file is first unmarshaled into a File and the
// values are read into [Config].
type File struct {
	Logging LoggingConfig
	Quiet   bool
	Verbose bool
}

// Equal reports if the Config that d points to is equal to the Config that c
// points to.
func (c *Config) Equal(d *Config) bool {
	if c == d {
		return true
	}

	if c == nil {
		return d == nil
	}

	if d == nil {
		return c == nil
	}

	// TODO: This must be fixed if the struct contains fields that are not
	// comparable.
	return *c == *d
}
