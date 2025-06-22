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

// A StreamsWriter is an [io.Writer] created from an instance of [IO] that
// can be used to write to the same output channel.
type StreamsWriter struct {
	s      *IO
	output Output
}

// NewWriter creates a new StreamWriter. It panics on errors.
func NewWriter(s *IO, output Output) *StreamsWriter {
	if s == nil {
		panic("attempt to create StreamWriter with nil IOStreams")
	}

	return &StreamsWriter{
		s:      s,
		output: output,
	}
}

// Write writes the contents of p into the output channel. It returns the number
// of bytes written.
func (w *StreamsWriter) Write(p []byte) (int, error) { //nolint:unparam // implements interface
	w.s.outCh <- message{
		msg:    string(p),
		output: w.output,
	}

	return len(p), nil
}
