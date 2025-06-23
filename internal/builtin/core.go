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

// coreManifest returns the manifest for the core plugin.
func coreManifest() *api.Manifest {
	return &api.Manifest{
		Name:    "builtin",
		Version: version.Version().String(),
		Domain:  "core",
		// TODO : Add a description.
		Description: "TODO",
		Help:        "",
		Executable:  "",
		Config:      nil,
		Commands: []*api.Command{
			{
				Name:  "attend",
				Usage: "attend",
				// TODO : Add a description.
				Description: "TODO",
				Help:        "TODO",
				Manual:      "TODO",
				Aliases:     []string{"apply", "tend"},
				Config:      nil,
				Commands:    nil,
			},
			{
				Name:  "version",
				Usage: "version",
				// TODO : Add a description.
				Description: "TODO",
				Help:        "TODO",
				Manual:      "TODO",
				Aliases:     nil,
				Config:      nil,
				Commands:    nil,
			},
		},
		Tasks: nil,
	}
}
