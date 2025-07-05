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
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/logger"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/system"
	"github.com/reginald-project/reginald/internal/typeconv"
)

// errNoUnionMatch is returned by the union value resolver when the current
// alternative does not match the variable in the config.
var errNoUnionMatch = errors.New("union value does not match")

// TaskApplyOptions is the type for the options for the ApplyTasks function.
type TaskApplyOptions struct {
	// Store contains the discovered plugin. It should not be set when applying
	// the built-in config values
	Store           *plugin.Store
	Defaults        plugin.TaskDefaults // default options for the task types
	currentDefaults map[string]any      // default options for the currently-parsed task
	Dir             fspath.Path         // base directory for the program operations
}

// ApplyTasks applies the default values for tasks from the given defaults,
// assigns the IDs and other missing values, and normalizes paths. It returns
// new configs for the tasks.
func ApplyTasks(ctx context.Context, rawCfg []map[string]any, opts TaskApplyOptions) ([]plugin.TaskConfig, error) {
	if opts.Store == nil {
		panic("nil plugin store")
	}

	plugins := opts.Store.Plugins
	if len(plugins) == 0 {
		return nil, fmt.Errorf("cannot apply task config: %w", errNilPlugins)
	}

	result := make([]plugin.TaskConfig, 0)
	counts := make(map[string]int)

	for _, rawEntry := range rawCfg {
		slog.Log(ctx, slog.Level(logger.LevelTrace), "checking task map entry", "entry", rawEntry)

		rawType, ok := rawEntry["type"]
		if !ok {
			return nil, fmt.Errorf("%w: task without a type", ErrInvalidConfig)
		}

		ttName, ok := rawType.(string)
		if !ok {
			return nil, fmt.Errorf("%w: task type is not a string (%v)", ErrInvalidConfig, rawType)
		}

		task := opts.Store.Task(ttName)
		if task == nil {
			return nil, fmt.Errorf("%w: unknown task type %q", ErrInvalidConfig, ttName)
		}

		c, err := newTaskConfig(task, rawEntry, counts)
		if err != nil {
			return nil, err
		}

		if len(c.Platforms) > 0 && !c.Platforms.Current() {
			slog.DebugContext(
				ctx,
				"task not enabled on platform",
				"id",
				c.ID,
				"taskType",
				ttName,
				"platforms",
				c.Platforms,
			)

			continue
		}

		var defaults map[string]any

		defaults, ok = opts.Defaults[ttName]
		if !ok {
			defaults = map[string]any{}
		}

		opts.currentDefaults = defaults

		c.Config, err = resolveTaskConfigs(task, c.ID, rawEntry, opts)
		if err != nil {
			return nil, err
		}

		slog.Log(ctx, slog.Level(logger.LevelTrace), "task config parsed", "cfg", c)

		result = append(result, c)
	}

	if err := validateTasks(result); err != nil {
		return nil, err
	}

	return result, nil
}

// newTaskConfig creates a new TaskConfig for a config entry.
func newTaskConfig(task *plugin.Task, rawEntry map[string]any, counts map[string]int) (plugin.TaskConfig, error) {
	var taskID string

	ttName := task.TaskType

	rawID, ok := rawEntry["id"]
	if ok {
		taskID, ok = rawID.(string)
		if !ok {
			return plugin.TaskConfig{}, fmt.Errorf("%w: task ID is not a string (%v)", ErrInvalidConfig, rawID)
		}
	} else {
		taskID = ttName + "-" + strconv.Itoa(counts[ttName])
	}

	counts[ttName]++

	var strPlatforms []string

	rawPlatforms, ok := rawEntry["platforms"]
	if ok {
		var p string

		p, ok = rawPlatforms.(string)
		if ok {
			strPlatforms = append(strPlatforms, p)
		} else {
			strPlatforms, ok = rawPlatforms.([]string)
			if !ok {
				return plugin.TaskConfig{}, fmt.Errorf(
					"%w: platforms for task %q is not a list of strings",
					ErrInvalidConfig,
					taskID,
				)
			}
		}
	}

	platforms := make(system.OSes, len(strPlatforms))

	for i, s := range strPlatforms {
		platforms[i] = system.OS(s)
	}

	requires, err := resolveTaskRequirements(rawEntry["requires"], false)
	if err != nil {
		return plugin.TaskConfig{}, fmt.Errorf("failed to parse %q: %w", taskID, err)
	}

	return plugin.TaskConfig{
		Config:    nil,
		ID:        taskID,
		Platforms: platforms,
		Requires:  requires,
		TaskType:  ttName,
	}, nil
}

// parseTaskConfigValue parses the value of the given KeyValue from the task
// options and the defaults. It returns the parsed value and any errors it
// encounters.
//
//nolint:cyclop,funlen,gocognit,gocyclo,maintidx // need for complexity when checking the config type
func parseTaskConfigValue(entry api.ConfigValue, rawMap map[string]any, opts TaskApplyOptions) (api.KeyVal, error) {
	var err error

	raw := entry.Val

	if entry.Type == api.IntValue {
		if raw, err = entry.Int(); err != nil {
			return api.KeyVal{}, fmt.Errorf("type conversion for %q failed: %w", entry.Key, err)
		}
	}

	if len(opts.currentDefaults) > 0 {
		defaultsValue, ok := opts.currentDefaults[entry.Key]
		if ok {
			raw = defaultsValue
		}
	}

	fileValue, ok := rawMap[entry.Key]
	if ok {
		raw = fileValue
	}

	if raw, err = resolveTaskOSValue(raw, entry); err != nil && !errors.Is(err, errNoOSMap) {
		return api.KeyVal{}, err
	}

	switch entry.Type {
	case api.BoolListValue:
		if raw == nil {
			raw = []bool{}
		}

		var a []any

		a, ok = raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		var x []bool

		x, err = typeconv.ToBoolSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.BoolValue:
		if raw == nil {
			raw = false
		}

		var x bool

		x, ok = raw.(bool)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to bool", typeconv.ErrConv, raw, entry.Key)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.ConfigSliceValue:
		return api.KeyVal{}, fmt.Errorf("%w: %q has invalid type %q", plugin.ErrInvalidConfig, entry.Key, entry.Type)
	case api.IntListValue:
		if raw == nil {
			raw = []int{}
		}

		var a []any

		a, ok = raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		var x []int

		x, err = typeconv.ToIntSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.IntValue:
		if raw == nil {
			raw = 0
		}

		var x int

		x, err = typeconv.ToInt(raw)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.PathListValue:
		if raw == nil {
			raw = []string{}
		}

		var a []any

		a, ok = raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		var paths []fspath.Path

		paths, err = typeconv.ToPathSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		x := make([]fspath.Path, len(paths))

		for i, path := range paths {
			path, err = path.Expand()
			if err != nil {
				return api.KeyVal{}, fmt.Errorf("failed to expand %q: %w", path, err)
			}

			if !path.IsAbs() {
				path = fspath.Join(opts.Dir, path)
			}

			x[i] = path
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.PathValue:
		if raw == nil {
			raw = ""
		}

		var s string

		s, ok = raw.(string)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to string", typeconv.ErrConv, raw, entry.Key)
		}

		x := fspath.Path(s)

		x, err = x.Expand()
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to expand %q: %w", x, err)
		}

		if !x.IsAbs() {
			x = fspath.Join(opts.Dir, x)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.StringListValue:
		if raw == nil {
			raw = []string{}
		}

		var a []any

		a, ok = raw.([]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to []any", typeconv.ErrConv, raw, entry.Key)
		}

		var x []string

		x, err = typeconv.ToStringSlice(a)
		if err != nil {
			return api.KeyVal{}, fmt.Errorf("failed to convert type for %q: %w", entry.Key, err)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	case api.StringValue:
		if raw == nil {
			raw = ""
		}

		var x string

		x, ok = raw.(string)
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %[2]v in %q to string", typeconv.ErrConv, raw, entry.Key)
		}

		return api.KeyVal{
			Value: api.Value{Val: x, Type: entry.Type},
			Key:   entry.Key,
		}, nil
	default:
		return api.KeyVal{}, fmt.Errorf("%w: %q has invalid type %q", plugin.ErrInvalidConfig, entry.Key, entry.Type)
	}
}

// parseTaskMappedValue parses the value of the given MappedValue from the task
// options and the defaults. It returns the parsed value and any errors it
// encounters.
func parseTaskMappedValue(top any, entry api.MappedValue, opts TaskApplyOptions) (api.KeyVal, error) {
	topMap, ok := top.(map[string]any)
	if !ok {
		return api.KeyVal{}, fmt.Errorf("%w: failed to convert to a map: %[2]v (%[2]T)", ErrInvalidConfig, top)
	}

	kvs := make(api.KeyValues, 0, len(topMap))

	// The key is the dymanic value and the value should be a map.
	for topMapKey, topMapValue := range topMap {
		origKey := topMapKey //nolint:copyloopvar // we modify the value later and need to preserve the original

		rawValueMap, ok := topMapValue.(map[string]any)
		if !ok {
			return api.KeyVal{}, fmt.Errorf(
				"%w: failed to convert value for %q to a map: %[3]v (%[3]T)",
				ErrInvalidConfig,
				topMapKey,
				topMapValue,
			)
		}

		switch entry.KeyType { //nolint:exhaustive // other types are not supported
		case api.PathValue:
			path := fspath.Path(topMapKey)

			var err error

			path, err = path.Expand()
			if err != nil {
				return api.KeyVal{}, fmt.Errorf("failed to expand %q: %w", origKey, err)
			}

			if !path.IsAbs() {
				path = fspath.Join(opts.Dir, path)
			}

			topMapKey = string(path)
		case api.StringValue:
			// no-op
		default:
			return api.KeyVal{}, fmt.Errorf(
				"%w: keys in %q have invalid type %q",
				plugin.ErrInvalidConfig,
				entry.Key,
				entry.KeyType,
			)
		}

		values := make(api.KeyValues, 0, len(entry.Values))

		for _, configValue := range entry.Values {
			kv, err := parseTaskConfigValue(configValue, rawValueMap, opts)
			if err != nil {
				return api.KeyVal{}, fmt.Errorf("failed to parse value %q in %q: %w", configValue.Key, origKey, err)
			}

			values = append(values, kv)
		}

		kvs = append(kvs, api.KeyVal{
			Value: api.Value{Val: values, Type: api.ConfigSliceValue},
			Key:   topMapKey,
		})
	}

	return api.KeyVal{
		Value: api.Value{Val: kvs, Type: api.ConfigSliceValue},
		Key:   entry.Key,
	}, nil
}

// parseTaskUnionValue resolves and parses the value of the given UnionValue
// from the task options and the defaults. If there is no matching value in
// the config file, the function uses the first alternative config type to
// resolve the default value. It returns the parsed value, the resolved key, and
// any errors it encounters.
func parseTaskUnionValue(entry api.UnionValue, rawMap map[string]any, opts TaskApplyOptions) (api.KeyVal, error) {
	for _, alt := range entry.Alternatives {
		kv, err := resolveUnionValue(alt, rawMap, opts)
		if err != nil {
			if errors.Is(err, errNoUnionMatch) {
				continue
			}

			return api.KeyVal{}, err
		}

		return kv, nil
	}

	alt := entry.Alternatives[0]
	switch firstTyped := alt.(type) {
	case api.MappedValue:
		entry, ok := rawMap[firstTyped.Key]
		if !ok {
			// No default is set for the MappedValues.
			return api.KeyVal{
				Value: api.Value{Val: nil, Type: api.ConfigSliceValue},
				Key:   firstTyped.Key,
			}, nil
		}

		kv, err := parseTaskMappedValue(entry, firstTyped, opts)
		if err != nil {
			return api.KeyVal{}, err
		}

		return kv, nil
	case api.ConfigValue:
		kv, err := parseTaskConfigValue(firstTyped, rawMap, opts)
		if err != nil {
			return api.KeyVal{}, err
		}

		return kv, nil
	default:
		return api.KeyVal{}, fmt.Errorf(
			"%w: entry in UnionValue has invalid type: %[2]T (%[2]v)",
			plugin.ErrInvalidConfig,
			alt,
		)
	}
}

// resolveTaskConfigs resolves the values for a task instance.
func resolveTaskConfigs(
	task *plugin.Task,
	taskID string,
	rawEntry map[string]any,
	opts TaskApplyOptions,
) (api.KeyValues, error) {
	cfgs := make(api.KeyValues, 0, len(task.Config))
	ttName := task.TaskType

	// TODO: The defaults are now wrong, the functions try to check
	// the top-level map instead of the values for the current task type.

	for _, config := range task.Config {
		switch cfgTyped := config.(type) {
		case api.ConfigValue:
			kv, err := parseTaskConfigValue(cfgTyped, rawEntry, opts)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to parse value %q for %q (%s): %w",
					cfgTyped.Key,
					taskID,
					ttName,
					err,
				)
			}

			cfgs = append(cfgs, kv)
		case api.MappedValue:
			topValue := rawEntry[cfgTyped.Key]
			if topValue == nil {
				cfgs = append(cfgs, api.KeyVal{
					Value: api.Value{Val: nil, Type: api.ConfigSliceValue},
					Key:   cfgTyped.Key,
				})

				continue
			}

			kv, err := parseTaskMappedValue(topValue, cfgTyped, opts)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to parse value %q for %q (%s): %w",
					cfgTyped.Key,
					taskID,
					ttName,
					err,
				)
			}

			cfgs = append(cfgs, kv)
		case api.UnionValue:
			kv, err := parseTaskUnionValue(cfgTyped, rawEntry, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to parse value in UnionValue for %q (%s): %w", taskID, ttName, err)
			}

			cfgs = append(cfgs, kv)
		default:
			return nil, fmt.Errorf(
				"%w: config entry defined by task %q has invalid type: %[3]T (%[3]v)",
				plugin.ErrInvalidConfig,
				ttName,
				config,
			)
		}
	}

	return cfgs, nil
}

// resolveTaskRequirements resolves the requirements for a task instance from
// a string or slice or a map that contains different values for different OSes.
func resolveTaskRequirements(raw any, second bool) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	if r, ok := raw.(string); ok {
		if r == "" {
			return nil, nil
		}

		return []string{r}, nil
	}

	if r, ok := raw.([]string); ok {
		return r, nil
	}

	if second {
		return nil, fmt.Errorf("%w: requires is not a list of strings", ErrInvalidConfig)
	}

	var (
		m  map[string]any
		ok bool
	)

	if m, ok = raw.(map[string]any); !ok {
		return nil, fmt.Errorf("%w: requires is not a list of strings", ErrInvalidConfig)
	}

	for k, v := range m {
		if system.OS(k).Current() {
			return resolveTaskRequirements(v, true)
		}
	}

	var def any

	if def, ok = m["default"]; ok {
		return resolveTaskRequirements(def, true)
	}

	if def, ok = m["_"]; ok {
		return resolveTaskRequirements(def, true)
	}

	// If the current platform is not found, we should assume that the task has
	// no dependencies on that platform.
	return nil, nil
}

// resolveTaskOSValue resolves the raw config value for a task config entry from
// a map that contains different values for different OSes. It return errNoOSMap
// if the plugin value is not given as an OS map.
func resolveTaskOSValue(raw any, entry api.ConfigValue) (any, error) {
	var (
		m  map[string]any
		ok bool
	)

	// Maps are not allowed for config values so a simple test if the given
	// value is a map should be sufficient for checking if the user has given
	// different values for different OSes.
	if m, ok = raw.(map[string]any); !ok {
		return raw, errNoOSMap
	}

	for k, v := range m {
		if system.OS(k).Current() {
			return v, nil
		}
	}

	var def any

	if def, ok = m["default"]; ok {
		return def, nil
	}

	if def, ok = m["_"]; ok {
		return def, nil
	}

	return nil, fmt.Errorf("%w: %q has no config value for current platform", ErrInvalidConfig, entry.Key)
}

// resolveUnionValue resolves a single alternative in a UnionValue. If
// the alternative matches the config, the function returns the KeyVal. If
// the alternative does not match, the function returns errNoUnionMatch.
// Otherwise, it returns the encountered error.
func resolveUnionValue(alternative api.ConfigType, rawMap map[string]any, opts TaskApplyOptions) (api.KeyVal, error) {
	switch altTyped := alternative.(type) {
	case api.MappedValue:
		entry, ok := rawMap[altTyped.Key]
		if !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %q", errNoUnionMatch, altTyped.Key)
		}

		if _, ok = entry.(map[string]any); !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %q", errNoUnionMatch, altTyped.Key)
		}

		kv, err := parseTaskMappedValue(entry, altTyped, opts)
		if err != nil {
			return api.KeyVal{}, err
		}

		return kv, nil
	case api.ConfigValue:
		if _, ok := rawMap[altTyped.Key]; !ok {
			return api.KeyVal{}, fmt.Errorf("%w: %q", errNoUnionMatch, altTyped.Key)
		}

		if _, ok := rawMap[altTyped.Key].(map[string]any); ok {
			return api.KeyVal{}, fmt.Errorf("%w: %q", errNoUnionMatch, altTyped.Key)
		}

		kv, err := parseTaskConfigValue(altTyped, rawMap, opts)
		if err != nil {
			if errors.Is(err, ErrInvalidConfig) {
				return api.KeyVal{}, fmt.Errorf("%w: %q", errNoUnionMatch, altTyped.Key)
			}

			return api.KeyVal{}, err
		}

		return kv, nil
	default:
		return api.KeyVal{}, fmt.Errorf(
			"%w: entry in UnionValue has invalid type: %[2]T (%[2]v)",
			plugin.ErrInvalidConfig,
			alternative,
		)
	}
}

// validateTasks does a basic validation of the task configs. It checks that
// the IDs are unique and that the requirements point to existing tasks. More
// validations are added when needed.
func validateTasks(tasks []plugin.TaskConfig) error {
	seenIDs := make(map[string]struct{})

	for _, task := range tasks {
		if _, ok := seenIDs[task.ID]; ok {
			return fmt.Errorf("%w: duplicate task ID %q", ErrInvalidConfig, task.ID)
		}

		seenIDs[task.ID] = struct{}{}
	}

	// Loop twice rather than using nested loops as it is much faster and scales
	// better if the user has a big config file.
	for _, task := range tasks {
		for _, r := range task.Requires {
			if _, ok := seenIDs[r]; !ok {
				return fmt.Errorf("%w: task %q requires an unknown task %q", ErrInvalidConfig, task.ID, r)
			}
		}
	}

	return nil
}
