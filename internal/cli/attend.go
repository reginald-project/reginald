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

package cli

import (
	"context"

	"github.com/anttikivi/reginald/internal/logging"
)

// NewAttend returns a new apply command.
func NewAttend() *Command {
	c := &Command{ //nolint:exhaustruct // private fields need zero values
		Name: "attend",
		Aliases: []string{
			"apply",
			"tend",
		},
		UsageLine: "attend [options]",
		Setup:     setupAttend,
		Run:       runAttend,
	}

	return c
}

func setupAttend(ctx context.Context, _ *Command, _ []string) error {
	logging.InfoContext(ctx, "setting up attend")

	return nil
}

func runAttend(ctx context.Context, _ *Command) error {
	logging.InfoContext(ctx, "running attend")

	return nil
}
