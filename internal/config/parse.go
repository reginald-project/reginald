// Copyright 2025 The Reginald Authors
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
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/flags"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/typeconv"
)

// Errors returned from the configuration parser.
var (
	ErrInvalidConfig = errors.New("invalid config")
	errNilFlag       = errors.New("no flag found")
	errNilPlugins    = errors.New("no plugins found")
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
var dynamicFields = []string{"Defaults", "Directory", "RawPlugins", "RawTasks", "Plugins", "Tasks"}

// ApplyOptions is the type for the options for the Apply function.
type ApplyOptions struct {
	Dir     fspath.Path    // base directory for the program operations
	FlagSet *flags.FlagSet // flag set for the apply operation

	// Store contains the discovered plugin. It should not be set when applying
	// the built-in config values
	Store *plugin.Store

	// idents is the list of the config identifiers that form the "path" to
	// the config value that is currently being parsed. It must always start
	// with the global prefix for the environment variables.
	idents []string
}

// Apply applies the values of the config values from environment variables and
// command-line flags to cfg. It modifies the pointed cfg.
func Apply(ctx context.Context, cfg *Config, opts ApplyOptions) error {
	return applyStruct(ctx, reflect.ValueOf(cfg).Elem(), initIdents(opts))
}

// ApplyPlugins applies the config values for plugins from environment variables
// and command-line flags to cfg. It modifies the pointed cfg.
func ApplyPlugins(ctx context.Context, cfg *Config, opts ApplyOptions) error {
	opts = initIdents(opts)

	log.Debug(ctx, "applying plugins")

	if opts.Store == nil {
		panic("nil plugin store")
	}

	plugins := opts.Store.Plugins
	if len(plugins) == 0 {
		log.Trace(ctx, "no plugins found")

		return nil
	}

	rawPlugins := cfg.RawPlugins
	cfgs := make(api.KeyValues, 0, len(opts.Store.Commands))

	// At this point, all of the plugins have been converted to commands.
	for _, cmd := range opts.Store.Commands {
		manifest := cmd.Plugin.Manifest()
		name := manifest.Name
		domain := manifest.Domain

		if !cmd.Plugin.External() {
			domain = cmd.Name
		}

		a, ok := rawPlugins[domain]
		if !ok {
			log.Trace(ctx, "no map for plugin found", "name", name, "domain", domain)

			a = make(map[string]any)
		}

		rawMap, ok := a.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"%w: config for plugin %q with config key %q is not a map",
				ErrInvalidConfig,
				name,
				domain,
			)
		}

		log.Trace(ctx, "initial config map resolved", "name", name, "domain", domain, "map", rawMap)

		newOpts := ApplyOptions{
			Dir:     opts.Dir,
			FlagSet: opts.FlagSet,
			Store:   opts.Store,
			idents:  append(opts.idents, domain),
		}

		values, err := applyPluginMap(ctx, rawMap, manifest.Config, cmd.Commands, newOpts)
		if err != nil {
			return err
		}

		log.Trace(ctx, "values applied to map", "domain", domain, "map", rawMap)

		cfgs = append(cfgs, api.KeyVal{
			Value: api.Value{Val: values, Type: api.ConfigSliceValue},
			Key:   domain,
		})
	}

	cfg.Plugins = cfgs

	return nil
}

// NormalizeKeys checks the config value keys in the given raw config map and
// changes them into the wanted format ("kebab-case") in case the config
// contains something in "camel-case". This way the config file is able to
// support JSON and YAML while allowing those files to have the keys more
// idiomatic for those formats.
func NormalizeKeys(cfg map[string]any) {
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
			NormalizeKeys(m)
		}
	}
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

	var fileErr *FileError

	if err := parseFile(ctx, dir, flagSet, cfg); err != nil {
		if !errors.As(err, &fileErr) {
			return nil, err
		}
	}

	log.Debug(ctx, "read config file", "cfg", cfg)

	opts := ApplyOptions{
		idents:  nil,
		Dir:     dir, // this is the working dir by default so no extra work is needed
		FlagSet: flagSet,
		Store:   nil,
	}
	if err := Apply(ctx, cfg, opts); err != nil {
		return nil, err
	}

	if fileErr != nil {
		return cfg, fileErr
	}

	return cfg, nil
}

// Validate checks if all of the config values that were left after unmarshaling
// the config are valid plugin or plugin command names.
//
// TODO: This should have a better implementation.
func Validate(cfg *Config, store *plugin.Store) error {
	if cfg.Quiet && cfg.Verbose {
		return fmt.Errorf("%w: cannot be both quiet and verbose", ErrInvalidConfig)
	}

	if cfg.Interactive && cfg.Strict {
		return fmt.Errorf("%w: cannot be both interactive and strict", ErrInvalidConfig)
	}

	for k := range cfg.RawPlugins {
		ok := false
	PluginLoop:
		for _, p := range store.Plugins {
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

// applyBool sets a boolean value from the environment variables and
// command-line flags to the config struct.
func applyBool(value reflect.Value, opts ApplyOptions) error {
	x, err := boolValue(value.Bool(), opts, nil)
	if err != nil {
		return err
	}

	value.SetBool(x)

	return nil
}

// applyColorMode sets a color mode value from the environment variables and
// command-line flags to the config struct.
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
			return err
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
			return err
		}

		// TODO: Unsafe conversion.
		x = terminal.ColorMode(v.Int())
	}

	value.SetInt(int64(x))

	return nil
}

// applyInt sets an integer value from the environment variables and
// command-line flags to the config struct.
func applyInt(value reflect.Value, opts ApplyOptions) error {
	var err error

	x := value.Int()
	env := envValue(opts.idents)

	if env != "" {
		x, err = parseInt(env, value)
		if err != nil {
			return err
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

// applyPath sets a filesystem path value from the environment variables and
// command-line flags to the config struct.
func applyPath(value reflect.Value, opts ApplyOptions) error {
	x, err := pathValue(fspath.Path(value.String()), opts, nil)
	if err != nil {
		return err
	}

	value.Set(reflect.ValueOf(x))

	return nil
}

// applyPathSlice sets a slice of filesystem paths from the environment
// variables and command-line flags to the config struct.
func applyPathSlice(value reflect.Value, opts ApplyOptions) error {
	i := value.Interface()

	x, ok := i.([]fspath.Path)
	if !ok {
		panic(fmt.Sprintf("failed to convert value to slice of paths: %[1]v (%[1]T)", i))
	}

	var err error

	x, err = pathSliceValue(x, opts, nil)
	if err != nil {
		return err
	}

	value.Set(reflect.ValueOf(x))

	return nil
}

// applyPluginCommands applies the config values for the given subcommands from
// the environment variables and the command-line flags to the plugin configs
// map.
func applyPluginCommands(
	ctx context.Context,
	rawMap map[string]any,
	cmds []*plugin.Command,
	opts ApplyOptions,
) (api.KeyValues, error) {
	result := make(api.KeyValues, len(cmds))
	parent := opts.idents[len(opts.idents)-1]

	log.Trace(ctx, "applying plugin commands", "parent", parent)

	for _, cmd := range cmds {
		name := cmd.Name

		a, ok := rawMap[name]
		if !ok {
			log.Trace(ctx, "no map for command found", "cmd", name)

			a = make(map[string]any)
		}

		raw, ok := a.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"%w: config for command %[2]q with config key %[2]q is not a map",
				ErrInvalidConfig,
				name,
			)
		}

		log.Trace(ctx, "initial config map resolved", "parent", parent, "cmd", name, "map", raw)

		newOpts := ApplyOptions{
			Dir:     opts.Dir,
			FlagSet: opts.FlagSet,
			Store:   opts.Store,
			idents:  append(opts.idents, name),
		}

		values, err := applyPluginMap(ctx, raw, cmd.Config, cmd.Commands, newOpts)
		if err != nil {
			return nil, err
		}

		kv := api.KeyVal{
			Value: api.Value{Val: values, Type: api.ConfigSliceValue},
			Key:   name,
		}

		result = append(result, kv)
	}

	return result, nil
}

// applyPluginMap applies the config values from the environment variables and
// the command-line flags to the given plugin configs map.
func applyPluginMap(
	ctx context.Context,
	rawMap map[string]any,
	entries []api.ConfigEntry,
	cmds []*plugin.Command,
	opts ApplyOptions,
) (api.KeyValues, error) {
	result := make(api.KeyValues, 0, len(entries)+len(cmds))

	log.Trace(ctx, "applying plugin map", "idents", opts.idents)

	parent := opts.idents[len(opts.idents)-1]

	values, err := applyPluginCommands(ctx, rawMap, cmds, opts)
	if err != nil {
		return nil, err
	}

	result = append(result, values...)

	for _, entry := range entries {
		raw, ok := rawMap[entry.Key]
		if ok && entry.FlagOnly {
			return nil, fmt.Errorf(
				"%w: unknown entry %q in config file for %q (config setting can only be set via a command-line flag)",
				ErrInvalidConfig,
				entry.Key,
				parent,
			)
		}

		if !ok {
			if entry.Type == api.IntValue {
				var err error

				// TODO: Is this conversion now done twice even for the default
				// value?
				raw, err = entry.Int()
				if err != nil {
					return nil, fmt.Errorf("failed to convert value for %q in %q to int: %w", entry.Key, parent, err)
				}
			} else {
				raw = entry.Val
			}
		}

		newOpts := ApplyOptions{
			Dir:     opts.Dir,
			FlagSet: opts.FlagSet,
			Store:   opts.Store,
			idents:  append(opts.idents, entry.Key),
		}

		kv, err := resolvePluginValue(raw, &entry, newOpts)
		if err != nil {
			return nil, err
		}

		result = append(result, kv)
	}

	log.Trace(ctx, "map applied", "key", parent, "cfg", rawMap)

	return result, nil
}

// applyString sets a string value from the environment variables and
// command-line flags to the config struct.
func applyString(value reflect.Value, opts ApplyOptions) error {
	x, err := stringValue(value.String(), opts, nil)
	if err != nil {
		return err
	}

	value.SetString(x)

	return nil
}

// applyStruct recursively sets the config values to cfg from the environment
// variables and command-line flags.
func applyStruct(ctx context.Context, cfg reflect.Value, opts ApplyOptions) error {
	var err error

	if len(opts.idents) == 1 {
		opts, err = setDir(ctx, cfg, opts)
		if err != nil {
			return err
		}
	}

	for i := range cfg.NumField() {
		field := cfg.Type().Field(i)
		val := cfg.Field(i)

		log.Trace(ctx, "checking config field", "key", field.Name, "value", val, "opts", opts)

		if !val.CanSet() {
			log.Trace(ctx, "skipping config field", "key", field.Name, "value", val)

			continue
		}

		if slices.Contains(dynamicFields, field.Name) {
			log.Trace(ctx, "skipping config field", "key", field.Name, "value", val)

			continue
		}

		newOpts := ApplyOptions{
			idents:  append(opts.idents, field.Name),
			Dir:     opts.Dir,
			FlagSet: opts.FlagSet,
			Store:   opts.Store,
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
			return err
		}

		log.Trace(ctx, "set config field", "key", field.Name, "value", val)
	}

	return nil
}

// boolSliceValue resolves a slice of bools from the environment variables and
// the command-line flags to be used in the config.
func boolSliceValue(x []bool, opts ApplyOptions, entry *api.ConfigEntry) ([]bool, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	// TODO: There might be a more robust way to parse the paths, but this is
	// fine for now.
	if env != "" && (entry == nil || !entry.FlagOnly) {
		parts := strings.Split(env, ",")
		x = make([]bool, len(parts))

		for i, part := range parts {
			x[i], err = strconv.ParseBool(part)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %q as a boolean: %w", parts, err)
			}
		}
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetBoolSlice(flagName)
		if err != nil {
			return nil, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	return x, nil
}

// boolValue resolves a boolean value from the environment variables and
// the command-line flags to be used in the config.
func boolValue(x bool, opts ApplyOptions, entry *api.ConfigEntry) (bool, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	if env != "" && (entry == nil || !entry.FlagOnly) {
		x, err = strconv.ParseBool(env)
		if err != nil {
			return false, fmt.Errorf("failed to parse %q as a boolean: %w", env, err)
		}
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetBool(flagName)
		if err != nil {
			return false, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	key := configKey(opts.idents)

	// TODO: Add plugin support for inverted flags and remove the plugin  check.
	if entry == nil && HasInvertedFlagName(key) {
		inverted := InvertedFlagName(key)
		if opts.FlagSet.Changed(inverted) {
			x, err = opts.FlagSet.GetBool(inverted)
			if err != nil {
				return false, fmt.Errorf("failed to get value for --%s: %w", inverted, err)
			}

			x = !x
		}
	}

	return x, nil
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
	key := ""

	for i, ident := range idents {
		if i > 0 {
			key += "_"
		}

		for j, c := range ident {
			if j > 0 && 'A' <= c && c <= 'Z' {
				key += "_"
			}

			key += string(c)
		}
	}

	return os.Getenv(strings.ToUpper(key))
}

// initIdents sets the correct initial identifiers to the ApplyOptions and
// checks that the initial identifiers are valid. It panics on errors.
func initIdents(opts ApplyOptions) ApplyOptions {
	if len(opts.idents) == 0 {
		opts.idents = []string{defaultPrefix}
	}

	if opts.idents[0] != defaultPrefix {
		panic(
			fmt.Sprintf(
				"Apply must be called with no config identifiers or with the global prefix for the environment variables as the first identifier: %q", //nolint:lll
				defaultPrefix,
			),
		)
	}

	return opts
}

// intSliceValue resolves a slice of ints from the environment variables and
// the command-line flags to be used in the config.
func intSliceValue(x []int, opts ApplyOptions, entry *api.ConfigEntry) ([]int, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	// TODO: There might be a more robust way to parse the paths, but this is
	// fine for now.
	if env != "" && (entry == nil || !entry.FlagOnly) {
		parts := strings.Split(env, ",")
		x = make([]int, len(parts))

		for i, part := range parts {
			var n int64

			n, err = strconv.ParseInt(part, 10, 0)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %q as an int: %w", parts, err)
			}

			// TODO: Unsafe?
			x[i] = int(n)
		}
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetIntSlice(flagName)
		if err != nil {
			return nil, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	return x, nil
}

// intValue resolves an integer value from the environment variables and
// the command-line flags to be used in the config.
func intValue(x int, opts ApplyOptions, entry *api.ConfigEntry) (int, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	if env != "" && (entry == nil || !entry.FlagOnly) {
		var i int64

		i, err = strconv.ParseInt(env, 10, 0)
		if err != nil {
			return 0, fmt.Errorf("failed to parse %q as an integer: %w", env, err)
		}

		// TODO: Unsafe?
		x = int(i)
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetInt(flagName)
		if err != nil {
			return 0, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	return x, nil
}

// parseFile finds and parses the config file and sets the values to cfg. It
// modifies the pointed cfg in place.
func parseFile(ctx context.Context, dir fspath.Path, flagSet *flags.FlagSet, cfg *Config) error {
	configFile, err := resolveFile(ctx, dir, flagSet)
	if err != nil {
		return err
	}

	cfg.sourceFile = configFile

	if configFile == "" {
		return nil
	}

	log.Trace(ctx, "reading config file", "path", configFile)

	data, err := os.ReadFile(string(configFile.Clean()))
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	rawCfg := make(map[string]any)

	if err = toml.Unmarshal(data, &rawCfg); err != nil {
		return fmt.Errorf("failed to decode the config file: %w", err)
	}

	log.Trace(ctx, "unmarshaled config file", "cfg", rawCfg)
	NormalizeKeys(rawCfg)
	log.Trace(ctx, "normalized keys", "cfg", rawCfg)

	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct // use default values
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     cfg,
	}

	log.Trace(ctx, "created default config", "cfg", cfg)

	d, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := d.Decode(rawCfg); err != nil {
		return fmt.Errorf("failed to decode the config file: %w", err)
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
			return 0, err
		}

		return v.Int(), nil
	}

	x, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q as an integer: %w", s, err)
	}

	return x, nil
}

// pathSliceValue resolves a slice of filesystem paths from the environment
// variables and the command-line flags to be used in the config.
func pathSliceValue(x []fspath.Path, opts ApplyOptions, entry *api.ConfigEntry) ([]fspath.Path, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	// TODO: There might be a more robust way to parse the paths, but this is
	// fine for now.
	if env != "" && (entry == nil || !entry.FlagOnly) {
		parts := strings.Split(env, ",")
		x = make([]fspath.Path, len(parts))

		for i, part := range parts {
			x[i] = fspath.Path(part)
		}
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetPathSlice(flagName)
		if err != nil {
			return nil, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	for i, p := range x {
		if !p.IsAbs() {
			path, err := fspath.NewAbs(string(opts.Dir), string(p))
			if err != nil {
				return nil, fmt.Errorf("failed to create absolute path from %q: %w", x, err)
			}

			x[i] = path.Clean()
		}
	}

	return x, nil
}

// pathValue resolves filesystem path from the environment variables and
// the command-line flags to be used in the config.
func pathValue(x fspath.Path, opts ApplyOptions, entry *api.ConfigEntry) (fspath.Path, error) {
	var err error

	env := pluginEnvValue(opts.idents, entry)

	if env != "" && (entry == nil || !entry.FlagOnly) {
		x = fspath.Path(env)
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetPath(flagName)
		if err != nil {
			return "", fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	if !x.IsAbs() {
		path, err := fspath.NewAbs(string(opts.Dir), string(x))
		if err != nil {
			return "", fmt.Errorf("failed to create absolute path from %q: %w", x, err)
		}

		x = path.Clean()
	}

	return x, nil
}

// pluginEnvValue returns the value of the environment variable for the given
// config identifiers, applying the environment variable name override from
// the plugin's config entry it is set.
func pluginEnvValue(idents []string, entry *api.ConfigEntry) string {
	if entry == nil || entry.EnvOverride == "" {
		return envValue(idents)
	}

	return os.Getenv(strings.ToUpper(defaultPrefix + "_" + entry.EnvOverride))
}

// pluginFlagName returns the name of the command-line flag for the given config
// identifiers, applying the flag name from the plugin's config entry it is set.
func pluginFlagName(idents []string, entry *api.ConfigEntry) string {
	if entry != nil {
		if entry.Flag == nil {
			return ""
		}

		if entry.Flag.Name == "" {
			return strings.ToLower(strings.ReplaceAll(configKey(idents), ".", "-"))
		}

		return entry.Flag.Name
	}

	key := configKey(idents)

	return FlagName(key)
}

// resolvePluginValue resolves the value of the given ConfigEntry and returns
// the parsed KeyVal.
//
//nolint:cyclop,funlen,gocognit,maintidx // need to check all of the types
func resolvePluginValue(raw any, entry *api.ConfigEntry, opts ApplyOptions) (api.KeyVal, error) {
	switch entry.Type {
	case api.BoolListValue:
		a, ok := raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := typeconv.ToBoolSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x, err = boolSliceValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.BoolValue:
		x, ok := raw.(bool)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to bool", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := boolValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.ConfigSliceValue:
		return api.KeyVal{}, fmt.Errorf(
			"%w: config entry %q has invalid type: %s",
			plugin.ErrInvalidConfig,
			entry.Key,
			entry.Type,
		)
	case api.IntListValue:
		a, ok := raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := typeconv.ToIntSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x, err = intSliceValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.IntValue:
		x, err := typeconv.ToInt(raw)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x, err = intValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.PathListValue:
		a, ok := raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := typeconv.ToPathSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x, err = pathSliceValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.PathValue:
		s, ok := raw.(string)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to string", typeconv.ErrConv, raw, entry.Key)
		}

		x := fspath.Path(s)

		x, err := pathValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.StringListValue:
		a, ok := raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := typeconv.ToStringSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x, err = stringSliceValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.StringValue:
		x, ok := raw.(string)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to string", typeconv.ErrConv, raw, entry.Key)
		}

		x, err := stringValue(x, opts, entry)
		if err != nil {
			return api.KeyVal{}, err
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	default:
		return api.KeyVal{}, fmt.Errorf(
			"%w: config entry %q has invalid type: %s",
			plugin.ErrInvalidConfig,
			entry.Key,
			entry.Type,
		)
	}
}

// setDir sets the correct config value for "Dir" at the start of the config
// struct parsing.
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

	log.Trace(
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

	newOpts := ApplyOptions{
		idents:  append(opts.idents, field.Name),
		Dir:     opts.Dir,
		FlagSet: opts.FlagSet,
		Store:   opts.Store,
	}

	if err := applyPath(val, newOpts); err != nil {
		return ApplyOptions{}, err
	}

	opts.Dir = fspath.Path(val.String())

	log.Trace(ctx, "set config field", "key", field.Name, "value", val)

	return opts, nil
}

// string resolves a slice of strings from the environment variables and
// the command-line flags to be used in the config.
func stringSliceValue(x []string, opts ApplyOptions, entry *api.ConfigEntry) ([]string, error) {
	var err error

	// TODO: Right now, we do not read the environment variable for this type.

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		x, err = opts.FlagSet.GetStringSlice(flagName)
		if err != nil {
			return nil, fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	return x, nil
}

// stringValue resolves a string value from the environment variables and
// the command-line flags to be used in the config.
func stringValue(x string, opts ApplyOptions, entry *api.ConfigEntry) (string, error) {
	env := pluginEnvValue(opts.idents, entry)

	if env != "" && (entry == nil || !entry.FlagOnly) {
		x = env
	}

	flagName := pluginFlagName(opts.idents, entry)

	if opts.FlagSet.Changed(flagName) {
		var err error

		x, err = opts.FlagSet.GetString(flagName)
		if err != nil {
			return "", fmt.Errorf("failed to get value for --%s: %w", flagName, err)
		}
	}

	return x, nil
}

// unmarshal converts s to the type of value by calling value's type's
// UnmarshalText function. It returns the actual value instead of a pointer to
// the value.
func unmarshal(value reflect.Value, s string) (reflect.Value, error) {
	ptr := reflect.New(value.Type())

	unmarshaler, ok := ptr.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return reflect.Value{}, fmt.Errorf("%w: type of %[2]q (%[2]T) to TextUnmarshaler", typeconv.ErrConv, value)
	}

	if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to unmarshal %q: %w", s, err)
	}

	return ptr.Elem(), nil
}
