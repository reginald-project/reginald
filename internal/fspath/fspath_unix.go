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

//go:build !windows

package fspath

import "os"

// expandOSEnv replaces ${var} or $var in the string according to the values of
// the current environment variables. References to undefined variables are
// replaced by empty string.
func expandOSEnv(path Path) Path {
	return Path(os.ExpandEnv(string(path)))
}
