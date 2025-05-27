package config

import (
	"context"
	"encoding"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/anttikivi/reginald/internal/flags"
	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"
)

// Errors returned from the configuration parser.
var (
	errConfigFileNotFound = errors.New("config file not found")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals // used like constant
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// A valueParser is a helper type that holds the current values for the config
// value that is currently being parsed.
type valueParser struct {
	// flagSet is the flag used for checking the values.
	flagSet *flags.FlagSet

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

	// flagName is the name of the command-line flag for checking the value for
	// the current field.
	flagName string
}

// LogValue implements [slog.LogValuer] for [valueParser]. It returns a group
// containing the fields of the parser that are relevant for logging.
func (p *valueParser) LogValue() slog.Value {
	var attrs []slog.Attr

	if p.flagSet == nil {
		attrs = append(attrs, slog.String("flagSet", "nil"))
	} else {
		attrs = append(attrs, slog.String("flagSet", "set"))
	}

	pluginNames := make([]string, 0, len(p.plugins))

	for _, plugin := range p.plugins {
		pluginNames = append(pluginNames, plugin.Name)
	}

	attrs = append(attrs, slog.Any("plugins", pluginNames))
	attrs = append(
		attrs,
		slog.Group(
			"value",
			slog.String("type", p.value.Type().Name()),
			slog.Any("value", p.value.Interface()),
		),
	)
	attrs = append(
		attrs,
		slog.Group(
			"field",
			slog.String("name", p.field.Name),
			slog.String("type", p.field.Type.Name()),
		),
	)
	attrs = append(attrs, slog.String("envName", p.envName))
	attrs = append(attrs, slog.String("envValue", p.envValue))
	attrs = append(attrs, slog.String("flagName", p.flagName))

	return slog.GroupValue(attrs...)
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
	fs afero.Fs,
	flagSet *flags.FlagSet,
	plugins []*plugins.Plugin,
) (*Config, error) {
	configFile, err := resolveFile(fs, flagSet)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config file: %w", err)
	}

	logging.TraceContext(ctx, "reading config file", "path", configFile)

	data, err := configFile.Clean().ReadFile(fs)
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

	cfg := DefaultConfig()
	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct // default values for the rest
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
		flagSet:  flagSet,
		plugins:  plugins,
		value:    reflect.ValueOf(cfg).Elem(),
		field:    reflect.StructField{}, //nolint:exhaustruct // zero value wanted
		envName:  EnvPrefix,
		envValue: "",
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
			flagSet:  parent.flagSet,
			plugins:  parent.plugins,
			value:    parent.value.Field(i),
			field:    parent.value.Type().Field(i),
			envName:  "",
			envValue: "",
			flagName: "",
		}
		parser.envName = toEnv(parser.field.Name, parent.envName)
		parser.envValue = os.Getenv(parser.envName)
		parser.flagName = toFlag(parser.field.Name)

		logging.TraceContext(ctx, "checking config field", "parser", parser)

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

		switch parser.value.Kind() { //nolint:exhaustive // TODO: implemented as needed
		case reflect.Bool:
			err = parser.setBool()
		case reflect.Int:
			err = parser.setInt()
		case reflect.String:
			if parser.value.Type().Name() == "Path" {
				err = parser.setPath()
			} else {
				err = parser.setString()
			}
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

		logging.TraceContext(ctx, "config value set", "parser", parser)
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

// setBool sets a boolean value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *valueParser) setBool() error {
	var err error

	x := p.value.Bool()

	if p.envValue != "" {
		if reflect.PointerTo(p.value.Type()).Implements(textUnmarshalerType) {
			unmarshaler, ok := p.value.Addr().Interface().(encoding.TextUnmarshaler)
			if !ok {
				panic(fmt.Sprintf("casting field %q to TextUnmarshaler", p.field.Name))
			}

			err = unmarshaler.UnmarshalText([]byte(p.envValue))
		} else {
			x, err = strconv.ParseBool(p.envValue)
		}
	}

	if err != nil {
		return fmt.Errorf("%s=%q, %w", p.envName, p.envValue, err)
	}

	if p.flagSet.Changed(p.flagName) {
		x, err = p.flagSet.GetBool(p.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}
	}

	p.value.SetBool(x)

	return nil
}

// setInt sets an integer value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *valueParser) setInt() error {
	var err error

	x := p.value.Int()

	if p.envValue != "" {
		if reflect.PointerTo(p.value.Type()).Implements(textUnmarshalerType) {
			unmarshaler, ok := p.value.Addr().Interface().(encoding.TextUnmarshaler)
			if !ok {
				panic(fmt.Sprintf("casting field %q to TextUnmarshaler", p.field.Name))
			}

			err = unmarshaler.UnmarshalText([]byte(p.envValue))
		} else {
			x, err = strconv.ParseInt(p.envValue, 10, 0)
		}
	}

	if err != nil {
		return fmt.Errorf("%s=%q, %w", p.envName, p.envValue, err)
	}

	if p.flagSet.Changed(p.flagName) {
		var n int

		n, err = p.flagSet.GetInt(p.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}

		x = int64(n)
	}

	p.value.SetInt(x)

	return nil
}

// setPath sets a string value from the environment variable or
// the command-line flag to the currently parsed value as an [fspath.Path]. It
// also cleans the path and possibly makes it absolute.
func (p *valueParser) setPath() error {
	var err error

	x := fspath.Path(p.value.String())

	if p.envValue != "" {
		x = fspath.Path(p.envValue)
	}

	if p.flagSet.Changed(p.flagName) {
		x, err = p.flagSet.GetPath(p.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}
	}

	x, err = x.Abs()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	p.value.SetString(string(x))

	return nil
}

// setString sets a string value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *valueParser) setString() error {
	x := p.value.String()

	if p.envValue != "" {
		x = p.envValue
	}

	if p.flagSet.Changed(p.flagName) {
		var err error

		x, err = p.flagSet.GetString(p.flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}
	}

	p.value.SetString(x)

	return nil
}
