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

package taskcfg

import (
	"errors"
	"fmt"
)

// Constants for the node visit statuses when traversing Graph.
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

// Graph is a task graph that can be sorted to execute the tasks in order,
// i.e. dependencies first.
type Graph map[string]*Node

// Node is a node in [Graph], representing a single task.
type Node struct {
	ID           string   // ID of the task in question
	Type         string   // type of the task in question
	Dependencies []string // dependencies of the task in question
	Dependents   []*Node  // nodes for the tasks that are dependent on this task
	DegreeIn     int      // number of incoming edges
}

// visitState is the type for the visit indicator during the cycle detection in
// Graph.
type visitState int

// NewGraph returns a new Graph built from the given task configuration.
func NewGraph(cfgs []Config) (Graph, error) {
	graph := make(Graph)

	for _, cfg := range cfgs {
		if cfg.ID == "" {
			// TODO: Automatically add the missing tasks if a dependency is just
			// a task type. This should be done earlier and not here, but this
			// warrants a panic once this is implemented.
			// panic(fmt.Sprintf("%v: task of type %s", errNilID, cfg.Type))
			return nil, fmt.Errorf("%w: task of type %s", errNilID, cfg.Type)
		}

		graph[cfg.ID] = &Node{
			ID:           cfg.ID,
			Type:         cfg.Type,
			Dependencies: cfg.Dependencies, // dependencies should be normalized before this
			Dependents:   make([]*Node, 0),
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
		return nil, fmt.Errorf("%w", err)
	}

	return graph, nil
}

// Sorted returns that graph as a topologically sorted list of stages for
// running. Each element of the slice is a slice that contains the tasks that
// can be executed in parallel.
func (g Graph) Sorted() ([][]*Node, error) {
	queue := make([]*Node, 0)

	for _, node := range g {
		if node.DegreeIn == 0 {
			queue = append(queue, node)
		}
	}

	var ( //nolint:prealloc // no need to preallocate here
		stages [][]*Node
		sorted []*Node
	)

	for len(queue) > 0 {
		current := make([]*Node, len(queue))

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
		return nil, fmt.Errorf("%w", errCycle)
	}

	return stages, nil
}

func (g Graph) checkCycles() error {
	state := make(map[string]visitState, len(g))

	for id := range g {
		state[id] = unvisited
	}

	for id, node := range g {
		if state[id] == unvisited {
			stack := make([]*Node, 0)
			if err := visit(node, state, &stack); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
	}

	return nil
}

func visit(node *Node, state map[string]visitState, stack *[]*Node) error {
	state[node.ID] = visiting

	*stack = append(*stack, node)

	for _, dependent := range node.Dependents {
		switch state[dependent.ID] {
		case unvisited:
			if err := visit(dependent, state, stack); err != nil {
				return fmt.Errorf("%w", err)
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

// newCycleError formats and returns an error for circular dependencies.
func newCycleError(startNode *Node, stack []*Node) error {
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
