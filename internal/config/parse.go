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
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"
)

// Errors returned from the configuration parser.
var (
	errConfigFileNotFound = errors.New("config file not found")
	errInvalidCast        = errors.New("cannot convert type")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals // used like constant
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// A ValueParser is a helper type that holds the current values for the config
// value that is currently being parsed.
type ValueParser struct {
	// FlagSet is the flag used for checking the values.
	FlagSet *flags.FlagSet

	// Plugins are the loaded Plugins.
	Plugins []*plugins.Plugin

	// Value is the Value of the currently parsed field.
	Value reflect.Value

	// Field is the currently parsed struct Field.
	Field reflect.StructField

	// FullName is the name of the field including the names of the parent
	// fields before it separated by dots.
	FullName string

	// EnvName is the name of the environment variable for checking the value
	// for the current field.
	EnvName string

	// EnvValue is the value of the environment variable for the current field.
	EnvValue string

	// FlagName is the name of the command-line flag for checking the value for
	// the current field.
	FlagName string
}

// LogValue implements [slog.LogValuer] for [valueParser]. It returns a group
// containing the fields of the parser that are relevant for logging.
func (p *ValueParser) LogValue() slog.Value {
	var attrs []slog.Attr

	if p.FlagSet == nil {
		attrs = append(attrs, slog.String("flagSet", "nil"))
	} else {
		attrs = append(attrs, slog.String("flagSet", "set"))
	}

	pluginNames := make([]string, 0, len(p.Plugins))

	for _, plugin := range p.Plugins {
		pluginNames = append(pluginNames, plugin.Name)
	}

	attrs = append(attrs, slog.Any("plugins", pluginNames))
	attrs = append(
		attrs,
		slog.Group(
			"value",
			slog.String("type", p.Value.Type().Name()),
			slog.Any("value", p.Value.Interface()),
		),
	)
	attrs = append(
		attrs,
		slog.Group(
			"field",
			slog.String("name", p.Field.Name),
			slog.String("type", p.Field.Type.Name()),
		),
	)
	attrs = append(attrs, slog.String("FullName", p.FullName))
	attrs = append(attrs, slog.String("envName", p.EnvName))
	attrs = append(attrs, slog.String("envValue", p.EnvValue))
	attrs = append(attrs, slog.String("flagName", p.FlagName))

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

	parser := &ValueParser{
		FlagSet:  flagSet,
		Plugins:  plugins,
		Value:    reflect.ValueOf(cfg).Elem(),
		Field:    reflect.StructField{}, //nolint:exhaustruct // zero value wanted
		FullName: "",
		EnvName:  EnvPrefix,
		EnvValue: "",
		FlagName: "",
	}
	if err := parser.ApplyOverrides(ctx); err != nil {
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
func (p *ValueParser) ApplyOverrides(ctx context.Context) error {
	for i := range p.Value.NumField() {
		var err error

		// TODO: Check the struct tags for env and flags.
		parser := &ValueParser{
			FlagSet:  p.FlagSet,
			Plugins:  p.Plugins,
			Value:    p.Value.Field(i),
			Field:    p.Value.Type().Field(i),
			FullName: "",
			EnvName:  "",
			EnvValue: "",
			FlagName: "",
		}

		if p.FullName != "" {
			parser.FullName += p.FullName + "."
		}

		parser.FullName += parser.Field.Name

		parser.EnvName = toEnv(parser.Field.Name, p.EnvName)
		parser.EnvValue = os.Getenv(parser.EnvName)
		parser.FlagName = FlagName(parser.FullName)

		logging.TraceContext(ctx, "checking config field", "parser", parser)

		if !parser.Value.CanSet() {
			continue
		}

		if parser.Field.Name == "Plugins" || parser.Field.Name == "Tasks" {
			continue
		}

		if parser.Value.Kind() == reflect.Struct {
			// Apply the overrides recursively but set the plugins to nil as
			// only the top-level config has the map for config values.
			err = parser.ApplyOverrides(ctx)
			if err != nil {
				return fmt.Errorf("%w", err)
			}

			continue
		}

		switch parser.Value.Kind() { //nolint:exhaustive // TODO: implemented as needed
		case reflect.Bool:
			err = parser.setBool()
		case reflect.Int:
			if parser.Value.Type().Name() == "ColorMode" {
				err = parser.setColorMode()
			} else {
				err = parser.setInt()
			}
		case reflect.String:
			if parser.Value.Type().Name() == "Path" {
				err = parser.setPath()
			} else {
				err = parser.setString()
			}
		case reflect.Struct:
			panic(
				fmt.Sprintf(
					"reached the struct check when converting environment variable to config value in %s: %s",
					parser.Field.Name,
					parser.Value.Kind(),
				),
			)
		default:
			panic(
				fmt.Sprintf(
					"unsupported config field type for %s: %s",
					parser.Field.Name,
					parser.Value.Kind(),
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
	if len(p.Plugins) == 0 {
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

// setBool sets a boolean value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *ValueParser) setBool() error {
	var err error

	x := p.Value.Bool()

	if p.EnvValue != "" {
		if p.canUnmarshal() {
			v, err := p.unmarshal(p.EnvValue)
			if err != nil {
				return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
			}

			x = v.Bool()
		} else {
			x, err = strconv.ParseBool(p.EnvValue)
		}
	}

	if err != nil {
		return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
	}

	if p.FlagSet.Changed(p.FlagName) {
		x, err = p.FlagSet.GetBool(p.FlagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.FlagName, err)
		}
	}

	if HasInvertedFlagName(p.FullName) {
		inverted := InvertedFlagName(p.FullName)
		if p.FlagSet.Changed(inverted) {
			x, err = p.FlagSet.GetBool(p.FlagName)
			if err != nil {
				return fmt.Errorf("failed to get value for --%s: %w", inverted, err)
			}

			x = !x
		}
	}

	p.Value.SetBool(x)

	return nil
}

// setColorMode sets a color mode value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *ValueParser) setColorMode() error {
	// TODO: Unsafe conversion.
	x := iostreams.ColorMode(p.Value.Int())

	if !p.canUnmarshal() {
		panic(fmt.Sprintf("casting type of field %q to TextUnmarshaler", p.Field.Name))
	}

	if p.EnvValue != "" {
		v, err := p.unmarshal(p.EnvValue)
		if err != nil {
			return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
		}

		// TODO: Unsafe conversion.
		x = iostreams.ColorMode(v.Int())
	}

	if p.FlagSet.Changed(p.FlagName) {
		f := p.FlagSet.Lookup(p.FlagName)
		if f == nil {
			panic("failed to get value for --" + p.FlagName)
		}

		v, err := p.unmarshal(f.Value.String())
		if err != nil {
			return fmt.Errorf("failed to unmarshal color mode %q: %w", f.Value.String(), err)
		}

		// TODO: Unsafe conversion.
		x = iostreams.ColorMode(v.Int())
	}

	p.Value.SetInt(int64(x))

	return nil
}

// setInt sets an integer value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *ValueParser) setInt() error {
	var err error

	x := p.Value.Int()

	if p.EnvValue != "" {
		if p.canUnmarshal() {
			v, err := p.unmarshal(p.EnvValue)
			if err != nil {
				return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
			}

			x = v.Int()
		} else {
			x, err = strconv.ParseInt(p.EnvValue, 10, 0)
		}
	}

	if err != nil {
		return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
	}

	if p.FlagSet.Changed(p.FlagName) {
		var n int

		n, err = p.FlagSet.GetInt(p.FlagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.FlagName, err)
		}

		x = int64(n)
	}

	p.Value.SetInt(x)

	return nil
}

// setPath sets a string value from the environment variable or
// the command-line flag to the currently parsed value as an [fspath.Path]. It
// also cleans the path and possibly makes it absolute.
func (p *ValueParser) setPath() error {
	var err error

	x := fspath.Path(p.Value.String())

	if p.EnvValue != "" {
		x = fspath.Path(p.EnvValue)
	}

	if p.FlagSet.Changed(p.FlagName) {
		x, err = p.FlagSet.GetPath(p.FlagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.FlagName, err)
		}
	}

	x, err = x.Abs()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	p.Value.SetString(string(x))

	return nil
}

// setString sets a string value from the environment variable or
// the command-line flag to the currently parsed value.
func (p *ValueParser) setString() error {
	x := p.Value.String()

	if p.EnvValue != "" {
		x = p.EnvValue
	}

	if p.FlagSet.Changed(p.FlagName) {
		var err error

		x, err = p.FlagSet.GetString(p.FlagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.FlagName, err)
		}
	}

	p.Value.SetString(x)

	return nil
}

// canUnmarshal reports whether the field can be cast to
// [encoding.TextUnmarshaler] and unmarshaled using it.
func (p *ValueParser) canUnmarshal() bool {
	return reflect.PointerTo(p.Value.Type()).Implements(textUnmarshalerType)
}

// unmarshal converts the string s to the type of the value that is currently
// being parsed by calling the type's UnmarshalText function. It returns
// the actual value instead of a pointer to the value.
func (p *ValueParser) unmarshal(s string) (reflect.Value, error) {
	ptr := reflect.New(p.Value.Type())

	unmarshaler, ok := ptr.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return reflect.Value{}, fmt.Errorf(
			"%w: type of %q to TextUnmarshaler",
			errInvalidCast,
			p.Field.Name,
		)
	}

	if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to unmarshal %q: %w", s, err)
	}

	return ptr.Elem(), nil
}
