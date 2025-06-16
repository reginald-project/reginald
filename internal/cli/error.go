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

package cli

// An ExitError is an error returned by the CLI that wraps an error that is
// causing the program to exit and associates an exit code with it. The program
// will return the exit code once it ends its execution.
type ExitError struct {
	// Code is the exit code associated with this error. It will be used by
	// the program as the exit code it returns to the caller.
	Code int
	err  error
}

// Error returns the value of e as a string. This function implements the error
// interface for ExitError.
func (e *ExitError) Error() string {
	return e.err.Error()
}
