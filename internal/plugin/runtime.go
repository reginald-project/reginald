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

import "github.com/reginald-project/reginald/internal/fspath"

// A runtime is a plugin runtime. The implementations for the runtimes are
// provided by the [runtimes] package.
type runtime interface {
	// Executable is the resolved executable path of this runtime on the system.
	Executable() fspath.Path

	// Name returns the name of this runtime.
	Name() string

	// Present reports whether this runtime is present on the system.
	Present() bool
}
