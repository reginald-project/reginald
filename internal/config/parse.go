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

	cf := defaultConfigFile()
	if err = toml.Unmarshal(data, cf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the config file: %w", err)
	}

	cfg := from(cf)

	if err = applyEnv(cfg); err != nil {
		return nil, fmt.Errorf("failed to read environment variables for config: %w", err)
	}

	applyFlags(cfg, fs)

	if err = validate(cfg); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return cfg, nil
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

// normalize normalizes the configuration values so that they have a predictable
// format for use later in the program. For example, this includes making paths
// absolute.
func normalize(_ *Config) error { //nolint:unused
	return nil
}

// validate checks the configuration values and an error if there is an invalid
// combination of configuration values.
func validate(c *Config) error {
	if c.Quiet && c.Verbose {
		return fmt.Errorf("%w: both quiet and verbose are set", errInvalidConfig)
	}

	return nil
}

// applyFlags applies the overrides of the configuration values from
// command-line flags. It modifies cfg.
func applyFlags(c *Config, fs *pflag.FlagSet) {
	if fs.Changed("logging") {
		b, err := fs.GetBool("logging")
		if err != nil {
			panic("failed to get the value for --logging")
		}

		c.Logging.Enabled = b
	}

	if fs.Changed("no-logging") {
		b, err := fs.GetBool("no-logging")
		if err != nil {
			panic("failed to get the value for --no-logging")
		}

		c.Logging.Enabled = !b
	}

	if fs.Changed("quiet") {
		b, err := fs.GetBool("quiet")
		if err != nil {
			panic("failed to get the value for --quiet")
		}

		c.Quiet = b
	}

	if fs.Changed("verbose") {
		b, err := fs.GetBool("verbose")
		if err != nil {
			panic("failed to get the value for --verbose")
		}

		c.Quiet = b
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

// from creates a new [Config] by creating one with default values and then
// applying all of the found values from f.
func from(f *File) *Config {
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
