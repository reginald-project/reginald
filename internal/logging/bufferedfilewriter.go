package logging

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// A BufferedFileWriter is a writer that is used by the bootstrap logger to
// write the logging information to a buffer if the logs are not enabled by
// setting `REGINALD_DEBUG=1`. If is is used, it flushes it contents to a file
// only the program exits with an error. Otherwise, the logs are discarded at
// the end of the program.
type BufferedFileWriter struct {
	buf  *bytes.Buffer
	mu   sync.Mutex
	file string
}

// NewBufferedFileWriter returns a new bootstrap writer for the given file.
// The given file must be a valid and absolute path. If it does not exist when
// the writer flushes, the file will be created.
func NewBufferedFileWriter(file string) *BufferedFileWriter {
	if !filepath.IsAbs(file) {
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
func (w *BufferedFileWriter) File() string {
	return w.file
}

// Flush writes the underlying buffer to the given file, creating the file and
// the intermediate directories if they don't exist.
func (w *BufferedFileWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir := filepath.Dir(w.file)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w", err)
	}

	f, err := os.OpenFile(w.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	defer f.Close()

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
