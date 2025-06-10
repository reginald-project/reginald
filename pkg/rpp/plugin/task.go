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

package plugin

import "github.com/anttikivi/reginald/pkg/rpp"

// Task is a task that Reginald can run. The task implementation is resolved by
// the applying commands from either Reginald itself or plugins.
type Task interface {
	// Type returns the name of the task type as it should be written by
	// the user when they specify it in, for example, their configuration. It
	// must not match any existing tasks either within Reginald or other
	// plugins.
	Type() string

	// Validate runs the validation of the config for the matching task type.
	// Only the actual task options are checked as the ID, type, and
	// dependencies are validated by Reginald.
	Validate(cfg []rpp.KeyValue) error

	// Run runs this task. In the function call, dir will be the base directory
	// of the Reginald run and cfg contains the configuration options for this
	// task.
	Run(dir string, cfg []rpp.KeyValue) error
}
