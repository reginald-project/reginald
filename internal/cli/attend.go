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
	"fmt"

	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/tasks"
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

func setupAttend(ctx context.Context, cmd *Command, args []string) error {
	logging.InfoContext(ctx, "setting up attend")

	if len(args) > 0 {
		return fmt.Errorf("%w: %q", ErrUnknownArg, args[0])
	}

	taskTypes, err := cmd.cli.TaskTypes()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.DebugContext(ctx, "loaded task types", "tasks", taskTypes)

	cfg := cmd.cli.Cfg

	cfg.Tasks, err = tasks.Configure(ctx, cfg.Tasks, cfg.Defaults, taskTypes)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.DebugContext(ctx, "config valid", "cfg", cfg)

	return nil
}

func runAttend(ctx context.Context, cmd *Command) error {
	logging.InfoContext(ctx, "running attend")

	taskTypes, err := cmd.cli.TaskTypes()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.DebugContext(ctx, "loaded task types", "tasks", taskTypes)

	cfg := cmd.cli.Cfg

	if err := tasks.Run(ctx, cfg, taskTypes); err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.DebugContext(ctx, "attend ran")

	return nil
}
