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
	errInvalidEnvVar      = errors.New("invalid config value in environment variable")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// A valueParser is a helper type that holds the current values for the config
// value that is currently being parsed.
type valueParser struct {
	// fs is the flag used for checking the values.
	fs *flags.FlagSet

	// plugins are the loaded plugins.
	plugins []*plugins.Plugin

	// value is the value of the currently parsed field.
	value reflect.Value

	// field is the currently parsed struct field.
	field reflect.StructField

	// envName is the name of the environment variable for checking the value
	// for the current field.
	envName string

	// envValue is the value of the environment variable for the current field.
	envValue string

	// envOk tells whether the value from the environment variable has been set.
	// It is used to check if the unmarshaling using TextUnmarshaler was
	// successful and the string from the environment doesn't need manual
	// parsing.
	envOk bool

	// flagName is the name of the command-line flag for checking the value for
	// the current field.
	flagName string
}

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

	logging.TraceContext(ctx, "reading config file", "path", configFile)

	data, err := os.ReadFile(filepath.Clean(configFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	rawCfg := make(map[string]any)

	if err = toml.Unmarshal(data, &rawCfg); err != nil {
		return nil, fmt.Errorf("failed to decode the config file: %w", err)
	}

	logging.TraceContext(ctx, "unmarshaled config file", "cfg", rawCfg)
	normalizeKeys(rawCfg)
	logging.TraceContext(ctx, "normalized keys", "cfg", rawCfg)

	cfg := &Config{}                              //nolint:exhaustruct
	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     cfg,
	}

	d, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := d.Decode(rawCfg); err != nil {
		return nil, fmt.Errorf("failed to read environment variables for config: %w", err)
	}

	parser := &valueParser{
		fs:       flagSet,
		plugins:  plugins,
		value:    reflect.ValueOf(cfg).Elem(),
		field:    reflect.StructField{}, //nolint:exhaustruct
		envName:  EnvPrefix,
		envValue: "",
		envOk:    false,
		flagName: "",
	}
	if err := ApplyOverrides(ctx, parser); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logging.TraceContext(ctx, "parsed config", "cfg", cfg)

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

		if k != key {
			delete(cfg, k)

			cfg[key] = v
		}

		if m, ok := v.(map[string]any); ok {
			normalizeKeys(m)
		}
	}
}

// ApplyOverrides applies the overrides of the config values from environment
// variables and command-line flags to cfg. It modifies the pointed cfg.
func ApplyOverrides(ctx context.Context, parent *valueParser) error {
	for i := range parent.value.NumField() {
		var err error

		// TODO: Check the struct tags for env and flags.
		parser := &valueParser{
			fs:       parent.fs,
			plugins:  parent.plugins,
			value:    parent.value.Field(i),
			field:    parent.value.Type().Field(i),
			envName:  "",
			envValue: "",
			envOk:    false,
			flagName: "",
		}
		parser.envName = toEnv(parser.field.Name, parent.envName)
		parser.envValue = os.Getenv(parser.envName)
		parser.flagName = toFlag(parser.field.Name)

		logging.TraceContext(
			ctx,
			"checking config field",
			"name",
			parser.field.Name,
			"parser",
			parser,
		)

		if !parser.value.CanSet() {
			continue
		}

		if parser.field.Name == "Plugins" || parser.field.Name == "Tasks" {
			continue
		}

		if parser.value.Kind() == reflect.Struct {
			// Apply the overrides recursively but set the plugins to nil as
			// only the top-level config has the map for config values.
			err = ApplyOverrides(ctx, parser)
			if err != nil {
				return fmt.Errorf("%w", err)
			}

			continue
		}

		err = setConfigField(ctx, parser)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		logging.TraceContext(
			ctx,
			"value checked",
			"name",
			parser.field.Name,
			"value",
			parser.value.Interface(),
			"parser",
			parser,
		)
	}

	// TODO: Apply the plugin values to the plugin fields as it is not done
	// using the reflection.
	if len(parent.plugins) == 0 {
		return nil
	}

	return nil
}

// toEnv converts a struct field from camel case to snake case and upper case in
// order to make the resulting environment variable names more natural.
func toEnv(name, prefix string) string {
	result := ""

	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result += "_"
		}

		result += string(r)
	}

	return strings.ToUpper(fmt.Sprintf("%s_%s", prefix, result))
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

// setConfigField sets the value of the given config field in the struct from
// environment variable or command-line flag.
func setConfigField(ctx context.Context, parser *valueParser) error {
	var err error

	if parser.envValue != "" {
		parser.envOk, err = tryUnmarshalText(ctx, parser.value, parser.field, parser.envValue)
		if err != nil {
			return fmt.Errorf(
				"failed to unmarshal text from %s=%q: %w",
				parser.envName,
				parser.envValue,
				err,
			)
		}
	}

	// TODO: Implement the types as they are needed.
	switch parser.value.Kind() { //nolint:exhaustive
	case reflect.Bool:
		err = setBool(parser)
	case reflect.Int:
		err = setInt(parser)
	case reflect.String:
		err = setString(parser)
	case reflect.Struct:
		panic(
			fmt.Sprintf(
				"reached the struct check when converting environment variable to config value in %s: %s",
				parser.field.Name,
				parser.value.Kind(),
			),
		)
	default:
		panic(
			fmt.Sprintf(
				"unsupported config field type for %s: %s",
				parser.field.Name,
				parser.value.Kind(),
			),
		)
	}

	if err != nil {
		return fmt.Errorf("failed to set config value: %w", err)
	}

	return nil
}

// tryUnmarshalText checks if it can use [encoding.TextUnmarshaler] to unmarshal
// the given value and set it to the field. The first return value tells whether
// this was successful and the second is error.
func tryUnmarshalText(
	ctx context.Context,
	fv reflect.Value,
	sf reflect.StructField,
	val string,
) (bool, error) {
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

		logging.TraceContext(ctx, "unmarshaled value", "value", fv.Interface())

		return true, nil
	}

	return false, nil
}

// setBool set a boolean value from the environment variable or the command-line
// flag to the currently parsed value.
func setBool(parser *valueParser) error {
	var (
		err error
		x   bool
	)

	changed := false

	if !parser.envOk && parser.envValue != "" {
		switch strings.ToLower(strings.TrimSpace(parser.envValue)) {
		case "true", "1":
			x = true
		case "false", "0":
			x = false
		default:
			return fmt.Errorf("%w: %s=%q", errInvalidEnvVar, parser.envName, parser.envValue)
		}

		changed = true
	}

	if parser.fs.Changed(parser.flagName) {
		x, err = parser.fs.GetBool(parser.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", parser.flagName, err)
		}

		changed = true
	}

	if changed {
		parser.value.SetBool(x)
	}

	return nil
}

// setInt set an integer value from the environment variable or the command-line
// flag to the currently parsed value.
func setInt(parser *valueParser) error {
	var (
		err error
		x   int64
	)

	changed := false

	if !parser.envOk && parser.envValue != "" {
		x, err = strconv.ParseInt(parser.envValue, 10, 0)
		if err != nil {
			return fmt.Errorf("%w: %s=%q", errInvalidEnvVar, parser.envName, parser.envValue)
		}

		changed = true
	}

	if parser.fs.Changed(parser.flagName) {
		var i int

		i, err = parser.fs.GetInt(parser.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", parser.flagName, err)
		}

		x = int64(i)

		changed = true
	}

	if changed {
		parser.value.SetInt(x)
	}

	return nil
}

// setString set a string value from the environment variable or
// the command-line flag to the currently parsed value.
func setString(parser *valueParser) error {
	var (
		err error
		x   string
	)

	changed := false

	if !parser.envOk && parser.envValue != "" {
		x = parser.envValue
		changed = true
	}

	if parser.fs.Changed(parser.flagName) {
		x, err = parser.fs.GetString(parser.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", parser.flagName, err)
		}

		changed = true
	}

	if changed {
		parser.value.SetString(x)
	}

	return nil
}
