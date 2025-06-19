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

// Package terminal defines the terminal and IO utilities for the Reginald
// terminal user interface. Most importantly, it defines the global instance
// that should be used for output in the program.
package terminal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
	green
	yellow
	blue
	magenta
	cyan
	white
)

// ErrQuietPrompt is returned when a prompt is requested from the user in quiet
// mode.
var ErrQuietPrompt = errors.New("cannot prompt for input in quiet mode")

// streams is the global IO streams instance for the program. It must be
// initialized before use.
var streams *IO //nolint:gochecknoglobals // global IO instance

// errInvalidOutput is appended to the stream errors when a message has
// an invalid output value.
var errInvalidOutput = errors.New("invalid message output")

// Output is the property of a message that tells its Output destination.
type Output int

// IO is the type of the global input and output object. By default, it
// locks with the global standard input and output streams' mutual exclusion
// lock before writing. If the reading or writing operations using this type
// return an error, it will be stored within the struct.
type IO struct {
	promptCh      chan promptRequest
	outCh         chan message
	flushCh       chan chan struct{}
	in            io.Reader
	out           io.Writer
	errOut        io.Writer
	wg            sync.WaitGroup
	errs          []error
	errsMu        sync.Mutex
	quiet         bool
	verbose       bool //nolint:unused // TODO: Will be used soon.
	interactive   bool
	colorsEnabled bool
}

// A StreamWriter is an [io.Writer] created from an instance of [IO] that
// can be used to write to the same output channel.
type StreamWriter struct {
	s      *IO
	output Output
}

// code is the type for the ANSI color codes.
type code int

// message is the type for the output messages that are sent to the IO.
type message struct {
	msg    string
	output Output
}

// promptRequest is the type for the prompts that are sent to the IO.
// A promptRequest signals that the program should wait for user input.
type promptRequest struct {
	prompt   string
	response chan string
}

// NewIO returns a new IO for the given settings.
func NewIO(ctx context.Context, quiet, verbose, interactive bool, colors ColorMode) *IO {
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

	s := &IO{
		promptCh:      make(chan promptRequest),
		outCh:         make(chan message),
		flushCh:       make(chan chan struct{}),
		in:            os.Stdin,
		out:           os.Stdout,
		errOut:        os.Stderr,
		wg:            sync.WaitGroup{},
		errs:          nil,
		errsMu:        sync.Mutex{},
		quiet:         quiet,
		verbose:       verbose,
		interactive:   interactive,
		colorsEnabled: colorsEnabled,
	}

	s.wg.Add(1)
	go s.output(ctx)

	return s
}

// NewWriter creates a new StreamWriter. It panics on errors.
func NewWriter(s *IO, output Output) *StreamWriter {
	if s == nil {
		panic("attempt to create StreamWriter with nil IOStreams")
	}

	return &StreamWriter{
		s:      s,
		output: output,
	}
}

// Ask asks the user for input. It returns the input that the user entered as
// a string and any errors that occurred during the process.
func (s *IO) Ask(prompt string) (string, error) {
	if s.quiet {
		return "", ErrQuietPrompt
	}

	responseCh := make(chan string)

	s.promptCh <- promptRequest{
		prompt:   prompt,
		response: responseCh,
	}

	resp, ok := <-responseCh
	if !ok {
		return "", s.Err()
	}

	return resp, nil
}

// Close closes the IO. It waits for the output goroutine to finish and then
// closes the input and output channels.
func (s *IO) Close() {
	s.Flush()
	close(s.outCh)
	s.wg.Wait()
}

// Confirm asks the user for a boolean input. It returns the input that the user
// entered as a boolean. If the function ecounters an error, it returns false.
// Errors are stored within s. If the program is not interactive, the default
// value is returned.
func (s *IO) Confirm(prompt string, defaultChoice bool) bool {
	confirmed, err := s.ConfirmE(prompt, defaultChoice)
	if err != nil {
		s.errs = append(s.errs, err)

		return false
	}

	return confirmed
}

// ConfirmE asks the user for a boolean input. It returns the input that
// the user entered as a boolean and any errors that occurred during
// the process. If the program is not interactive, the default value is
// returned.
func (s *IO) ConfirmE(prompt string, defaultChoice bool) (bool, error) {
	if !s.interactive {
		return defaultChoice, nil
	}

	if s.quiet {
		return false, ErrQuietPrompt
	}

	var options string
	if defaultChoice {
		options = "[Y/n]"
	} else {
		options = "[y/N]"
	}

	fullPrompt := fmt.Sprintf("%s %s ", strings.TrimSpace(prompt), options)

	for {
		answer, err := s.Ask(fullPrompt)
		if err != nil {
			return false, err
		}

		answer = strings.ToLower(strings.TrimSpace(answer))

		switch strings.ToLower(answer) {
		case "":
			return defaultChoice, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			s.PrintErrf(
				"Invalid input. Please enter \"y\", \"yes\", \"n\", or \"no\".\n",
			)
		}
	}
}

// Err returns the errors that s has encountered. [errors.Join] is called on the
// errors before returning them.
func (s *IO) Err() error {
	s.errsMu.Lock()
	defer s.errsMu.Unlock()

	return errors.Join(s.errs...)
}

// Flush flushes the underlying buffer.
func (s *IO) Flush() {
	ack := make(chan struct{})
	s.flushCh <- ack
	<-ack
}

// Errorf formats according to a format specifier and writes to standard error
// output of s. If colors are enabled, the message is printed in red. It stores
// possible errors within s.
func (s *IO) Errorf(format string, a ...any) {
	s.outCh <- message{
		msg:    s.colorf(red, format, a...),
		output: Stderr,
	}
}

// Print formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are added between operands when neither
// is a string. It stores possible errors within s.
func (s *IO) Print(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:    fmt.Sprint(a...),
		output: Buffered,
	}
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of s. It stores possible errors within s.
func (s *IO) PrintErrf(format string, a ...any) {
	s.outCh <- message{
		msg:    fmt.Sprintf(format, a...),
		output: Stderr,
	}
}

// Printf formats according to a format specifier and writes to standard output
// buffer of s. It stores possible errors within s.
func (s *IO) Printf(format string, a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:    fmt.Sprintf(format, a...),
		output: Buffered,
	}
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are always added between operands and
// a newline is appended. It stores possible errors within s.
func (s *IO) Println(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:    fmt.Sprintln(a...),
		output: Buffered,
	}
}

// Warnln formats using the default formats for its operands and writes to standard error output of s. Spaces are always added between operands and a newline is appended. If colors are enabled, the message is printed in yellow. It stores possible errors within s.
func (s *IO) Warnln(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:    s.colorln(yellow, a...),
		output: Stderr,
	}
}

// Write writes the contents of p into the output channel. It returns the number
// of bytes written.
func (w *StreamWriter) Write(p []byte) (int, error) {
	w.s.outCh <- message{
		msg:    string(p),
		output: w.output,
	}

	return len(p), nil
}

// Ask asks the user for input. It returns the input that the user entered as
// a string and any errors that occurred during the process.
func Ask(prompt string) (string, error) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	return Streams().Ask(prompt)
}

// Confirm asks the user for a boolean input. It returns the input that the user
// entered as a boolean. If the function ecounters an error, it returns false.
// Errors are stored within the default IO streams. If the program is not
// value is returned.
func Confirm(prompt string, defaultChoice bool) bool {
	if streams == nil {
		panic("tried to call nil IO")
	}

	return Streams().Confirm(prompt, defaultChoice)
}

// ConfirmE asks the user for a boolean input. It returns the input that
// the user entered as a boolean and any errors that occurred during
// the process. If the program is not interactive, the default value is
// returned.
func ConfirmE(prompt string, defaultChoice bool) (bool, error) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	return Streams().ConfirmE(prompt, defaultChoice)
}

// Errorf formats according to a format specifier and writes to standard error
// output of [Streams]. If colors are enabled, the message is printed in red. It
// stores possible errors within [Streams].
func Errorf(format string, a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Errorf(format, a...)
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of [Streams]. It stores possible errors within [Streams].
func PrintErrf(format string, a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.PrintErrf(format, a...)
}

// SetStreams set the default global IO instace to the given [IO].
func SetStreams(io *IO) {
	streams = io
}

// Streams returns the default global terminal IO instance.
func Streams() *IO {
	return streams
}

// Warnln formats using the default formats for its operands and writes to standard error output of the default IO streams. Spaces are always added between operands and a newline is appended. If colors are enabled, the message is printed in yellow. It stores possible errors within the default IO streams.
func Warnln(a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Warnln(a...)
}

func (s *IO) appendErr(err error) {
	s.errsMu.Lock()
	defer s.errsMu.Unlock()
	s.errs = append(s.errs, err)
}

func (s *IO) colorf(c code, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)

	if !s.colorsEnabled {
		return msg
	}

	return fmt.Sprintf("%s[%dm%s%s[%dm", escape, c, msg, escape, reset)
}

func (s *IO) colorln(c code, a ...any) string {
	if !s.colorsEnabled {
		return fmt.Sprintln(a...)
	}

	msg := fmt.Sprintln(a...)
	msg = strings.TrimSuffix(msg, "\n")

	return fmt.Sprintf("%s[%dm%s%s[%dm\n", escape, c, msg, escape, reset)
}

// output is the main loop for the IO, run in its own goroutine. It reads
// the messages from the input channel and writes them to the output channel,
// and also handles prompting the user for input.
//
// TODO: Rename the function to something more descriptive as it's not just
// outputting messages anymore.
func (s *IO) output(ctx context.Context) {
	defer s.wg.Done()

	buf := bufio.NewWriter(s.out)
	scanner := bufio.NewScanner(s.in)

	flush := func() {
		if err := buf.Flush(); err != nil {
			s.appendErr(err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()

			return
		case msg, ok := <-s.outCh:
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
				_, err = fmt.Fprint(s.out, msg.msg)
			case Stderr:
				flush()
				_, err = fmt.Fprint(s.errOut, msg.msg)
			default:
				// TODO: Maybe the program should panic here.
				err = fmt.Errorf("%w: %v", errInvalidOutput, msg.output)
			}

			if err != nil {
				s.appendErr(err)
			}
		case p, ok := <-s.promptCh:
			if !ok {
				if err := buf.Flush(); err != nil {
					s.appendErr(err)
				}

				continue
			}

			flush()

			if _, err := buf.WriteString(p.prompt); err != nil {
				s.appendErr(err)
				close(p.response)

				continue
			}

			flush()

			if scanner.Scan() {
				p.response <- scanner.Text()
			} else {
				if err := scanner.Err(); err != nil {
					s.appendErr(err)
				}

				close(p.response)
			}
		case ack := <-s.flushCh:
			flush()
			close(ack)
		}
	}
}
