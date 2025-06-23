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

// A Writer is an [io.Writer] created from an instance of [Terminal] that can be
// used to write to the output channels of the [Terminal].
type Writer struct {
	s    *Terminal
	mode OutputMode
}

// NewWriter creates a new Writer for the given [Terminal]. It panics on errors.
func NewWriter(s *Terminal, mode OutputMode) *Writer {
	if s == nil {
		panic("attempt to create Writer with nil Terminal")
	}

	return &Writer{
		s:    s,
		mode: mode,
	}
}

// Write writes the contents of p into the output channel. It returns the number
// of bytes written.
func (w *Writer) Write(p []byte) (int, error) { //nolint:unparam // implements interface
	w.s.outCh <- message{
		msg:  string(p),
		mode: w.mode,
	}

	return len(p), nil
}
