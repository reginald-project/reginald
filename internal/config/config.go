// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"fmt"
	"log/slog"
	"reflect"
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

// from creates a new [Config] by creating one with default values and then
// applying all of the found values from f.
func (f *File) from() *Config {
	cfg := defaultConfig()
	cfgValue := reflect.ValueOf(cfg).Elem()
	cfgFile := reflect.ValueOf(f).Elem()

	applyFileValues(cfgFile, cfgValue)

	return cfg
}

// applyFileValues applies the configuration values from the value of
// [File] given as the first parameter to the value [Config] given as
// the second parameter. It calls itself recursively to resolve structs. It
// panics if there is an error.
func applyFileValues(cfgFile, cfg reflect.Value) {
	for i := range cfgFile.NumField() {
		fieldValue := cfgFile.Field(i)
		structField := cfgFile.Type().Field(i)
		target := cfg.FieldByName(structField.Name)

		if !target.IsValid() || !target.CanSet() {
			panic("target value in Config cannot be set: " + structField.Name)
		}

		slog.Debug("checking config file field", "field", structField.Name)

		if fieldValue.Kind() == reflect.Struct {
			applyFileValues(fieldValue, target)

			continue
		}

		if !fieldValue.Type().AssignableTo(target.Type()) {
			panic(
				fmt.Sprintf(
					"config file value from field %q is not assignable to config",
					structField.Name,
				),
			)
		}

		target.Set(fieldValue)
	}
}
