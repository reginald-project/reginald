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

package logging

// logCallerDepth is the depth of the stack trace to skip when logging.
// The skipped stack is [runtime.Callers, the function, the function's caller].
const logCallerDepth = 3

// IgnorePC controls whether to invoke runtime.Callers to get the pc in
// the logging functions. This is solely for making the logging function
// analogous with the logging functions in the standard library.
var ignorePC = false //nolint:gochecknoglobals // can be set at compile time
