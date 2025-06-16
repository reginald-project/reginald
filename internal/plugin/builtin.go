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

import "github.com/reginald-project/reginald-sdk-go/api"

// A Builtin is a built-in plugin provided by Reginald. It is implemented within
// the program and it must not use an external executable.
type Builtin struct {
	// manifest is the manifest for this plugin.
	manifest api.Manifest
}

// Manifest returns the loaded manifest for the plugin.
func (b *Builtin) Manifest() api.Manifest {
	return b.manifest
}

// newBuiltin returns a new built-in plugin for the given manifest.
func newBuiltin(m api.Manifest) *Builtin {
	return &Builtin{
		manifest: m,
	}
}
