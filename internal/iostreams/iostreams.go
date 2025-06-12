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
	"os"
	"sync"

	"golang.org/x/term"
)

// Message output destinations.
const (
	Buffered Output = iota
	Stdout
	Stderr
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

// errInvalidOutput is appended to the stream errors when a message has
// an invalid output value.
var errInvalidOutput = errors.New("invalid message output")

// Output is the property of a message that tells its Output destination.
type Output int

// IOStreams is the type of the global input and output object. By default, it
// locks with the global standard input and output streams' mutual exclusion
// lock before writing. If the reading or writing operations using this type
// return an error, it will be stored within the struct.
type IOStreams struct {
	out           chan message
	flush         chan chan struct{}
	wg            sync.WaitGroup
	errs          []error
	errsMu        sync.Mutex
	quiet         bool
	verbose       bool //nolint:unused // TODO: Will be used soon.
	colorsEnabled bool
}

// A StreamWriter is an [io.Writer] created from an instance of [IOStreams] that
// can be used to write to the same output channel.
type StreamWriter struct {
	s      *IOStreams
	output Output
}

// code is the type for the ANSI color codes.
type code int

type message struct {
	msg    string
	output Output
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

	s := &IOStreams{
		out:           make(chan message),
		flush:         make(chan chan struct{}),
		wg:            sync.WaitGroup{},
		errs:          nil,
		errsMu:        sync.Mutex{},
		quiet:         quiet,
		verbose:       verbose,
		colorsEnabled: colorsEnabled,
	}

	s.wg.Add(1)
	go s.output()

	return s
}

// NewWriter creates a new StreamWriter. It panics on errors.
func NewWriter(s *IOStreams, output Output) *StreamWriter {
	if s == nil {
		panic("attempt to create StreamWriter with nil IOStreams")
	}

	return &StreamWriter{
		s:      s,
		output: output,
	}
}

func (s *IOStreams) Close() {
	s.Flush()
	close(s.out)
	s.wg.Wait()
}

// Err returns the errors that s has encountered. [errors.Join] is called on the
// errors before returning them.
func (s *IOStreams) Err() error {
	s.errsMu.Lock()
	defer s.errsMu.Unlock()

	return errors.Join(s.errs...)
}

// Flush flushes the underlying buffer.
func (s *IOStreams) Flush() {
	ack := make(chan struct{})
	s.flush <- ack
	<-ack
}

// Errorf formats according to a format specifier and writes to standard error
// output of s. If colors are enabled, the message is printed in red. It stores
// possible errors within s.
func (s *IOStreams) Errorf(format string, a ...any) {
	s.out <- message{
		msg:    s.colorf(red, format, a...),
		output: Stderr,
	}
}

// Print formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are added between operands when neither
// is a string. It stores possible errors within s.
func (s *IOStreams) Print(a ...any) {
	if s.quiet {
		return
	}

	s.out <- message{
		msg:    fmt.Sprint(a...),
		output: Buffered,
	}
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of s. It stores possible errors within s.
func (s *IOStreams) PrintErrf(format string, a ...any) {
	s.out <- message{
		msg:    fmt.Sprintf(format, a...),
		output: Stderr,
	}
}

// Printf formats according to a format specifier and writes to standard output
// buffer of s. It stores possible errors within s.
func (s *IOStreams) Printf(format string, a ...any) {
	if s.quiet {
		return
	}

	s.out <- message{
		msg:    fmt.Sprintf(format, a...),
		output: Buffered,
	}
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are always added between operands and
// a newline is appended. It stores possible errors within s.
func (s *IOStreams) Println(a ...any) {
	if s.quiet {
		return
	}

	s.out <- message{
		msg:    fmt.Sprintln(a...),
		output: Buffered,
	}
}

// Write writes the contents of p into the output channel. It returns the number
// of bytes written.
func (w *StreamWriter) Write(p []byte) (int, error) {
	w.s.out <- message{
		msg:    string(p),
		output: w.output,
	}

	return len(p), nil
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

func (s *IOStreams) appendErr(err error) {
	s.errsMu.Lock()
	defer s.errsMu.Unlock()
	s.errs = append(s.errs, err)
}

func (s *IOStreams) colorf(c code, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)

	if !s.colorsEnabled {
		return msg
	}

	return fmt.Sprintf("%s[%dm%s%s[%dm", escape, c, msg, escape, reset)
}

func (s *IOStreams) output() {
	defer s.wg.Done()

	buf := bufio.NewWriter(os.Stdout)

	flush := func() {
		if err := buf.Flush(); err != nil {
			s.appendErr(err)
		}
	}

	for {
		select {
		case msg, ok := <-s.out:
			if !ok {
				if err := buf.Flush(); err != nil {
					s.appendErr(err)
				}

				return
			}

			var err error

			switch msg.output {
			case Buffered:
				_, err = buf.WriteString(msg.msg)
			case Stdout:
				flush()
				_, err = fmt.Fprint(os.Stdout, msg.msg)
			case Stderr:
				flush()
				_, err = fmt.Fprint(os.Stderr, msg.msg)
			default:
				// TODO: Maybe the program should panic here.
				err = fmt.Errorf("%w: %v", errInvalidOutput, msg.output)
			}

			if err != nil {
				s.appendErr(err)
			}
		case ack := <-s.flush:
			flush()
			close(ack)
		}
	}
}
