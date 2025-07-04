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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/system"
)

// Constants for the node visit statuses when traversing TaskGraph.
const (
	unvisited visitState = iota
	visiting
	visited
)

// Errors returned by the graph functions.
var (
	errCycle = errors.New("circular task dependencies detected")
	errNilID = errors.New("task config with empty ID")
)

// A Task is the program representation of a plugin task type that is defined in
// the manifest.
type Task struct {
	// Plugin is the plugin that this task is defined in.
	Plugin Plugin
	api.Task
}

// A TaskConfig is the config for a task instance.
type TaskConfig struct {
	// TaskType is the type of this task. It defines which task implementation
	// is called when this task is executed.
	TaskType string

	// ID is the unique ID for this task. It must be unique. The ID must also be
	// different from the provided task types.
	ID string

	// Config contains the parsed config values for the task.
	Config api.KeyValues

	// Requires contains the task IDs or types that this task depends on.
	Requires []string

	// Platforms contains the operating systems to run the task on. Empty slice
	// means that the task is run on every operating system.
	Platforms system.OSes

	// run tells whether this task instance is already run.
	run bool
}

// TaskDefaults is the type for the default config values set for the tasks.
type TaskDefaults map[string]map[string]any

// taskGraph is a graph of TaskNodes that can be sorted topographically
// to determine the execution order of the task instances.
type taskGraph map[string]*taskNode

// A taskNode is a node in the task graph that determines the execution order of
// the tasks.
type taskNode struct {
	id           string      // ID of the task in question
	taskType     string      // type of the task in question
	dependencies []string    // dependencies of the task in question
	dependents   []*taskNode // nodes for the tasks that are dependent on this task
	degreeIn     int         // number of incoming edges
}

// visitState is the type for the visit indicator during the cycle detection in
// TaskGraph.
type visitState int

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
		attrs[i] = slog.Any(task.Plugin.Manifest().Domain+"/"+task.TaskType, task)
	}

	return slog.GroupValue(attrs...)
}

// LogValue implements [slog.LogValuer] for Task. It returns a group value for
// logging a Task.
func (t *Task) LogValue() slog.Value {
	if t == nil {
		return slog.StringValue("<nil>")
	}

	return slog.GroupValue(slog.String("type", t.TaskType), slog.String("description", t.Description))
}

// RunTask runs a task by calling the correct plugin.
func RunTask(ctx context.Context, store *Store, cfg *TaskConfig, tasks []TaskConfig) error {
	if store == nil {
		panic("calling RunTask with nil store")
	}

	if cfg.run {
		slog.DebugContext(ctx, "task already run", "task", cfg.ID)

		return nil
	}

	task := store.Task(cfg.TaskType)
	if task == nil {
		panic("calling Run on nil task")
	}

	if task.Plugin == nil {
		panic(fmt.Sprintf("task %q has nil plugin", task.TaskType))
	}

	if err := store.start(ctx, task.Plugin, tasks); err != nil {
		return err
	}

	i := strings.IndexByte(task.TaskType, '/')
	if i == -1 {
		panic("invalid task type: " + task.TaskType)
	}

	tt := task.TaskType[i+1:]

	if err := callRunTask(ctx, task.Plugin, tt, cfg); err != nil {
		return err
	}

	cfg.run = true

	return nil
}

// newCycleError formats and returns an error for circular dependencies.
func newCycleError(startNode *taskNode, stack []*taskNode) error {
	path := ""
	startIndex := -1

	for i, node := range stack {
		if node.id == startNode.id {
			startIndex = i

			break
		}
	}

	for i := startIndex; i < len(stack); i++ {
		path += stack[i].id + " -> "
	}

	path += startNode.id

	return fmt.Errorf("%w: %s", errCycle, path)
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
		// Normalize the task type here by adding the plugin domain.
		t.TaskType = manifest.Domain + "/" + t.TaskType
		tasks[i] = &Task{
			Plugin: plugin,
			Task:   t,
		}
	}

	return tasks
}

// newTaskGraph returns a new TaskGraph built from the given task configuration.
func newTaskGraph(cfgs []TaskConfig) (taskGraph, error) {
	graph := make(taskGraph)

	for _, cfg := range cfgs {
		if cfg.ID == "" {
			// TODO: Automatically add the missing tasks if a dependency is just
			// a task type. This should be done earlier and not here, but this
			// warrants a panic once this is implemented.
			// panic(fmt.Sprintf("%v: task of type %s", errNilID, cfg.Type))
			return nil, fmt.Errorf("%w: task of type %s", errNilID, cfg.TaskType)
		}

		graph[cfg.ID] = &taskNode{
			id:           cfg.ID,
			taskType:     cfg.TaskType,
			dependencies: cfg.Requires, // dependencies should be normalized before this
			dependents:   make([]*taskNode, 0),
			degreeIn:     0,
		}
	}

	for _, node := range graph {
		for _, d := range node.dependencies {
			depNode, ok := graph[d]
			if !ok {
				panic(
					fmt.Sprintf(
						"task %q (type %s) has unknown dependency: %s",
						node.id,
						node.taskType,
						d,
					),
				)
			}

			depNode.dependents = append(depNode.dependents, node)
			node.degreeIn++
		}
	}

	if err := graph.checkCycles(); err != nil {
		return nil, err
	}

	return graph, nil
}

// checkCycles checks if g contains cycles and returns an error if it does.
func (g taskGraph) checkCycles() error {
	state := make(map[string]visitState, len(g))

	for id := range g {
		state[id] = unvisited
	}

	for id, node := range g {
		if state[id] == unvisited {
			stack := make([]*taskNode, 0)
			if err := visit(node, state, &stack); err != nil {
				return err
			}
		}
	}

	return nil
}

// sorted returns g as a topologically sorted list of stages for running. Each
// element of the slice is a slice that contains the tasks that can be executed
// in parallel.
func (g taskGraph) sorted() ([][]*taskNode, error) {
	queue := make([]*taskNode, 0)

	for _, node := range g {
		if node.degreeIn == 0 {
			queue = append(queue, node)
		}
	}

	var ( //nolint:prealloc // no need to preallocate here
		stages [][]*taskNode
		sorted []*taskNode
	)

	for len(queue) > 0 {
		current := make([]*taskNode, len(queue))

		copy(current, queue)

		stages = append(stages, current)

		queue = nil

		for _, node := range current {
			sorted = append(sorted, node)

			for _, dependent := range node.dependents {
				dependent.degreeIn--
				if dependent.degreeIn == 0 {
					queue = append(queue, dependent)
				}
			}
		}
	}

	if len(sorted) != len(g) {
		return nil, errCycle
	}

	return stages, nil
}

func visit(node *taskNode, state map[string]visitState, stack *[]*taskNode) error {
	state[node.id] = visiting

	*stack = append(*stack, node)

	for _, dependent := range node.dependents {
		switch state[dependent.id] {
		case unvisited:
			if err := visit(dependent, state, stack); err != nil {
				return err
			}
		case visiting:
			return newCycleError(dependent, *stack)
		case visited:
			continue
		default:
			panic(
				fmt.Sprintf(
					"invalid state while visiting task graph node: %v",
					state[dependent.id],
				),
			)
		}
	}

	state[node.id] = visited
	*stack = (*stack)[:len(*stack)-1]

	return nil
}
