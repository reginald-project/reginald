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
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/version"
)

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
		Name:    "reginald-link",
		Version: version.Version().String(),
		Domain:  "link",
		// TODO : Add a description.
		Description: "TODO",
		Help:        "",
		Executable:  "",
		Config:      nil,
		Commands:    nil,
		Tasks: []api.Task{
			{
				Type:        "create",
				Description: "TODO",
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
									{
										KeyVal: api.KeyVal{
											Value: api.Value{
												Val:  false,
												Type: "bool",
											},
											Key: "contents",
										},
										Description: "If the resolved `src` file is a directory, create the links for each file in the directory into the destination directory instead of creating a link of the directory.",
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
