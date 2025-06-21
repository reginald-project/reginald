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

package cli

import (
	"errors"
	"strings"

	"github.com/reginald-project/reginald/internal/plugin"
)

// An ExitError is an error returned by the CLI that wraps an error that is
// causing the program to exit and associates an exit code with it. The program
// will return the exit code once it ends its execution.
type ExitError struct {
	err error

	// Code is the exit code associated with this error. It will be used by
	// the program as the exit code it returns to the caller.
	Code int
}

// SuccessError is an error that is returned by the CLI when the program is
// successfully executed but voluntarily exits early. It used, for example, if
// the user opts into exiting in interactive mode.
//
// The name might be confusing, but let it go.
type SuccessError struct{}

// strictError is an error that is returned by the CLI when the program is
// executed in strict mode and the config file or the plugins directory is not
// found.

// strictError is used to store errors that are returned during the bootstrap
// process that cause the program to exit only if the strict mode is enabled.
type strictError struct {
	errs []error
}

// Error returns the value of the error as a string. This function implements
// the error interface for Success.
func (*SuccessError) Error() string {
	return "success"
}

// Error returns the value of e as a string.
func (e *ExitError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e *ExitError) Unwrap() error {
	return e.err
}

// Error returns the value of e as a string.
func (e *strictError) Error() string {
	if len(e.errs) == 1 {
		return e.errs[0].Error()
	}

	s := ""

	for _, err := range e.errs {
		var pathErrs plugin.PathErrors
		if errors.As(err, &pathErrs) && len(pathErrs) > 1 {
			s += "\n  - plugin search paths not found: " + strings.Join(pathErrs.Paths(), "; ")

			continue
		}

		s += "\n  - " + err.Error()
	}

	return s
}

// Unwrap returns the wrapped errors from s.
func (e *strictError) Unwrap() []error {
	return e.errs
}
