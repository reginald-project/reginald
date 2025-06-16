// Copyright 2025 Antti Kivi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
)

// Errors returned from the configuration parser.
var (
	ErrInvalidConfig      = errors.New("invalid config")
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

	// Cfg is the Config that is currently being parsed.
	Cfg *Config

	// FlagSet is the flag set used for checking the values.
	FlagSet *flags.FlagSet

	// Plugin is the plugin currently being parsed if the parser has moved to
	// parsing plugin configs. Otherwise, it should be nil.
	Plugin plugin.Plugin

	// Plugins is the plugin store for the current plugins.
	Plugins *plugin.Store

	// Value is the Value of the currently parsed field.
	Value reflect.Value

	// Field is the currently parsed struct Field.
	Field reflect.StructField
}

// A pluginParser is a helper type that holds the current values for the config
// value from a plugin that is currently being parsed.
type pluginParser struct {
	// envName is the name of the environment variable for checking the value
	// for the current field.
	envName string

	// envValue is the value of the environment variable for the current field.
	envValue string

	// flagName is the name of the command-line flag for checking the value for
	// the current field.
	flagName string

	// FlagSet is the flag set used for checking the values.
	flagSet *flags.FlagSet

	// m is the config map that is currently being modified.
	m map[string]any

	// c is the [api.ConfigEntry] that is currently being checked.
	c api.ConfigEntry
}

// LogValue implements [slog.LogValuer] for [valueParser]. It returns a group
// containing the fields of the parser that are relevant for logging.
func (p *ValueParser) LogValue() slog.Value {
	var attrs []slog.Attr

	attrs = append(attrs, slog.Any("cfg", p.Cfg))

	if p.FlagSet == nil {
		attrs = append(attrs, slog.String("flagSet", "nil"))
	} else {
		attrs = append(attrs, slog.String("flagSet", "set"))
	}

	if p.Plugins == nil {
		attrs = append(attrs, slog.String("Plugins", "<nil>"))
	} else {
		pluginNames := make([]string, 0, len(p.Plugins.Plugins))

		for _, plugin := range p.Plugins.Plugins {
			pluginNames = append(pluginNames, plugin.Manifest().Name)
		}

		attrs = append(attrs, slog.Any("Plugins", pluginNames))
	}
	attrs = append(
		attrs,
		slog.Group(
			"Value",
			slog.String("type", p.Value.Type().Name()),
			slog.Any("value", p.Value.Interface()),
		),
		slog.Group(
			"field",
			slog.String("name", p.Field.Name),
			slog.String("type", p.Field.Type.Name()),
		),
	)

	if p.Plugin == nil {
		attrs = append(attrs, slog.String("Plugin", "<nil>"))
	} else {
		attrs = append(attrs, slog.String("Plugin", p.Plugin.Manifest().Name))
	}

	attrs = append(
		attrs,
		slog.String("FullName", p.FullName),
		slog.String("EnvName", p.EnvName),
		slog.String("EnvValue", p.EnvValue),
		slog.String("FlagName", p.FlagName),
	)

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
func Parse(ctx context.Context, flagSet *flags.FlagSet) (*Config, error) {
	configFile, err := resolveFile(flagSet)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config file: %w", err)
	}

	logging.TraceContext(ctx, "reading config file", "path", configFile)

	data, err := configFile.Clean().ReadFile()
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
	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct // use default values
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

	logging.DebugContext(ctx, "read raw config", "cfg", cfg)

	parser := &ValueParser{
		Cfg:      cfg,
		FlagSet:  flagSet,
		Plugins:  nil,
		Value:    reflect.ValueOf(cfg).Elem(),
		Field:    reflect.StructField{}, //nolint:exhaustruct // zero value wanted
		Plugin:   nil,
		FullName: "",
		EnvName:  EnvPrefix,
		EnvValue: "",
		FlagName: "",
	}
	if err := parser.ApplyOverrides(ctx); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logging.InfoContext(ctx, "parsed config", "cfg", cfg)

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

// Validate checks if all of the config values that were left after unmarshaling
// the config are valid plugin or plugin command names.
func Validate(cfg *Config, plugins *plugin.Store) error {
	for k := range cfg.Plugins {
		ok := false
	PluginLoop:
		for _, plugin := range plugins.Plugins {
			manifest := plugin.Manifest()

			if manifest.Domain == k && manifest.Config != nil {
				ok = true

				break PluginLoop
			}

			for _, c := range manifest.Commands {
				if c.Name == k && c.Config != nil {
					ok = true

					break PluginLoop
				}
			}
		}

		if !ok {
			return fmt.Errorf("%w: invalid config key %q", ErrInvalidConfig, k)
		}
	}

	return nil
}

// ApplyOverrides applies the overrides of the config values from environment
// variables and command-line flags to cfg. It modifies the pointed cfg.
func (p *ValueParser) ApplyOverrides(ctx context.Context) error {
	if err := p.applyStructOverrides(ctx); err != nil {
		return fmt.Errorf("%w", err)
	}

	if p.Plugins == nil || p.Plugins.Len() == 0 {
		return nil
	}

	for _, plugin := range p.Plugins.Plugins {
		p.Plugin = plugin
		manifest := p.Plugin.Manifest()
		pluginMap := make(map[string]any)
		rawVal := p.Cfg.Plugins[manifest.Domain]

		m, ok := rawVal.(map[string]any)
		if ok {
			pluginMap = m
		} else if rawVal != nil {
			return fmt.Errorf("%w: config for plugin %q is not a map", ErrInvalidConfig, manifest.Name)
		}

		// Use the name of the plugin as the prefix for the environment
		// variable.
		p.EnvName = manifest.Name

		if err := p.applyPluginOverrides(ctx, pluginMap, manifest.Config); err != nil {
			return fmt.Errorf("failed to apply configs for plugins: %w", err)
		}

		p.Cfg.Plugins[manifest.Domain] = pluginMap

		for _, cmd := range manifest.Commands {
			// Plugin configs take precedence.
			if cmd.Name == manifest.Domain && len(manifest.Config) > 0 {
				continue
			}

			cmdMap := make(map[string]any)
			// All of the tables for the plugins and commands should be in
			// the root of the config.
			rawVal := p.Cfg.Plugins[cmd.Name]

			m, ok := rawVal.(map[string]any)
			if ok {
				cmdMap = m
			} else if rawVal != nil {
				return fmt.Errorf("%w: config for plugin command %q is not a map", ErrInvalidConfig, cmd.Name)
			}

			// Use the name of the command as the prefix for the environment
			// variable.
			p.EnvName = cmd.Name

			if err := p.applyPluginOverrides(ctx, cmdMap, cmd.Config); err != nil {
				return fmt.Errorf("failed to apply configs for plugin commands: %w", err)
			}

			p.Cfg.Plugins[cmd.Name] = cmdMap
		}
	}

	return nil
}

func (p *ValueParser) applyStructOverrides(ctx context.Context) error {
	for i := range p.Value.NumField() {
		var err error

		// TODO: Check the struct tags for env.
		parser := &ValueParser{
			Cfg:      p.Cfg,
			FlagSet:  p.FlagSet,
			Plugins:  p.Plugins,
			Value:    p.Value.Field(i),
			Field:    p.Value.Type().Field(i),
			Plugin:   nil,
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

		if parser.Field.Name == "Defaults" || parser.Field.Name == "Plugins" ||
			parser.Field.Name == "Tasks" {
			continue
		}

		if parser.Value.Kind() == reflect.Struct {
			// Apply the overrides recursively but set the plugins to nil as
			// only the top-level config has the map for config values.
			err = parser.applyStructOverrides(ctx)
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
		case reflect.Slice:
			elem := parser.Value.Type().Elem()
			if elem.Kind() == reflect.String && elem.Name() == "Path" {
				err = parser.setPathSlice()
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

	return nil
}

// applyPluginOverrides applies the overrides of the config values from
// environment variables and command-line flags to plugin configs in cfg in p.
// It modifies the pointed cfg.
func (p *ValueParser) applyPluginOverrides(ctx context.Context, cfgMap map[string]any, configs []api.ConfigEntry) error {
	logging.TraceContext(ctx, "applying plugin overrides", "cfgs", configs)

	for _, cfgVal := range configs {
		parser := &pluginParser{
			flagSet:  p.FlagSet,
			m:        cfgMap,
			c:        cfgVal,
			envName:  "",
			envValue: "",
			flagName: "",
		}

		if cfgVal.EnvOverride == "" {
			prefix := toEnv(p.EnvName, EnvPrefix)
			parser.envName = toEnv(cfgVal.Key, prefix)
		} else {
			parser.envName = EnvPrefix + "_" + cfgVal.EnvOverride
		}

		parser.envValue = os.Getenv(parser.envName)

		f := cfgVal.Flag

		parser.flagName = f.Name

		logging.TraceContext(ctx, "checking plugin config", "parser", parser)

		switch cfgVal.Type {
		case api.BoolValue:
			x, err := parser.bool()
			if err != nil {
				return fmt.Errorf(
					"failed to set value for %q by %q: %w",
					cfgVal.Key,
					p.Plugin.Manifest().Name,
					err,
				)
			}

			cfgMap[cfgVal.Key] = x
		case api.IntValue:
			x, err := parser.int()
			if err != nil {
				return fmt.Errorf(
					"failed to set value for %q by %q: %w",
					cfgVal.Key,
					p.Plugin.Manifest().Name,
					err,
				)
			}

			cfgMap[cfgVal.Key] = x
		case api.StringValue:
			x, err := parser.string()
			if err != nil {
				return fmt.Errorf(
					"failed to set value for %q by %q: %w",
					cfgVal.Key,
					p.Plugin.Manifest().Name,
					err,
				)
			}

			cfgMap[cfgVal.Key] = x
		default:
			return fmt.Errorf(
				"%w: ConfigEntry %q in plugin %q has invalid type: %s",
				ErrInvalidConfig,
				cfgVal.Key,
				p.Plugin.Manifest().Name,
				cfgVal.Type,
			)
		}
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
			var v reflect.Value

			v, err = p.unmarshal(p.EnvValue)
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
	x := terminal.ColorMode(p.Value.Int())

	if !p.canUnmarshal() {
		panic(fmt.Sprintf("casting type of field %q to TextUnmarshaler", p.Field.Name))
	}

	if p.EnvValue != "" {
		v, err := p.unmarshal(p.EnvValue)
		if err != nil {
			return fmt.Errorf("%s=%q: %w", p.EnvName, p.EnvValue, err)
		}

		// TODO: Unsafe conversion.
		x = terminal.ColorMode(v.Int())
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
		x = terminal.ColorMode(v.Int())
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
			var v reflect.Value

			v, err = p.unmarshal(p.EnvValue)
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

// setPathSlice sets a string slice value from the environment variable or
// the command-line flag to the currently parsed value as a slice of
// [fspath.Path]. It also cleans the paths and possibly makes them absolute.
func (p *ValueParser) setPathSlice() error {
	var (
		err error
		ok  bool
		x   []fspath.Path
	)

	x, ok = p.Value.Interface().([]fspath.Path)
	if !ok {
		return fmt.Errorf("%w: given value for %q is not a slice of paths: %[3]v (%[3]T)", errInvalidCast, p.Field.Name, p.Value)
	}

	// TODO: There might be a more robust way to parse the paths, but this is
	// fine for now.
	if p.EnvValue != "" {
		s := p.EnvValue
		parts := strings.Split(s, ",")
		x = make([]fspath.Path, 0, len(parts))

		for _, part := range parts {
			x = append(x, fspath.Path(part))
		}
	}

	if p.FlagSet.Changed(p.FlagName) {
		x, err = p.FlagSet.GetPathSlice(p.FlagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", p.FlagName, err)
		}
	}

	for i, p := range x {
		x[i], err = p.Abs()
		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	p.Value.Set(reflect.ValueOf(x))

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

// bool resolves the value of a bool config value in plugin configurations.
func (p *pluginParser) bool() (bool, error) {
	var (
		err error
		x   bool
	)

	val, ok := p.m[p.c.Key]
	if !ok {
		// At start the config entry should have the default value.
		x, ok = p.c.Value.(bool)
		if !ok {
			return x, fmt.Errorf(
				"%w: default value for %q is not a bool: %[3]v (%[3]T)",
				errInvalidCast,
				p.c.Key,
				p.c.Value,
			)
		}
	} else {
		x, ok = val.(bool)
		if !ok {
			return x, fmt.Errorf("%w: given value for %q is not a bool: %[3]v (%[3]T)", errInvalidCast, p.c.Key, p.c.Value)
		}
	}

	if p.envValue != "" {
		x, err = strconv.ParseBool(p.envValue)
		if err != nil {
			return x, fmt.Errorf("%s=%q: %w", p.envName, p.envValue, err)
		}
	}

	// TODO: Inverse flags.
	if p.flagName != "" && p.flagSet.Changed(p.flagName) {
		x, err = p.flagSet.GetBool(p.flagName)
		if err != nil {
			return x, fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}
	}

	return x, nil
}

// int resolves the value of an int config value in plugin configurations.
func (p *pluginParser) int() (int64, error) {
	var (
		err error
		x   int64
	)

	val, ok := p.m[p.c.Key]
	if !ok {
		var v int

		v, err = p.c.Int()
		if err != nil {
			return x, fmt.Errorf("failed to get int value for %q: %w", p.c.Key, err)
		}

		// At start the config entry should have the default value.
		x = int64(v)
	} else {
		x, ok = val.(int64)
		if !ok {
			return x, fmt.Errorf("%w: given value for %q is not an int: %[3]v (%[3]T)", errInvalidCast, p.c.Key, p.c.Value)
		}
	}

	if p.envValue != "" {
		x, err = strconv.ParseInt(p.envValue, 10, 0)
		if err != nil {
			return x, fmt.Errorf("%s=%q: %w", p.envName, p.envValue, err)
		}
	}

	if p.flagName != "" && p.flagSet.Changed(p.flagName) {
		var n int

		n, err = p.flagSet.GetInt(p.flagName)
		if err != nil {
			return x, fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}

		x = int64(n)
	}

	return x, nil
}

// string resolves the value of a string config value in plugin configurations.
func (p *pluginParser) string() (string, error) {
	var (
		err error
		x   string
	)

	val, ok := p.m[p.c.Key]
	if !ok {
		// At start the config entry should have the default value.
		x, ok = p.c.Value.(string)
		if !ok {
			return x, fmt.Errorf(
				"%w: default value for %q is not an int: %[3]v (%[3]T)",
				errInvalidCast,
				p.c.Key,
				p.c.Value,
			)
		}
	} else {
		x, ok = val.(string)
		if !ok {
			return x, fmt.Errorf("%w: given value for %q is not an int: %[3]v (%[3]T)", errInvalidCast, p.c.Key, p.c.Value)
		}
	}

	if p.envValue != "" {
		x = p.envValue
	}

	if p.flagName != "" && p.flagSet.Changed(p.flagName) {
		x, err = p.flagSet.GetString(p.flagName)
		if err != nil {
			return x, fmt.Errorf("failed to get value for --%s: %w", p.flagName, err)
		}
	}

	return x, nil
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
