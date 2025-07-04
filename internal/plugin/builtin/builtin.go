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

// Package builtin provides the definitions for the built-in plugins of
// Reginald. It includes the manifests for the plugins that provide the commands
// and tasks that are included with Reginald. It also defines the built-in
// plugins' service options as it fits OK here and otherwise it would cause
// an import cycle.
package builtin

import (
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/plugin"
)

// Manifests returns the plugin manifests for the built-in plugins.
func Manifests() []*api.Manifest {
	return []*api.Manifest{coreManifest(), linkManifest()}
}

// Service returns the service function for the given built-in plugin name.
func Service(pluginName string) plugin.Service {
	switch pluginName {
	case coreManifest().Name:
		return coreService
	case linkManifest().Name:
		return linkService
	default:
		panic("invalid built-in plugin name: " + pluginName)
	}
}
