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

package plugin

import "github.com/reginald-project/reginald-sdk-go/api"

// An External is an external plugin that is not provided by the program itself.
// It implements the plugin client in Reginald for calling methods from
// the plugin executables.
type External struct {
	// manifest is the manifest for this plugin.
	manifest *api.Manifest

	// loaded tells whether the executable for this plugin is loaded and started
	// up.
	loaded bool //nolint:unused // TODO: Will be used soon.
}

// Manifest returns the loaded manifest for the plugin.
func (e *External) Manifest() *api.Manifest {
	return e.manifest
}

// newExternal returns a new external plugin for the given manifest.
func newExternal(m *api.Manifest) *External {
	return &External{
		manifest: m,
		loaded:   false,
	}
}
