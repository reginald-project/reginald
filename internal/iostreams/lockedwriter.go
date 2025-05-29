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
