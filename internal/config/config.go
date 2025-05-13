// Package config contains the program configuration. The configuration is
// parsed from the configuration file, environment variables, and command-line
// arguments.
package config

import (
	"encoding"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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

// Errors returned from the configuration parser.
var (
	errConfigFileNotFound = errors.New("config file not found")
	errInvalidEnvVar      = errors.New("invalid config value in environment variable")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

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

	if err = applyEnv(config); err != nil {
		return nil, fmt.Errorf("failed to read environment variables for config: %w", err)
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

// applyEnv applies the overrides of the configuration values from environment
// variables to cfg. It modifies the pointed cfg.
func applyEnv(cfg *Config) error {
	v := reflect.ValueOf(cfg).Elem()

	if err := unmarshalEnv(v, cli.ProgramName); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// unmarshalEnv is the implementation for applying the environment variables
// into the configuration struct. It reads the struct fields from v and checks
// if there is an environment variable with the name <PREFIX>_<FIELD NAME> (all
// upper case). If a variable with that name is set, the function tries to set
// the struct value accordingly. It prioritizes [encoding.TextUnmarshaler] if
// the type implements that; otherwise it tries to manually find the correct
// value. This function may panic. It returns an error when the user has given
// an invalid value for the variable.
//
// TODO: Allow using struct tags for specifying the name of the variable.
func unmarshalEnv(v reflect.Value, prefix string) error {
	for i := range v.NumField() {
		fieldValue := v.Field(i)
		structField := v.Type().Field(i)

		if !fieldValue.CanSet() {
			continue
		}

		slog.Debug("checking config field", "field", structField.Name)

		env := strings.ToUpper(fmt.Sprintf("%s_%s", prefix, structField.Name))

		if fieldValue.Kind() == reflect.Struct {
			if err := unmarshalEnv(fieldValue, env); err != nil {
				return fmt.Errorf("%w", err)
			}

			continue
		}

		slog.Debug("reading config value from env", "var", env)

		if val := os.Getenv(env); val != "" {
			ok, err := tryUnmarshalText(fieldValue, structField, val)
			if ok {
				continue
			}

			if err != nil {
				return fmt.Errorf("failed to unmarshal text from %s=%q: %w", env, val, err)
			}

			// TODO: Implement the types as they are needed.
			switch fieldValue.Kind() { //nolint:exhaustive
			case reflect.Bool:
				switch strings.ToLower(strings.TrimSpace(val)) {
				case "true", "1":
					fieldValue.SetBool(true)
				case "false", "0":
					fieldValue.SetBool(false)
				default:
					return fmt.Errorf("%w: %s=%q", errInvalidEnvVar, env, val)
				}
			case reflect.String:
				fieldValue.SetString(val)
			case reflect.Struct:
				panic(
					fmt.Sprintf(
						"reached the struct check when converting environment variable to config value in %s: %s",
						structField.Name,
						fieldValue.Kind(),
					),
				)
			default:
				panic(
					fmt.Sprintf(
						"unsupported config field type for %s: %s",
						structField.Name,
						fieldValue.Kind(),
					),
				)
			}
		}
	}

	return nil
}

// tryUnmarshalText checks if it can use [encoding.TextUnmarshaler] to unmarshal
// the given value and set it to the field. The first return value tells whether
// this was successful and the second is error.
func tryUnmarshalText(fv reflect.Value, sf reflect.StructField, val string) (bool, error) {
	if reflect.PointerTo(fv.Type()).Implements(textUnmarshalerType) {
		slog.Debug("pointer to field implements TextUnmarshaler", "field", sf.Name)

		unmarshaler, ok := fv.Addr().Interface().(encoding.TextUnmarshaler)
		if !ok {
			panic(
				fmt.Sprintf(
					"failed to cast field %q to encoding.TextUnmarshaler",
					sf.Name,
				),
			)
		}

		if err := unmarshaler.UnmarshalText([]byte(val)); err != nil {
			return false, fmt.Errorf("%w", err)
		}

		return true, nil
	}

	return false, nil
}
