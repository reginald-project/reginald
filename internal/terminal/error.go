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

package terminal

import (
	"errors"
	"sync"
)

// An asyncError is the error type for [Terminal]. It stores the asynchronous
// that happen during the Terminal's execution in a stack. asyncError is
// thread-safe.
type asyncError struct {
	errs []error
	mu   sync.Mutex
}

// Error returns the value of e as a string.
func (e *asyncError) Error() string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.errs) == 0 {
		return ""
	}

	return errors.Join(e.errs...).Error()
}

// Unwrap returns the wrapped errors from e.
func (e *asyncError) Unwrap() []error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.errs
}

func (e *asyncError) append(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.errs = append(e.errs, err)
}

func (e *asyncError) joined() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.errs) == 0 {
		return nil
	}

	return errors.Join(e.errs...)
}
