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

// Package task offers utilities related to tasks in Reginald. These are shared
// between Reginald implementations and the plugin utilities.
package task

// A Config is the configuration of a task.
type Config struct {
	// Options contains the rest of the config options for the task.
	Options map[string]any `mapstructure:",remain"` //nolint:tagliatelle // linter doesn't know about "remain"

	// Type is the type of this task. It defines which task implementation is
	// called when this task is executed.
	Type string `mapstructure:"type"`

	// ID is the unique ID for this task. It must be unique.
	ID string `mapstructure:"id,omitempty"`
}
