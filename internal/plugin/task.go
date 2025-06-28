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

package plugin

import (
	"log/slog"

	"github.com/reginald-project/reginald-sdk-go/api"
)

// A Task is the program representation of a plugin task type that is defined in
// the manifest.
type Task struct {
	// Plugin is the plugin that this task is defined in.
	Plugin Plugin

	api.Task
}

// logTasks is a helper type for logging a slice of tasks.
type logTasks []*Task

// LogValue implements [slog.LogValuer] for logTasks. It formats the slice of
// tasks as a group correctly for the different types of [slog.Handler] in use.
func (t logTasks) LogValue() slog.Value {
	if len(t) == 0 {
		return slog.StringValue("<nil>")
	}

	attrs := make([]slog.Attr, len(t))
	for i, task := range t {
		attrs[i] = slog.Any(task.Plugin.Manifest().Domain+"/"+task.Type, task)
	}

	return slog.GroupValue(attrs...)
}

// LogValue implements [slog.LogValuer] for TAsk. It returns a group value for
// logging a Task.
func (t *Task) LogValue() slog.Value {
	if t == nil {
		return slog.StringValue("<nil>")
	}

	return slog.GroupValue(slog.String("type", t.Type), slog.String("description", t.Description))
}

// newTasks creates the internal task representations for the given plugin. It
// panics if the plugin is nil.
func newTasks(plugin Plugin) []*Task {
	if plugin == nil {
		panic("creating tasks for nil plugin")
	}

	manifest := plugin.Manifest()
	if manifest == nil || len(manifest.Tasks) == 0 {
		return nil
	}

	tasks := make([]*Task, len(manifest.Tasks))

	for i, t := range manifest.Tasks {
		tasks[i] = &Task{
			Plugin: plugin,
			Task:   t,
		}
	}

	return tasks
}
