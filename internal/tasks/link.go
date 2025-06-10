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

package tasks

import (
	"context"
	"fmt"

	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/internal/taskcfg"
)

// NewLink returns a new instance of the link task.
func NewLink() *Task {
	return &Task{
		Validate: validateLink,
		Run:      runLink,
		Type:     "link",
	}
}

func validateLink(_ context.Context, _ *Task, opts taskcfg.Options) error {
	for k, v := range opts {
		switch k {
		case "create", "create-dirs", "force":
			if !opts.IsBool(k) {
				return fmt.Errorf("%w: %q is not a bool (%T)", config.ErrInvalidConfig, k, v)
			}
		default:
			return fmt.Errorf("%w: %q (value %v)", config.ErrInvalidConfig, k, v)
		}
	}

	return nil
}

func runLink(_ context.Context, _ *Task, _ fspath.Path, _ taskcfg.Options) error {
	return nil
}
