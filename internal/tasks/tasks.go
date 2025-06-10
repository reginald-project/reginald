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

// Package tasks contains the built-in tasks of Reginald the internal
// task-related utilities for running tasks and validating the task configs.
package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/logging"
	"github.com/anttikivi/reginald/internal/plugins"
	"github.com/anttikivi/reginald/internal/taskcfg"
	"github.com/anttikivi/reginald/pkg/rpp"
)

// Errors returned by the general taks functions.
var (
	ErrDuplicate   = errors.New("duplicate task")
	errNilRun      = errors.New("func Run is nil")
	errNilValidate = errors.New("func Validate is nil")
)

// TaskTypes is a map of the available task types by type.
type TaskTypes map[string]*Task

// A Task is a task within Reginald.
type Task struct {
	// Validate validates the given cfg for this task type.
	Validate func(ctx context.Context, t *Task, opts taskcfg.Options) error

	// Run runs the task.
	Run func(ctx context.Context, t *Task, dir fspath.Path, opts taskcfg.Options) error

	// Type is the name of the task type.
	Type string
}

// Tasks returns the task instances that are available.
func Tasks(ps []*plugins.Plugin) (TaskTypes, error) {
	tasks := make(TaskTypes)

	tasks["link"] = NewLink()

	for _, p := range ps {
		for _, info := range p.Tasks {
			if _, ok := tasks[info.Type]; ok {
				return nil, fmt.Errorf(
					"%w: plugin %q tried to add task %q but it already exists",
					ErrDuplicate,
					p.Name,
					info.Type,
				)
			}

			t := &Task{
				Validate: func(ctx context.Context, t *Task, opts taskcfg.Options) error {
					if err := p.ValidateTask(ctx, t.Type, opts); err != nil {
						return fmt.Errorf("%w", err)
					}

					return nil
				},
				Run: func(ctx context.Context, t *Task, dir fspath.Path, opts taskcfg.Options) error {
					if err := p.RunTask(ctx, t.Type, dir, opts); err != nil {
						return fmt.Errorf("%w", err)
					}

					return nil
				},
				Type: info.Type,
			}

			tasks[t.Type] = t
		}
	}

	return tasks, nil
}

// Configure propagates the default values for task configs, assigns missing
// task IDs, and validates the task configs. The functions returns the new tasks
// configs and does not edit the slice in place.
func Configure(
	ctx context.Context,
	cfg []taskcfg.Config,
	defaults taskcfg.Defaults,
	types TaskTypes,
) ([]taskcfg.Config, error) {
	result := make([]taskcfg.Config, 0, len(cfg))
	counts := make(map[string]int)

	if err := validateDefaults(ctx, defaults, types); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	for _, tc := range cfg {
		task, ok := types[tc.Type]
		if !ok {
			return nil, fmt.Errorf("%w: task type %q not found", config.ErrInvalidConfig, tc.Type)
		}

		id := tc.ID
		if id == "" {
			id = tc.Type + "-" + strconv.Itoa(counts[task.Type])
		}

		counts[task.Type]++

		c := taskcfg.Config{
			Options: tc.Options,
			ID:      id,
			Type:    task.Type,
		}

		if def, ok := defaults[task.Type]; ok {
			m, ok := def.(map[string]any)
			if !ok {
				return nil, fmt.Errorf(
					"%w: defaults for task type %q are not a map but %[3]T: %[3]v",
					config.ErrInvalidConfig,
					task.Type,
					def,
				)
			}

			addDefaults(&c, m)
		}

		logging.TraceContext(
			ctx,
			"running task validation",
			"id",
			c.ID,
			"type",
			c.Type,
			"options",
			c.Options,
		)

		if task.Validate == nil {
			return nil, fmt.Errorf(
				"cannot check config for %q (type %q): %w",
				id,
				task.Type,
				errNilValidate,
			)
		}

		if err := validate(ctx, task, c.Options); err != nil {
			return nil, fmt.Errorf("%w: ID %q", err, id)
		}

		result = append(result, c)
	}

	logging.DebugContext(ctx, "task config parsed", "cfg", result)

	return result, nil
}

// Run runs the tasks defined by cfg. The configuration does not guarantee
// the execution order, but Run resolves the defined dependencies and executes
// according to them.
func Run(ctx context.Context, cfg *config.Config, types TaskTypes) error {
	for _, tc := range cfg.Tasks {
		task, ok := types[tc.Type]
		if !ok {
			return fmt.Errorf("%w: task type %q not found", config.ErrInvalidConfig, tc.Type)
		}

		logging.TraceContext(
			ctx,
			"running task",
			"id",
			tc.ID,
			"type",
			task.Type,
			"options",
			tc.Options,
		)

		if task.Run == nil {
			return fmt.Errorf("cannot run task %q (type %q): %w", tc.ID, task.Type, errNilRun)
		}

		if err := task.Run(ctx, task, cfg.Directory, tc.Options); err != nil {
			return fmt.Errorf("failed to run task %q (type %q): %w", tc.ID, task.Type, err)
		}
	}

	return nil
}

// LogValue implements [slog.LogValuer] for TaskTypes.
func (t TaskTypes) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, len(t))

	for k, v := range t {
		attrs = append(attrs, slog.Any(k, *v))
	}

	return slog.GroupValue(attrs...)
}

// addDefaults adds the default config values to the given task Config. It
// modifies the config in place.
func addDefaults(cfg *taskcfg.Config, defaults map[string]any) {
	// This should never be nil, but safeguard.
	if cfg.Options == nil {
		cfg.Options = make(map[string]any)
	}

	for k, v := range defaults {
		if _, ok := cfg.Options[k]; ok {
			continue
		}

		cfg.Options[k] = v
	}
}

// validate is a helper function that runs the Validate function from the task
// and resolves the error type to return a more informative message.
func validate(ctx context.Context, t *Task, opts taskcfg.Options) error {
	if err := t.Validate(ctx, t, opts); err != nil {
		if errors.Is(err, config.ErrInvalidConfig) {
			return fmt.Errorf("invalid config for %q: %w", t.Type, err)
		}

		var rppErr *rpp.Error
		if errors.As(err, &rppErr) && rppErr.Code == rpp.InvalidConfig {
			return fmt.Errorf("invalid config for %q: %w", t.Type, err)
		}

		return fmt.Errorf("failed to validate config for %q: %w", t.Type, err)
	}

	return nil
}

// validateDefaults runs the task config validation for the defaults given for
// each task. The defaults should pass the same validation check as the actual
// config values.
func validateDefaults(ctx context.Context, defaults taskcfg.Defaults, types TaskTypes) error {
	for tt, opts := range defaults {
		t, ok := types[tt]
		if !ok {
			return fmt.Errorf("%w: task type %q not found", config.ErrInvalidConfig, tt)
		}

		if t.Validate == nil {
			return fmt.Errorf("cannot check defaults for %q: %w", tt, errNilValidate)
		}

		m, ok := opts.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"%w: defaults for task type %q are not a map but %[3]T: %[3]v",
				config.ErrInvalidConfig,
				tt,
				opts,
			)
		}

		logging.TraceContext(ctx, "running defaults validation", "type", tt, "options", m)

		if err := validate(ctx, t, m); err != nil {
			return fmt.Errorf("%w: invalid defaults", err)
		}
	}

	logging.DebugContext(ctx, "defaults validated", "defaults", defaults)

	return nil
}
