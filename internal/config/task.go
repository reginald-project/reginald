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
	"fmt"
	"strconv"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log"
	"github.com/reginald-project/reginald/internal/plugin"
)

// TaskApplyOptions is the type for the options for the ApplyTasks function.
type TaskApplyOptions struct {
	// Store contains the discovered plugin. It should not be set when applying
	// the built-in config values
	Store    *plugin.Store
	Defaults plugin.TaskDefaults // default options for the task types
	Dir      fspath.Path         // base directory for the program operations
}

// ApplyTasks applies the default values for tasks from the given defaults,
// assigns the IDs and other missing values, and normalizes paths. It returns
// new configs for the tasks.
func ApplyTasks(ctx context.Context, cfg []plugin.TaskConfig, opts TaskApplyOptions) ([]plugin.TaskConfig, error) {
	log.Debug(ctx, "applying task configs")

	if opts.Store == nil {
		panic("nil plugin store")
	}

	plugins := opts.Store.Plugins
	if len(plugins) == 0 {
		return nil, fmt.Errorf("cannot apply task config: %w", errNilPlugins)
	}

	taskCfg := make([]plugin.TaskConfig, 0, len(cfg))
	counts := make(map[string]int)

	for _, tc := range cfg { //nolint:varnamelen
		// TODO: Remove the tasks that are not run on the current platform.
		log.Trace(ctx, "finding task", "type", tc.Type)

		tt := opts.Store.Task(tc.Type)
		if tt == nil {
			return nil, fmt.Errorf("%w: unknown task type %q", ErrInvalidConfig, tc.Type)
		}

		id := tc.ID //nolint:varnamelen
		if id == "" {
			id = tc.Type + "-" + strconv.Itoa(counts[tc.Type])
		}

		counts[tc.Type]++

		options := make(plugin.TaskOptions, len(tt.Config))

		for _, config := range tt.Config {
			switch cfgValue := config.(type) {
			case api.KeyValue:
				log.Trace(
					ctx,
					"parsing task config value as KeyValue",
					"id",
					id,
					"task",
					tc.Type,
					"key",
					cfgValue.Key,
					"kvType",
					cfgValue.Type,
				)

				value, err := parseTaskKeyValue(cfgValue, tc.Options, opts)
				if err != nil {
					return nil, fmt.Errorf("failed to parse value for %q (%s): %w", id, tc.Type, err)
				}

				log.Trace(
					ctx,
					"setting task value",
					"key",
					cfgValue.Key,
					"id",
					id,
					"task",
					tc.Type,
					"kvType",
					cfgValue.Type,
					"value",
					value,
					"type",
					fmt.Sprintf("%T", value),
				)

				options[cfgValue.Key] = value
			case api.UnionValue:
				log.Trace(
					ctx,
					"parsing task config value as UnionValue",
					"id",
					id,
					"task",
					tc.Type,
					"alternatives",
					cfgValue.Alternatives,
				)

				value, key, err := parseTaskUnionValue(cfgValue, tc.Options, opts)
				if err != nil {
					return nil, fmt.Errorf("failed to parse value for %q (%s): %w", id, tc.Type, err)
				}

				options[key] = value
			case api.MappedValue:
				log.Trace(
					ctx,
					"parsing task config value as MappedValue",
					"id",
					id,
					"task",
					tc.Type,
					"key",
					cfgValue.Key,
					"keyType",
					cfgValue.KeyType,
				)

				topValue := tc.Options[cfgValue.Key]
				if topValue == nil {
					continue
				}

				log.Trace(ctx, "got the top map", "key", cfgValue, "map", topValue)

				value, err := parseTaskMappedValue(topValue, cfgValue, opts)
				if err != nil {
					return nil, fmt.Errorf("failed to parse value for %q (%s): %w", id, tc.Type, err)
				}

				log.Trace(
					ctx,
					"setting MappedValue to options",
					"key",
					cfgValue.Key,
					"id",
					id,
					"task",
					tc.Type,
					"map", value,
				)

				options[cfgValue.Key] = value
			default:
				return nil, fmt.Errorf(
					"%w: config entry defined by task %q has invalid type: %[3]T (%[3]v)",
					plugin.ErrInvalidConfig,
					tc.Type,
					config,
				)
			}
		}

		c := plugin.TaskConfig{
			ID:        id,
			Type:      tc.Type,
			Options:   options,
			Platforms: tc.Platforms,
			Requires:  tc.Requires,
		}

		taskCfg = append(taskCfg, c)
	}

	return taskCfg, nil
}

// parseTaskKeyValue parses the value of the given KeyValue from the task
// options and the defaults. It returns the parsed value and any errors it
// encounters.
//
//nolint:cyclop,funlen,gocognit,gocyclo // need for complexity when checking the config type
func parseTaskKeyValue(kv api.KeyValue, taskOptions plugin.TaskOptions, opts TaskApplyOptions) (any, error) {
	value := kv.Value

	if kv.Type == api.IntValue {
		var err error

		if value, err = kv.Int(); err != nil {
			return nil, fmt.Errorf("failed to convert %v to int: %w", value, err)
		}
	}

	if opts.Defaults != nil {
		defaultsValue, ok := opts.Defaults[kv.Key]
		if ok {
			value = defaultsValue
		}
	}

	fileValue, ok := taskOptions[kv.Key]
	if ok {
		value = fileValue
	}

	switch kv.Type {
	case api.BoolListValue:
		if value == nil {
			value = []bool{}
		}

		value, ok = value.([]bool)
	case api.BoolValue:
		if value == nil {
			value = false
		}

		value, ok = value.(bool)
	case api.IntListValue:
		if value == nil {
			value = []int{}
		}

		value, ok = value.([]int)
	case api.IntValue:
		if value == nil {
			value = 0
		}

		value, ok = value.(int)
	case api.MapValue:
		if value == nil {
			value = make(map[string]any)
		}

		value, ok = value.(map[string]any)
	case api.PathListValue:
		if value == nil {
			value = []fspath.Path{}
		}

		var a []string

		a, ok = value.([]string)
		if !ok {
			break
		}

		paths := make([]fspath.Path, len(a))

		for i, s := range a {
			path := fspath.Path(s)

			var err error

			path, err = path.Expand()
			if err != nil {
				return nil, fmt.Errorf("failed to expand %q: %w", path, err)
			}

			if !path.IsAbs() {
				path = fspath.Join(opts.Dir, path)
			}

			paths[i] = path
		}

		value = paths
	case api.PathValue:
		if value == nil {
			value = ""
		}

		var s string

		s, ok = value.(string)
		if !ok {
			break
		}

		path := fspath.Path(s)

		var err error

		path, err = path.Expand()
		if err != nil {
			return nil, fmt.Errorf("failed to expand %q: %w", path, err)
		}

		if !path.IsAbs() {
			path = fspath.Join(opts.Dir, path)
		}

		value = path
	case api.StringListValue:
		if value == nil {
			value = []string{}
		}

		value, ok = value.([]string)
	case api.StringValue:
		if value == nil {
			value = ""
		}

		value, ok = value.(string)
	default:
		return nil, fmt.Errorf("%w: %q has invalid type %q", plugin.ErrInvalidConfig, kv.Key, kv.Type)
	}

	if !ok {
		return nil, fmt.Errorf(
			"%w: value in %q has wrong type: want %s, got %[4]T (%[4]v)",
			ErrInvalidConfig,
			kv.Key,
			kv.Type,
			value,
		)
	}

	return value, nil
}

// parseTaskMappedValue parses the value of the given MappedValue from the task
// options and the defaults. It returns the parsed value and any errors it
// encounters.
func parseTaskMappedValue(top any, cfg api.MappedValue, opts TaskApplyOptions) (map[string]any, error) {
	topMap, ok := top.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: failed to convert to a map: %[2]v (%[2]T)", ErrInvalidConfig, top)
	}

	// The key is the dymanic value and the value should be a map.
	for topMapKey, topMapValues := range topMap {
		valueMap, ok := topMapValues.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"%w: failed to convert value for %q to a map: %[3]v (%[3]T)",
				ErrInvalidConfig,
				topMapKey,
				topMapValues,
			)
		}

		delete(topMap, topMapKey)

		switch cfg.KeyType { //nolint:exhaustive // other types are not supported
		case api.PathValue:
			path := fspath.Path(topMapKey)

			var err error

			path, err = path.Expand()
			if err != nil {
				return nil, fmt.Errorf("failed to expand %q: %w", path, err)
			}

			if !path.IsAbs() {
				path = fspath.Join(opts.Dir, path)
			}

			topMapKey = string(path)
		case api.StringValue:
			// no-op
		default:
			return nil, fmt.Errorf("%w: keys in %q have invalid type %q", plugin.ErrInvalidConfig, cfg.Key, cfg.KeyType)
		}

		for _, kv := range cfg.Values {
			value, err := parseTaskKeyValue(kv, valueMap, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to parse value in %q: %w", topMapKey, err)
			}

			valueMap[kv.Key] = value
		}

		topMap[topMapKey] = topMapValues
	}

	return topMap, nil
}

// parseTaskUnionValue resolves and parses the value of the given UnionValue
// from the task options and the defaults. If there is no matching value in
// the config file, the function uses the first alternative config type to
// resolve the default value. It returns the parsed value, the resolved key, and
// any errors it encounters.
func parseTaskUnionValue(cfg api.UnionValue, cfgOptions map[string]any, opts TaskApplyOptions) (any, string, error) {
	var (
		err   error
		key   string
		value any
	)

AltLoop:
	for _, alt := range cfg.Alternatives {
		switch altTyped := alt.(type) {
		case api.MappedValue:
			entry, ok := cfgOptions[altTyped.Key]
			if !ok {
				continue
			}

			var v map[string]any

			v, err = parseTaskMappedValue(entry, altTyped, opts)
			if len(v) == 0 || err != nil {
				continue
			}

			value = v
			key = altTyped.Key

			break AltLoop
		case api.KeyValue:
			value, err = parseTaskKeyValue(altTyped, cfgOptions, opts)
			if value == nil || err != nil {
				continue
			}

			key = altTyped.Key

			break AltLoop
		default:
			return nil, "", fmt.Errorf(
				"%w: entry in UnionValue has invalid type: %[2]T (%[2]v)",
				plugin.ErrInvalidConfig,
				alt,
			)
		}
	}

	if value == nil {
		alt := cfg.Alternatives[0]
		switch firstTyped := alt.(type) {
		case api.MappedValue:
			entry, ok := cfgOptions[firstTyped.Key]
			if !ok {
				// No default is set for the MappedValues, so we return a nil
				// value but the right key.
				return nil, firstTyped.Key, nil
			}

			value, err = parseTaskMappedValue(entry, firstTyped, opts)
			if err != nil {
				return nil, "", err
			}

			key = firstTyped.Key
		case api.KeyValue:
			value, err = parseTaskKeyValue(firstTyped, cfgOptions, opts)
			if err != nil {
				return nil, "", err
			}

			key = firstTyped.Key
		default:
			return nil, "", fmt.Errorf(
				"%w: entry in UnionValue has invalid type: %[2]T (%[2]v)",
				plugin.ErrInvalidConfig,
				alt,
			)
		}
	}

	return value, key, nil
}
