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

import (
	"errors"

	"github.com/reginald-project/reginald/internal/fspath"
)

// Errors returned when a plugin is invalid.
var (
	ErrInvalidConfig   = errors.New("invalid plugin config")
	errHandshake       = errors.New("plugin provided incompatible response")
	errInvalidCast     = errors.New("cannot convert type")
	errInvalidResponse = errors.New("invalid response")
	errInvalidLength   = errors.New("number of bytes read does not match")
	errInvalidManifest = errors.New("invalid plugin manifest")
	errNoResponse      = errors.New("no response")
	errUnknownMethod   = errors.New("unknown method")
	errZeroLength      = errors.New("Content-Length is zero")
)

// A PathError is returned when a plugin search path is not found.
type PathError struct {
	Path fspath.Path
}

// PathErrors is a slice of PathError that collects all of the failed plugin
// search paths. It may only contain PathErrors.
type PathErrors []error

// Error returns the value of e as a string.
func (e *PathError) Error() string {
	if e.Path == "" {
		return "plugin search path not found"
	}

	return "plugin search path not found: " + string(e.Path)
}

// Error returns the value of e as a string.
func (e PathErrors) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}

	s := "plugin search paths not found"

	if len(e) == 0 {
		return s
	}

	s += ":"

	for _, err := range e {
		var pathErr *PathError
		if !errors.As(err, &pathErr) {
			panic("PathErrors contains an error that is not a PathError")
		}

		s += "\n  - " + string(pathErr.Path)
	}

	return s
}

// Paths returns the paths that caused the error as strings.
func (e PathErrors) Paths() []string {
	paths := make([]string, len(e))

	for i, err := range e {
		var pathErr *PathError
		if !errors.As(err, &pathErr) {
			panic("PathErrors contains an error that is not a PathError")
		}

		paths[i] = string(pathErr.Path)
	}

	return paths
}
