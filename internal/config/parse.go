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
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
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
	errDefaultConfig      = errors.New("using default config")
	errInvalidCast        = errors.New("cannot convert type")
	errNilFlag            = errors.New("no flag found")
)

// textUnmarshalerType is a helper variable for checking if types of fields in
// Config implement [encoding.TextUnmarshaler].
//
//nolint:gochecknoglobals // used like constant
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// dynamicFields are the fields that are not statically defined in the Config
// but depend on the plugins. They will not be set by the regular applying of
// the config values but separately after the plugins manifests have been
// loaded.
//
//nolint:gochecknoglobals // used like constant
var dynamicFields = []string{"Defaults", "Directory", "Plugins", "Tasks"}

// ApplyOptions is the type for the options for the Apply function.
type ApplyOptions struct {
	Dir     fspath.Path    // base directory for the program operations
	FlagSet *flags.FlagSet // flag set for the apply operation

	// idents is the list of the config identifiers that form the "path" to
	// the config value that is currently being parsed. It must always start
	// with the global prefix for the environment variables.
	idents []string
}

// Apply applies the values of the config values from environment variables and
// command-line flags to cfg. It modifies the pointed cfg.
func Apply(ctx context.Context, cfg *Config, opts ApplyOptions) error {
	if len(opts.idents) == 0 {
		opts.idents = []string{EnvPrefix}
	}

	if opts.idents[0] != EnvPrefix {
		panic(
			fmt.Sprintf(
				"Apply must be called with no config identifiers or with the global prefix for the environment variables as the first identifier: %q", //nolint:lll
				EnvPrefix,
			),
		)
	}

	err := applyStruct(ctx, reflect.ValueOf(cfg).Elem(), opts)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// ApplyPlugins applies the config values for plugins from environment variables
// and command-line flags to cfg. It modifies the pointed cfg.
func ApplyPlugins(ctx context.Context) {
	logging.Debug(ctx, "applying plugins")
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
	cfg := DefaultConfig()
	dir := cfg.Directory

	if err := parseFile(ctx, flagSet, cfg); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logging.Debug(ctx, "read config file", "cfg", cfg)

	opts := ApplyOptions{
		idents:  nil,
		Dir:     dir, // this is the working dir by default so no extra work is needed
		FlagSet: flagSet,
	}
	if err := Apply(ctx, cfg, opts); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logging.Info(ctx, "parsed config", "cfg", cfg)

	return cfg, nil
}

// Validate checks if all of the config values that were left after unmarshaling
// the config are valid plugin or plugin command names.
//
// TODO: This should have a better implementation.
func Validate(cfg *Config, plugins *plugin.Store) error {
	for k := range cfg.Plugins {
		ok := false
	PluginLoop:
		for _, p := range plugins.Plugins {
			manifest := p.Manifest()

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

func applyBool(value reflect.Value, opts ApplyOptions) error {
	var err error

	x := value.Bool()
	env := envValue(opts.idents)

	if env != "" {
		x, err = strconv.ParseBool(env)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetBool(flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	if HasInvertedFlagName(key) {
		inverted := InvertedFlagName(key)
		if opts.FlagSet.Changed(inverted) {
			x, err = opts.FlagSet.GetBool(inverted)
			if err != nil {
				return fmt.Errorf("failed to get value for --%s: %w", inverted, err)
			}

			x = !x
		}
	}

	value.SetBool(x)

	return nil
}

func applyColorMode(value reflect.Value, opts ApplyOptions) error {
	if !canUnmarshal(value) {
		panic(fmt.Sprintf("failed to cast value to encoding.TextUnmarshaler: %[1]v (%[1]T)", value))
	}

	var err error

	// TODO: Unsafe conversion.
	x := terminal.ColorMode(value.Int())
	env := envValue(opts.idents)

	if env != "" {
		var v reflect.Value

		v, err = unmarshal(value, env)
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		// TODO: Unsafe conversion.
		x = terminal.ColorMode(v.Int())
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		f := opts.FlagSet.Lookup(flagName)
		if f == nil {
			return fmt.Errorf("%w: %s", errNilFlag, flagName)
		}

		var v reflect.Value

		v, err = unmarshal(value, f.Value.String())
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		// TODO: Unsafe conversion.
		x = terminal.ColorMode(v.Int())
	}

	value.SetInt(int64(x))

	return nil
}

func applyInt(value reflect.Value, opts ApplyOptions) error {
	var err error

	x := value.Int()
	env := envValue(opts.idents)

	if env != "" {
		x, err = parseInt(env, value)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		var i int

		// TODO: Allow the use of unmarshalling here, probably using
		// f.Value.String().
		i, err = opts.FlagSet.GetInt(flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}

		x = int64(i)
	}

	value.SetInt(x)

	return nil
}

func applyPath(value reflect.Value, opts ApplyOptions) error {
	var err error

	x := fspath.Path(value.String())
	env := envValue(opts.idents)

	if env != "" {
		x = fspath.Path(env)
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetPath(flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	if !x.IsAbs() {
		path, err := fspath.NewAbs(string(opts.Dir), string(x))
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		x = path.Clean()
	}

	value.Set(reflect.ValueOf(x))

	return nil
}

func applyPathSlice(value reflect.Value, opts ApplyOptions) error {
	var err error

	i := value.Interface()

	x, ok := i.([]fspath.Path)
	if !ok {
		panic(fmt.Sprintf("failed to convert value to slice of paths: %[1]v (%[1]T)", i))
	}

	env := envValue(opts.idents)

	// TODO: There might be a more robust way to parse the paths, but this is
	// fine for now.
	if env != "" {
		parts := strings.Split(env, ",")
		x = make([]fspath.Path, 0, len(parts))

		for _, part := range parts {
			x = append(x, fspath.Path(part))
		}
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetPathSlice(flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	for i, p := range x {
		if !p.IsAbs() {
			path, err := fspath.NewAbs(string(opts.Dir), string(p))
			if err != nil {
				return fmt.Errorf("%w", err)
			}

			x[i] = path.Clean()
		}
	}

	value.Set(reflect.ValueOf(x))

	return nil
}

func applyString(value reflect.Value, opts ApplyOptions) error {
	x := value.String()
	env := envValue(opts.idents)

	if env != "" {
		x = env
	}

	key := configKey(opts.idents)
	flagName := FlagName(key)

	if opts.FlagSet.Changed(flagName) {
		var err error

		x, err = opts.FlagSet.GetString(flagName)
		if err != nil {
			return fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	value.SetString(x)

	return nil
}

func applyStruct(ctx context.Context, cfg reflect.Value, opts ApplyOptions) error {
	var err error

	if len(opts.idents) == 1 {
		opts, err = setDir(ctx, cfg, opts)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	for i := range cfg.NumField() {
		field := cfg.Type().Field(i)
		val := cfg.Field(i)

		logging.Trace(
			ctx,
			"checking config field",
			"key",
			field.Name,
			"value",
			val,
			"opts",
			opts,
		)

		if !val.CanSet() {
			logging.Trace(ctx, "skipping config field", "key", field.Name, "value", val)

			continue
		}

		if slices.Contains(dynamicFields, field.Name) {
			logging.Trace(ctx, "skipping config field", "key", field.Name, "value", val)

			continue
		}

		newOpts := ApplyOptions{
			idents:  append(opts.idents, field.Name),
			Dir:     opts.Dir,
			FlagSet: opts.FlagSet,
		}

		switch val.Kind() { //nolint:exhaustive // TODO: implemented as needed
		case reflect.Bool:
			err = applyBool(val, newOpts)
		case reflect.Int:
			if val.Type().Name() == "ColorMode" {
				err = applyColorMode(val, newOpts)
			} else {
				err = applyInt(val, newOpts)
			}
		case reflect.Slice:
			e := val.Type().Elem()
			if e.Kind() != reflect.String || e.Name() != "Path" {
				panic(
					fmt.Sprintf("unsupported config field type for %s: %s", field.Name, val.Kind()),
				)
			}

			err = applyPathSlice(val, newOpts)
		case reflect.String:
			if val.Type().Name() == "Path" {
				err = applyPath(val, newOpts)
			} else {
				err = applyString(val, newOpts)
			}
		case reflect.Struct:
			err = applyStruct(ctx, val, newOpts)
		default:
			panic(fmt.Sprintf("unsupported config field type for %s: %s", field.Name, val.Kind()))
		}

		if err != nil {
			return fmt.Errorf("%w", err)
		}

		logging.Trace(ctx, "set config field", "key", field.Name, "value", val)
	}

	return nil
}

// canUnmarshal reports whether value can be cast to [encoding.TextUnmarshaler]
// and unmarshaled using it.
func canUnmarshal(value reflect.Value) bool {
	return reflect.PointerTo(value.Type()).Implements(textUnmarshalerType)
}

// configKey returns the key for the given config identifiers.
func configKey(idents []string) string {
	return strings.Join(idents[1:], ".")
}

// envValue returns the value of the environment variable for the given config
// identifiers.
func envValue(idents []string) string {
	return os.Getenv(strings.ToUpper(strings.Join(idents, "_")))
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
			if i > 0 && unicode.IsUpper(r) {
				key += "-"
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

// parseFile finds and parses the config file and sets the values to cfg. It
// modifies the pointed cfg in place.
func parseFile(ctx context.Context, flagSet *flags.FlagSet, cfg *Config) error {
	configFile, err := resolveFile(flagSet)
	if err != nil && !errors.Is(err, errDefaultConfig) {
		return fmt.Errorf("%w", err)
	}

	cfg.sourceFile = configFile

	if configFile == "" {
		return nil
	}

	logging.Trace(ctx, "reading config file", "path", configFile)

	data, err := configFile.Clean().ReadFile()
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	rawCfg := make(map[string]any)

	if err = toml.Unmarshal(data, &rawCfg); err != nil {
		return fmt.Errorf("failed to decode the config file: %w", err)
	}

	logging.Trace(ctx, "unmarshaled config file", "cfg", rawCfg)
	normalizeKeys(rawCfg)
	logging.Trace(ctx, "normalized keys", "cfg", rawCfg)

	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct // use default values
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     cfg,
	}

	logging.Trace(ctx, "created default config", "cfg", cfg)

	d, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := d.Decode(rawCfg); err != nil {
		return fmt.Errorf("failed to read environment variables for config: %w", err)
	}

	return nil
}

// parseInt parses the given string value into an int64. If the value can be
// resolved using a TextUnmarshaler, the function uses the given value's type to
// unmarshal the value.
func parseInt(s string, value reflect.Value) (int64, error) {
	if canUnmarshal(value) {
		v, err := unmarshal(value, s)
		if err != nil {
			return 0, fmt.Errorf("%w", err)
		}

		return v.Int(), nil
	}

	x, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w", err)
	}

	return x, nil
}

func setDir(ctx context.Context, cfg reflect.Value, opts ApplyOptions) (ApplyOptions, error) {
	i := -1

	for j := range cfg.NumField() {
		if cfg.Type().Field(j).Name == "Directory" {
			i = j

			break
		}
	}

	if i < 0 {
		panic(fmt.Sprintf("failed to find Directory field in %q", cfg.Type().Name()))
	}

	field := cfg.Type().Field(i)
	val := cfg.Field(i)

	logging.Trace(
		ctx,
		"checking config field",
		"key",
		field.Name,
		"value",
		val,
		"opts",
		opts,
	)

	if !val.CanSet() {
		panic(fmt.Sprintf("cannot set Directory field in %q", cfg.Type().Name()))
	}

	var err error

	newOpts := ApplyOptions{
		idents:  append(opts.idents, field.Name),
		Dir:     opts.Dir,
		FlagSet: opts.FlagSet,
	}

	if err = applyPath(val, newOpts); err != nil {
		return ApplyOptions{}, fmt.Errorf("%w", err)
	}

	opts.Dir = fspath.Path(val.String())

	logging.Trace(ctx, "set config field", "key", field.Name, "value", val)

	return opts, nil
}

// unmarshal converts s to the type of value by calling value's type's
// UnmarshalText function. It returns the actual value instead of a pointer to
// the value.
func unmarshal(value reflect.Value, s string) (reflect.Value, error) {
	ptr := reflect.New(value.Type())

	unmarshaler, ok := ptr.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return reflect.Value{}, fmt.Errorf(
			"%w: type of %q to TextUnmarshaler",
			errInvalidCast,
			value,
		)
	}

	if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to unmarshal %q: %w", s, err)
	}

	return ptr.Elem(), nil
}
