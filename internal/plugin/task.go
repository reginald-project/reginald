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
	// Type is the type of this task. It defines which task implementation is
	// called when this task is executed.
	Type string

	// ID is the unique ID for this task. It must be unique. The ID must also be
	// different from the provided task types.
	ID string

	// Config contains the parsed config values for the task.
	Config api.KeyValues

	// Requires contains the task IDs or types that this task depends on.
	Requires TaskRequirements

	// Platforms contains the operating systems to run the task on. Empty slice
	// means that the task is run on every operating system.
	Platforms system.OSes
}

// TaskDefaults is the type for the default config values set for the tasks.
type TaskDefaults map[string]map[string]any

// TaskGraph is a graph of TaskNodes that can be sorted topographically
// to determine the execution order of the task instances.
type TaskGraph map[string]*TaskNode

// A TaskNode is a node in the task graph that determines the execution order of
// the tasks.
type TaskNode struct {
	ID           string      // ID of the task in question
	Type         string      // type of the task in question
	Dependencies []string    // dependencies of the task in question
	Dependents   []*TaskNode // nodes for the tasks that are dependent on this task
	DegreeIn     int         // number of incoming edges
}

// TaskRequirements is list of tasks a task depends on.
type TaskRequirements []string

// logTasks is a helper type for logging a slice of tasks.
type logTasks []*Task

// visitState is the type for the visit indicator during the cycle detection in
// TaskGraph.
type visitState int

// NewTaskGraph returns a new TaskGraph built from the given task configuration.
func NewTaskGraph(cfgs []TaskConfig) (TaskGraph, error) {
	graph := make(TaskGraph)

	for _, cfg := range cfgs {
		if cfg.ID == "" {
			// TODO: Automatically add the missing tasks if a dependency is just
			// a task type. This should be done earlier and not here, but this
			// warrants a panic once this is implemented.
			// panic(fmt.Sprintf("%v: task of type %s", errNilID, cfg.Type))
			return nil, fmt.Errorf("%w: task of type %s", errNilID, cfg.Type)
		}

		graph[cfg.ID] = &TaskNode{
			ID:           cfg.ID,
			Type:         cfg.Type,
			Dependencies: cfg.Requires, // dependencies should be normalized before this
			Dependents:   make([]*TaskNode, 0),
			DegreeIn:     0,
		}
	}

	for _, node := range graph {
		for _, d := range node.Dependencies {
			depNode, ok := graph[d]
			if !ok {
				panic(
					fmt.Sprintf(
						"task %q (type %s) has unknown dependency: %s",
						node.ID,
						node.Type,
						d,
					),
				)
			}

			depNode.Dependents = append(depNode.Dependents, node)
			node.DegreeIn++
		}
	}

	if err := graph.checkCycles(); err != nil {
		return nil, err
	}

	return graph, nil
}

// Sorted returns g as a topologically sorted list of stages for running. Each
// element of the slice is a slice that contains the tasks that can be executed
// in parallel.
func (g TaskGraph) Sorted() ([][]*TaskNode, error) {
	queue := make([]*TaskNode, 0)

	for _, node := range g {
		if node.DegreeIn == 0 {
			queue = append(queue, node)
		}
	}

	var ( //nolint:prealloc // no need to preallocate here
		stages [][]*TaskNode
		sorted []*TaskNode
	)

	for len(queue) > 0 {
		current := make([]*TaskNode, len(queue))

		copy(current, queue)

		stages = append(stages, current)

		queue = nil

		for _, node := range current {
			sorted = append(sorted, node)

			for _, dependent := range node.Dependents {
				dependent.DegreeIn--
				if dependent.DegreeIn == 0 {
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

// UnmarshalText implements [encoding.TextUnmarshaler]. It decodes a single
// string into TaskRequirements.
func (r *TaskRequirements) UnmarshalText(data []byte) error { //nolint:unparam // implements interface
	if len(data) == 0 {
		*r = make(TaskRequirements, 0)

		return nil
	}

	parts := strings.Split(string(data), ",")
	out := make(TaskRequirements, len(parts))

	copy(out, parts)

	*r = out

	return nil
}

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

// checkCycles checks if g contains cycles and returns an error if it does.
func (g TaskGraph) checkCycles() error {
	state := make(map[string]visitState, len(g))

	for id := range g {
		state[id] = unvisited
	}

	for id, node := range g {
		if state[id] == unvisited {
			stack := make([]*TaskNode, 0)
			if err := visit(node, state, &stack); err != nil {
				return err
			}
		}
	}

	return nil
}

// newCycleError formats and returns an error for circular dependencies.
func newCycleError(startNode *TaskNode, stack []*TaskNode) error {
	path := ""
	startIndex := -1

	for i, node := range stack {
		if node.ID == startNode.ID {
			startIndex = i

			break
		}
	}

	for i := startIndex; i < len(stack); i++ {
		path += stack[i].ID + " -> "
	}

	path += startNode.ID

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
		tasks[i] = &Task{
			Plugin: plugin,
			Task:   t,
		}
	}

	return tasks
}

func visit(node *TaskNode, state map[string]visitState, stack *[]*TaskNode) error {
	state[node.ID] = visiting

	*stack = append(*stack, node)

	for _, dependent := range node.Dependents {
		switch state[dependent.ID] {
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
					state[dependent.ID],
				),
			)
		}
	}

	state[node.ID] = visited
	*stack = (*stack)[:len(*stack)-1]

	return nil
}
