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
	"fmt"
	"log/slog"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/version"
)

const linkName = "reginald-link"

// linkManifest returns the manifest for the link plugin.
func linkManifest() *api.Manifest {
	//nolint:lll
	force := api.ConfigValue{
		KeyVal: api.KeyVal{
			Value: api.Value{
				Val:  false,
				Type: api.BoolValue,
			},
			Key: "force",
		},
		Description: "If enabled, any existing file that has the same name as the link that is created will be removed.",
	}

	return &api.Manifest{
		Name:        linkName,
		Version:     version.Version().String(),
		Domain:      "link",
		Description: "The \"reginald-link\" plugin contains the tasks for creating links with Reginald.",
		Help:        "",
		Executable:  "",
		Runtime:     nil,
		Config:      nil,
		Commands:    nil,
		Tasks: []api.Task{
			{
				TaskType:    "create",
				Description: "Create links.",
				Provides:    "",
				RawConfig:   nil,
				Config: []api.ConfigType{
					force,
					api.UnionValue{
						Alternatives: []api.ConfigType{
							api.ConfigValue{
								KeyVal: api.KeyVal{
									Value: api.Value{
										Val:  []string{},
										Type: api.PathListValue,
									},
									Key: "links",
								},
								//nolint:lll
								Description: "List of symbolic links to create where the key is link file to create. The file that the link points to is created from the path of the link as described in the task's documentation.",
							},
							//nolint:lll
							api.MappedValue{
								Key:         "links",
								KeyType:     api.PathValue,
								Description: "List of symbolic links to create where the key is link file to create. If no `src` is given, the file that the link points to is created from the path of the link as described in the task's documentation.",
								Values: []api.ConfigValue{
									force,
									{
										KeyVal: api.KeyVal{
											Value: api.Value{
												Val:  "",
												Type: "path",
											},
											Key: "src",
										},
										Description: "The file that the created link points to. If omitted, it will be resolved from the path given as the key for this table entry as described in the task's documentation.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// linkService is the service function for the "reginald-link" plugin.
func linkService(ctx context.Context, _ *plugin.Store, method string, _ any) error {
	switch method {
	case api.MethodRunTask:
		slog.InfoContext(ctx, "running task")

		return nil
	default:
		panic(fmt.Sprintf("invalid method call to %q: %s", linkName, method))
	}
}
