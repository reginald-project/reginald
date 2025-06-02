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

package iostreams

import (
	"fmt"
	"io"
	"sync"
)

// StdioMu is the mutual exclusion lock used globally to lock the standard input
// and output streams where necessary to prevent multiple sources reading or
// writing at the same time. It is usually used with the [io.Writer] acquired by
// [NewLockedWriter].
var StdioMu sync.Mutex //nolint:gochecknoglobals // used by multiple goroutines

type lockedWriter struct {
	w io.Writer
}

// NewLockedWriter creates a new locked writer that guarantees sequential
// writing to the given writer. It uses the global [StdioMu] for locking.
func NewLockedWriter(w io.Writer) io.Writer {
	return &lockedWriter{w: w}
}

// Write acquires a global writing lock and writes len(p) bytes from p to the
// underlying data stream. It returns the number of bytes written from p
// (0 <= n <= len(p)) and any error encountered that caused the write to stop
// early.
func (w *lockedWriter) Write(p []byte) (int, error) {
	StdioMu.Lock()
	defer StdioMu.Unlock()

	n, err := w.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("%w", err)
	}

	return n, nil
}
