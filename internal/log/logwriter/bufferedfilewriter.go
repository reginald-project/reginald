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

// Package logwriter defines and holds the writer for the bootstrap logger. It
// is a separate package to avoid import cycles.
package logwriter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/reginald-project/reginald/internal/fspath"
)

// Default values for the BufferedFileWriter.
const (
	defaultBufFileWriterPerm    = 0o600
	defaultBufFileWriterDirPerm = 0o750
)

// BootstrapWriter is the writer used by the bootstrap logger. It is global so
// that in case of errors the final handler of the error can check if its type
// is [BufferedFileWriter] and flush its contents to the given file if that is
// the case.
var BootstrapWriter io.Writer //nolint:gochecknoglobals // needed by the panic handler

// A BufferedFileWriter is a writer that is used by the bootstrap logger to
// write the logging information to a buffer if the logs are not enabled by
// setting `REGINALD_DEBUG=1`. If it is used, it flushes it contents to a file
// only the program exits with an error. Otherwise, the logs are discarded at
// the end of the program.
type BufferedFileWriter struct {
	buf  *bytes.Buffer
	file fspath.Path
	mu   sync.Mutex
}

// NewBufferedFileWriter returns a new bootstrap writer for the given file.
// The given file must be a valid and absolute path. If it does not exist when
// the writer flushes, the file will be created.
func NewBufferedFileWriter(file fspath.Path) *BufferedFileWriter {
	if !file.IsAbs() {
		panic("tried to set an invalid path to bootstrap writer")
	}

	return &BufferedFileWriter{
		buf:  &bytes.Buffer{},
		mu:   sync.Mutex{},
		file: file,
	}
}

// Bytes returns a copy of the bytes currently in the buffer.
func (w *BufferedFileWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()

	src := w.buf.Bytes()
	dst := make([]byte, len(src))

	copy(dst, src)

	return dst
}

// File returns path to the file where the logs are written.
func (w *BufferedFileWriter) File() fspath.Path {
	return w.file
}

// Flush writes the underlying buffer to the given file, creating the file and
// the intermediate directories if they don't exist.
func (w *BufferedFileWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := w.file.Dir()
	if err := dir.MkdirAll(defaultBufFileWriterDirPerm); err != nil {
		return fmt.Errorf("%w", err)
	}

	f, err := w.file.OpenFile(os.O_CREATE|os.O_WRONLY|os.O_APPEND, defaultBufFileWriterPerm)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			// TODO: See if there is some better way to handle this error.
			fmt.Fprintf(os.Stderr, "failed to close buffered file writer file: %v\n", err)
		}
	}()

	if _, err := f.Write(w.buf.Bytes()); err != nil {
		return fmt.Errorf("%w", err)
	}

	w.buf.Reset()

	return nil
}

// Write writes the contents of p into the buffer. It returns the number of
// bytes written.
func (w *BufferedFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("%w", err)
	}

	return n, nil
}
