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
	"errors"
	"fmt"
	"log/slog"

	"github.com/anttikivi/reginald/internal/plugins"
)

// Errors returned by the general taks functions.
var (
	ErrDuplicate = errors.New("duplicate task")
)

// TaskTypes is a map of the available task types by type.
type TaskTypes map[string]*Task

// A Task is a task within Reginald.
type Task struct {
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
				Type: info.Type,
			}

			tasks[t.Type] = t
		}
	}

	return tasks, nil
}

// LogValue implements [slog.LogValuer] for TaskTypes.
func (t TaskTypes) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, len(t))

	for k, v := range t {
		attrs = append(attrs, slog.Any(k, *v))
	}

	return slog.GroupValue(attrs...)
}
