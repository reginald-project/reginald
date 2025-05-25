package config

import (
	"context"
	"encoding"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/anttikivi/reginald/internal/flags"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
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
func Parse(
	ctx context.Context,
	flagSet *flags.FlagSet,
	plugins []*plugins.Plugin,
) (*Config, error) {
	configFile, err := resolveFile(flagSet)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config file: %w", err)
	}

	data, err := os.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	rawCfg := make(map[string]any)

	if err = toml.Unmarshal(data, &rawCfg); err != nil {
		return nil, fmt.Errorf("failed to decode the config file: %w", err)
	}

	normalizeKeys(rawCfg)

	cfg := &Config{}

	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     cfg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := d.Decode(rawCfg); err != nil {
		return nil, fmt.Errorf("failed to read environment variables for config: %w", err)
	}

	fmt.Println(cfg)

	v := reflect.ValueOf(cfg).Elem()
	if err := ApplyOverrides(ctx, cfg, v, EnvPrefix, flagSet, plugins); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return cfg, nil
}

// normalizeKeys checks the config value keys in the given raw config map and
// changes them into the wanted format ("kebab-case") in case the config
// contains something in "camel-case". This way the config file is able to
// support JSON and YAML while allowing those files to have the keys more
// idiomatic for those formats.
func normalizeKeys(cfg map[string]any) {
	if cfg == nil {
		return
	}

	for k, v := range cfg {
		key := ""

		for i, r := range k {
			if r == '-' {
				key += "_"

				continue
			}

			if i > 0 && unicode.IsUpper(r) && !strings.HasPrefix(key, "_") {
				key += "_"
			}

			key += strings.ToLower(string(r))
		}

		cfg[key] = v

		delete(cfg, k)

		if m, ok := v.(map[string]any); ok {
			normalizeKeys(m)
		}
	}
}

// ApplyOverrides applies the overrides of the config values from environment
// variables and command-line flags to cfg. It modifies the pointed cfg.
func ApplyOverrides(
	ctx context.Context,
	cfg *Config,
	v reflect.Value,
	envPrefix string,
	flagSet *flags.FlagSet,
	plugins []*plugins.Plugin,
) error {
	for i := range v.NumField() {
		var err error

		value := v.Field(i)
		field := v.Type().Field(i)

		logging.TraceContext(ctx, "checking config field", "field", field.Name)

		if !value.CanSet() {
			continue
		}

		if field.Name == "Plugins" || field.Name == "Tasks" {
			continue
		}

		varname := strings.ToUpper(fmt.Sprintf("%s_%s", envPrefix, toEnv(field.Name)))

		if value.Kind() == reflect.Struct {
			// Apply the overrides recursively but set the plugins to nil as
			// only the top-level config has the map for config values.
			if err = ApplyOverrides(ctx, cfg, value, varname, flagSet, nil); err != nil {
				return fmt.Errorf("%w", err)
			}

			continue
		}

		// TODO: Check the struct tags.
		flagName := toFlag(field.Name)
		val := os.Getenv(varname)
		ok := false

		if val != "" {
			ok, err = tryUnmarshalText(value, field, val)
			if err != nil {
				return fmt.Errorf("failed to unmarshal text from %s=%q: %w", varname, val, err)
			}
		}

		checkEnv := !ok && val != ""

		// TODO: Implement the types as they are needed.
		switch value.Kind() { //nolint:exhaustive
		case reflect.Bool:
			var x bool
			changed := false

			if checkEnv {
				switch strings.ToLower(strings.TrimSpace(val)) {
				case "true", "1":
					x = true
				case "false", "0":
					x = false
				default:
					return fmt.Errorf("%w: %s=%q", errInvalidEnvVar, varname, val)
				}

				changed = true
			}

			if flagSet.Changed(flagName) {
				x, err = flagSet.GetBool(flagName)
				if err != nil {
					return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
				}

				changed = true
			}

			if changed {
				value.SetBool(x)
			}
		case reflect.Int:
			var x int64
			changed := false

			if checkEnv {
				x, err = strconv.ParseInt(val, 10, 0)
				if err != nil {
					return fmt.Errorf("%w: %s=%q", errInvalidEnvVar, varname, val)
				}

				changed = true
			}

			if flagSet.Changed(flagName) {
				x, err = flagSet.GetInt64(flagName)
				if err != nil {
					return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
				}

				changed = true
			}

			if changed {
				value.SetInt(x)
			}
		case reflect.String:
			var x string
			changed := false

			if checkEnv {
				x = val
				changed = true
			}

			if flagSet.Changed(flagName) {
				x, err = flagSet.GetString(flagName)
				if err != nil {
					return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
				}

				changed = true
			}

			if changed {
				value.SetString(x)
			}
		case reflect.Struct:
			panic(
				fmt.Sprintf(
					"reached the struct check when converting environment variable to config value in %s: %s",
					field.Name,
					value.Kind(),
				),
			)
		default:
			panic(fmt.Sprintf("unsupported config field type for %s: %s", field.Name, value.Kind()))
		}
	}

	// TODO: Apply the plugin values to the plugin fields as it is not done
	// using the reflection.
	if len(plugins) == 0 {
		return nil
	}

	return nil
}

// toEnv converts a struct field from camel case to snake case and upper case in
// order to make the resulting environment variable names more natural.
func toEnv(name string) string {
	result := ""

	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result += "_"
		}

		result += string(r)
	}

	return strings.ToUpper(result)
}

// toFlag converts a struct field from camel case to lower case and "kebab-case"
// in order to have it match command-line flag name if no flag name is provided
// manually.
func toFlag(name string) string {
	result := ""

	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result += "-"
		}

		result += string(r)
	}

	return strings.ToLower(result)
}

// tryUnmarshalText checks if it can use [encoding.TextUnmarshaler] to unmarshal
// the given value and set it to the field. The first return value tells whether
// this was successful and the second is error.
func tryUnmarshalText(fv reflect.Value, sf reflect.StructField, val string) (bool, error) {
	if reflect.PointerTo(fv.Type()).Implements(textUnmarshalerType) {
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
