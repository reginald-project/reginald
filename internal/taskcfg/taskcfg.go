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

// Package taskcfg provides the configuration types for the tasks.
package taskcfg

// Defaults is the type for the default config values set for the tasks.
type Defaults map[string]any

// Options is the type for the config options in a task config entry.
type Options map[string]any

// A Config is the configuration of a task.
type Config struct {
	// Type is the type of this task. It defines which task implementation is
	// called when this task is executed.
	Type string `mapstructure:"type"`

	// ID is the unique ID for this task. It must be unique. The ID must also be
	// different from the provided task types.
	ID string `mapstructure:"id,omitempty"`

	// Options contains the rest of the config options for the task.
	Options Options `mapstructure:",remain"` //nolint:tagliatelle // linter doesn't know about "remain"

	// Dependencies are the task IDs or types that this task depends on.
	Dependencies []string `mapstructure:"dependencies"`
}

// IsBool reports whether o has an entry with the given key that is a bool.
func (o Options) IsBool(key string) bool {
	v, ok := o[key]
	if !ok {
		return false
	}

	if _, ok := v.(bool); !ok {
		return false
	}

	return true
}
