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

package builtin

import (
	"context"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/version"
)

// coreManifest returns the manifest for the core plugin.
func coreManifest() *api.Manifest {
	return &api.Manifest{
		Name:    "reginald-core",
		Version: version.Version().String(),
		Domain:  "core",
		//nolint:lll
		Description: "The \"reginald-core\" plugin contains the core command for Reginald. These commands are used for running the basic operations of Reginald, like executing the tasks.",
		Help:        "",
		Executable:  "",
		Runtime:     nil,
		Config:      nil,
		Commands: []*api.Command{
			{
				Name:        "attend",
				Usage:       "attend",
				Description: "Execute the tasks.",
				//nolint:lll
				Help:     "Executes the tasks defined in the Reginald config file. The order of the tasks is not guaranteed; `attend` may run the tasks in parallel and in any order. However, tasks depending on other tasks are executed after the tasks they depend on. Task dependencies are declared in the `requires` field using the task IDs.",
				Manual:   "TODO",
				Aliases:  []string{"apply", "tend"},
				Config:   nil,
				Commands: nil,
				Args:     nil,
			},
			{
				Name:  "version",
				Usage: "version",
				// TODO : Add a description.
				Description: "Print version.",
				Help:        "Prints the version information to the standard output and exits.",
				Manual:      "",
				Aliases:     nil,
				Config:      nil,
				Commands:    nil,
				Args:        nil,
			},
		},
		Tasks: nil,
	}
}

// coreService is the service function for the "reginald-core" plugin.
func coreService(_ context.Context, _ plugin.ServiceInfo, _ string, _ any) error {
	return nil
}
