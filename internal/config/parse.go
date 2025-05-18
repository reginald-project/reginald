package config

import (
	"encoding"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/anttikivi/reginald/internal/pathname"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
)

// Constants for parsing the configuration.
const (
	defaultFile = "reginald" // default name of the config file without the possible leading dot and type extension
	envPrefix   = "REGINALD" // prefix used for the environment variables.
)

// Errors returned from the configuration parser.
var (
	errConfigFileNotFound = errors.New("config file not found")
	errInvalidConfig      = errors.New(
		"invalid configuration",
	) // if there is an invalid combination of config values
	errInvalidEnvVar = errors.New("invalid config value in environment variable")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// Parse parses the configuration according to the configuration given with
// flagSet. The flag set should contain all of the flags for the program as the
// function uses the flags to override values from the configuration file. The
// function returns a pointer to the parsed configuration and any errors it
// encounters.
//
// The function also resolves the configuration file according to the standard
// paths for the file or according the flags. The relevant flags are
// `--directory` and `--config`.
func Parse(flagSet *pflag.FlagSet) (*Config, error) {
	dir, err := flagSet.GetString("directory")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get the value for command-line option '--directory': %w",
			err,
		)
	}

	if !filepath.IsAbs(dir) {
		dir, err = pathname.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to make the working directory absolute: %w", err)
		}
	}

	filename, err := flagSet.GetString("config")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get the value for command-line option '--config': %w",
			err,
		)
	}

	configFile, err := resolveFile(dir, filename)
	if err != nil {
		return nil, fmt.Errorf("searching for config file failed: %w", err)
	}

	slog.Debug("resolved config file path", "path", configFile)

	f, err := os.Open(filepath.Clean(configFile))
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}

	d := toml.NewDecoder(f).DisallowUnknownFields()
	cf := defaultConfigFile()

	if err = d.Decode(cf); err != nil {
		var strictMissingError *toml.StrictMissingError
		if !errors.As(err, &strictMissingError) {
			panic(
				fmt.Sprintf(
					"err should have been a *toml.StrictMissingError, but got %s (%T)",
					err,
					err,
				),
			)
		}

		return nil, fmt.Errorf(
			"failed to decode the config file: %w\n%s",
			strictMissingError,
			strictMissingError.String(),
		)
	}

	cfg, err := cf.from()
	if err != nil {
		return nil, fmt.Errorf("failed to create config from the file data: %w", err)
	}

	if err = applyEnv(cfg); err != nil {
		return nil, fmt.Errorf("failed to read environment variables for config: %w", err)
	}

	applyFlags(cfg, flagSet)

	cfg.ConfigFile = configFile
	cfg.Directory = dir

	if err = normalize(cfg); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if err = validate(cfg); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return cfg, nil
}

// resolveFile looks up the possible paths for the configuration file and
// returns the first one that contains a file with a valid name. The returned
// path is absolute. If no configuration file is found, the function returns an
// empty string and an error.
func resolveFile(wd, file string) (string, error) {
	original := file

	slog.Debug("resolving config file", "wd", wd, "f", file)

	// Use the config file f if it is an absolute path.
	if filepath.IsAbs(file) {
		slog.Debug("given config file is an absolute path", "f", file)

		if ok, err := pathname.IsFile(file); err != nil {
			return "", fmt.Errorf("%w", err)
		} else if ok {
			slog.Debug("found config file", "f", file)

			return filepath.Clean(file), nil
		}
	}

	// Check if the config file f matches a file in the working directory.
	file = filepath.Join(wd, file)

	slog.Debug("checking joint path", "f", file)

	if ok, err := pathname.IsFile(file); err != nil {
		return "", fmt.Errorf("%w", err)
	} else if ok {
		slog.Debug("found config file", "f", file)

		return file, nil
	}

	// If the config file flag is set but it didn't resolve, fail so that the
	// program doesn't use a config file from some other location by surprise.
	if original != "" {
		return "", fmt.Errorf(
			"%w: value given with flag --config did not resolve to any file",
			errConfigFileNotFound,
		)
	}

	// TODO: Add more locations.
	configDirs := []string{
		wd,
	}
	configNames := []string{
		strings.ToLower(defaultFile),
		"." + strings.ToLower(defaultFile),
	}
	extensions := []string{
		"toml",
	}

	// This is crazy.
	for _, d := range configDirs {
		for _, n := range configNames {
			for _, e := range extensions {
				file = filepath.Join(d, fmt.Sprintf("%s.%s", n, e))

				slog.Debug("checking default path", "f", file)

				if ok, err := pathname.IsFile(file); err != nil {
					return "", fmt.Errorf("%w", err)
				} else if ok {
					slog.Debug("found config file", "f", file)

					return file, nil
				}
			}
		}
	}

	return "", fmt.Errorf("%w", errConfigFileNotFound)
}

// normalize normalizes the configuration values so that they have a predictable
// format for use later in the program. For example, this includes making paths
// absolute.
func normalize(cfg *Config) error {
	var err error

	// Logging file path should be absolute to safely use it later.
	if cfg.Logging.Output != "stderr" && cfg.Logging.Output != "stdout" &&
		!filepath.IsAbs(cfg.Logging.Output) {
		cfg.Logging.Output, err = pathname.Abs(cfg.Logging.Output)
		if err != nil {
			return fmt.Errorf("failed to make the log file path absolute: %w", err)
		}
	}

	return nil
}

// validate checks the configuration values and an error if there is an invalid
// combination of configuration values.
func validate(c *Config) error {
	if !filepath.IsAbs(c.ConfigFile) {
		return fmt.Errorf("%w: config file is not absolute: %s", errInvalidConfig, c.ConfigFile)
	}

	if !filepath.IsAbs(c.Directory) {
		return fmt.Errorf(
			"%w: working directory is not absolute: %s",
			errInvalidConfig,
			c.Directory,
		)
	}

	if c.Quiet && c.Verbose {
		return fmt.Errorf("%w: both quiet and verbose are set", errInvalidConfig)
	}

	return nil
}

// applyFlags applies the overrides of the configuration values from
// command-line flags. It modifies cfg.
func applyFlags(cfg *Config, fs *pflag.FlagSet) {
	if fs.Changed("color") {
		b, err := fs.GetBool("color")
		if err != nil {
			panic("failed to get the value for --color")
		}

		cfg.Color = b
	}

	if fs.Changed("no-color") {
		b, err := fs.GetBool("no-color")
		if err != nil {
			panic("failed to get the value for --no-color")
		}

		cfg.Color = !b
	}

	if fs.Changed("logging") {
		b, err := fs.GetBool("logging")
		if err != nil {
			panic("failed to get the value for --logging")
		}

		cfg.Logging.Enabled = b
	}

	if fs.Changed("no-logging") {
		b, err := fs.GetBool("no-logging")
		if err != nil {
			panic("failed to get the value for --no-logging")
		}

		cfg.Logging.Enabled = !b
	}

	if fs.Changed("quiet") {
		b, err := fs.GetBool("quiet")
		if err != nil {
			panic("failed to get the value for --quiet")
		}

		cfg.Quiet = b
	}

	if fs.Changed("verbose") {
		b, err := fs.GetBool("verbose")
		if err != nil {
			panic("failed to get the value for --verbose")
		}

		cfg.Quiet = b
	}
}

// applyEnv applies the overrides of the configuration values from environment
// variables to cfg. It modifies the pointed cfg.
func applyEnv(cfg *Config) error {
	v := reflect.ValueOf(cfg).Elem()

	if err := unmarshalEnv(v, envPrefix); err != nil {
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
