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

// Package iostreams defines the IO stream utilities for the Reginald terminal
// user interface. Most importantly, it defines the global instance that should
// be used for output in the program.
package iostreams

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// escape is the escape character for the escape sequences.
const escape = "\x1b"

// Basic attribute ANSI codes.
const (
	reset code = iota
)

// Foreground text color codes.
const (
	black code = iota + 30
	red
)

// Streams is the global IO streams instance for the program. It must be
// initialized before use.
var Streams *IOStreams //nolint:gochecknoglobals // global IO instance

// code is the type for the ANSI color codes.
type code int

// IOStreams is the type of the global input and output object. By default, it
// locks with the global standard input and output streams' mutual exclusion
// lock before writing. If the reading or writing operations using this type
// return an error, it will be stored within the struct.
type IOStreams struct {
	out           io.Writer
	errOut        io.Writer
	buf           *bufio.Writer
	errs          []error
	quiet         bool
	verbose       bool //nolint:unused // TODO: Will be used soon.
	colorsEnabled bool
}

// New returns a new IOStreams for the given settings.
func New(quiet, verbose bool, colors ColorMode) *IOStreams {
	var colorsEnabled bool

	switch colors {
	case ColorAlways:
		colorsEnabled = true
	case ColorNever:
		colorsEnabled = false
	case ColorAuto:
		colorsEnabled = term.IsTerminal(int(os.Stdout.Fd()))
	default:
		panic(fmt.Sprintf("invalid IOStreams color mode: %v", colors))
	}

	s := &IOStreams{ //nolint:exhaustruct // buf is set later
		errs:          nil,
		out:           NewLockedWriter(os.Stdout),
		errOut:        NewLockedWriter(os.Stderr),
		quiet:         quiet,
		verbose:       verbose,
		colorsEnabled: colorsEnabled,
	}

	s.buf = bufio.NewWriter(s.out)

	return s
}

// Err returns the errors that s has encountered. [errors.Join] is called on the
// errors before returning them.
func (s *IOStreams) Err() error {
	return errors.Join(s.errs...)
}

// Flush flushes the underlying buffer.
func (s *IOStreams) Flush() error {
	if err := s.buf.Flush(); err != nil {
		return fmt.Errorf("failed to flush the output buffer: %w", err)
	}

	return nil
}

// Errorf formats according to a format specifier and writes to standard error
// output of s. If colors are enabled, the message is printed in red. It stores
// possible errors within s.
func (s *IOStreams) Errorf(format string, a ...any) {
	if _, err := s.errOut.Write([]byte(s.colorf(red, format, a...))); err != nil {
		s.errs = append(s.errs, err)
	}
}

// BufPrint formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are added between operands when neither
// is a string. It stores possible errors within s.
func (s *IOStreams) BufPrint(a ...any) {
	if s.quiet {
		return
	}

	if _, err := fmt.Fprint(s.buf, a...); err != nil {
		s.errs = append(s.errs, err)
	}
}

// BufPrintf formats according to a format specifier and writes to standard
// output buffer of s. It stores possible errors within s.
func (s *IOStreams) BufPrintf(format string, a ...any) {
	if s.quiet {
		return
	}

	if _, err := fmt.Fprintf(s.buf, format, a...); err != nil {
		s.errs = append(s.errs, err)
	}
}

// BufPrintln formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are always added between operands and a
// newline is appended. It stores possible errors within s.
func (s *IOStreams) BufPrintln(a ...any) {
	if s.quiet {
		return
	}

	if _, err := fmt.Fprintln(s.buf, a...); err != nil {
		s.errs = append(s.errs, err)
	}
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of s. It stores possible errors within s.
func (s *IOStreams) PrintErrf(format string, a ...any) {
	if _, err := fmt.Fprintf(s.errOut, format, a...); err != nil {
		s.errs = append(s.errs, err)
	}
}

// Errorf formats according to a format specifier and writes to standard error
// output of [Streams]. If colors are enabled, the message is printed in red. It
// stores possible errors within [Streams].
func Errorf(format string, a ...any) {
	if Streams == nil {
		panic("tried to call nil Streams")
	}

	Streams.Errorf(format, a...)
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of [Streams]. It stores possible errors within [Streams].
func PrintErrf(format string, a ...any) {
	if Streams == nil {
		panic("tried to call nil Streams")
	}

	Streams.PrintErrf(format, a...)
}

func (s *IOStreams) colorf(c code, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)

	if !s.colorsEnabled {
		return msg
	}

	return fmt.Sprintf("%s[%dm%s%s[%dm", escape, c, msg, escape, reset)
}
