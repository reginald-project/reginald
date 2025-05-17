// Package iostreams defines the IO stream utilities for the Reginald terminal
// user interface. Most importantly, it defines the global instance that should
// be used for output in the program.
package iostreams

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
)

// IOStreams is the type of the global input and output object.
type IOStreams struct {
	out           io.Writer
	errOut        io.Writer
	mu            sync.Mutex
	buf           *bufio.Writer
	quiet         bool
	verbose       bool
	colorsEnabled bool
}

// Streams is the global IO streams instance for the program. It must be
// initialized before use.
var Streams *IOStreams //nolint:gochecknoglobals

// New returns a new IOStreams for the given settings.
func New(quiet, verbose, colors bool) *IOStreams {
	return &IOStreams{
		out:           os.Stdout,
		errOut:        os.Stderr,
		mu:            sync.Mutex{},
		buf:           bufio.NewWriter(os.Stdout),
		quiet:         quiet,
		verbose:       verbose,
		colorsEnabled: colors,
	}
}

// Flush flushes the underlying buffer.
func (s *IOStreams) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.buf.Flush(); err != nil {
		return fmt.Errorf("failed to flush the output buffer: %w", err)
	}

	return nil
}

// Printf formats according to a format specifier and writes to standard output
// buffer of s. It suppresses possible errors.
func (s *IOStreams) Printf(format string, a ...any) {
	if s.quiet {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = fmt.Fprintf(s.buf, format, a...)
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are always added between operands and a
// newline is appended. It suppresses possible errors.
func (s *IOStreams) Println(a ...any) {
	if s.quiet {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = fmt.Fprintln(s.buf, a...)
}
