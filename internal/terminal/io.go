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
	in            io.Reader
	out           io.Writer
	errOut        io.Writer
	promptCh      chan promptRequest
	outCh         chan message
	flushCh       chan chan struct{}
	errs          []error
	quiet         bool
	verbose       bool //nolint:unused // TODO: Will be used soon.
	interactive   bool
	colorsEnabled bool
	errsMu        sync.Mutex
	wg            sync.WaitGroup
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
	response chan string
	prompt   string
}

// NewIO returns a new IO instace.
func NewIO(ctx context.Context) *IO {
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
		quiet:         false,
		verbose:       false,
		interactive:   false,
		colorsEnabled: false,
	}

	s.wg.Add(1)
	go s.output(ctx)

	return s
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
		if len(s.errs) > 0 {
			s.errsMu.Lock()
			defer s.errsMu.Unlock()
			return "", errors.Join(s.errs...)
		}

		return "", nil
	}

	return resp, nil
}

// Close closes the IO. It waits for the output goroutine to finish and then
// closes the input and output channels. It also implements [io.Closer].
func (s *IO) Close() error {
	close(s.outCh)
	close(s.promptCh)
	s.wg.Wait()

	fmt.Println("STREAMS CLOSED", s)

	if len(s.errs) > 0 {
		s.errsMu.Lock()
		defer s.errsMu.Unlock()

		return errors.Join(s.errs...)
	}

	return nil
}

// Confirm asks the user for a boolean input. It returns the input that the user
// entered as a boolean. If the function ecounters an error, it returns false.
// Errors are stored within s. If the program is not interactive, the default
// value is returned.
func (s *IO) Confirm(prompt string, defaultChoice bool) bool {
	confirmed, err := s.ConfirmE(prompt, defaultChoice)
	if err != nil {
		s.appendErr(err)

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

// Flush flushes the underlying buffer.
func (s *IO) Flush() {
	ack := make(chan struct{})
	s.flushCh <- ack

	<-ack
}

// InitStreams initializes s for by propagating the config values.
func (s *IO) Init(quiet, verbose, interactive bool, colors ColorMode) {
	s.quiet = quiet
	s.verbose = verbose
	s.interactive = interactive

	switch colors {
	case ColorAlways:
		s.colorsEnabled = true
	case ColorNever:
		s.colorsEnabled = false
	case ColorAuto:
		s.colorsEnabled = term.IsTerminal(int(os.Stdout.Fd()))
	default:
		panic(fmt.Sprintf("invalid IOStreams color mode: %v", colors))
	}
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

// PrintErrf formats according to a format specifier and writes to standard
// error output of s. It stores possible errors within s.
func (s *IO) PrintErrf(format string, a ...any) {
	s.outCh <- message{
		msg:    fmt.Sprintf(format, a...),
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

// Warnln formats using the default formats for its operands and writes to
// standard error output of s. Spaces are always added between operands and
// a newline is appended. If colors are enabled, the message is printed in
// yellow. It stores possible errors within s.
func (s *IO) Warnln(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:    s.colorln(yellow, a...),
		output: Stderr,
	}
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

// Flush flushes the underlying buffer of [Streams].
func Flush() {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Flush()
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of [Streams]. It stores possible errors within [Streams].
func PrintErrf(format string, a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.PrintErrf(format, a...)
}

// Print formats using the default formats for its operands and writes to
// standard output buffer of [Streams]. Spaces are added between operands when
// neither is a string. It stores possible errors within [Streams].
func Print(a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Print(a...)
}

// Printf formats according to a format specifier and writes to standard output
// buffer of [Streams]. It stores possible errors within [Streams].
func Printf(format string, a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Printf(format, a...)
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of [Streams]. Spaces are always added between operands
// and a newline is appended. It stores possible errors within [Streams].
func Println(a ...any) {
	if streams == nil {
		panic("tried to call nil IO")
	}

	streams.Println(a...)
}

// SetStreams set the default global IO instace to the given [IO].
func SetStreams(s *IO) {
	streams = s
}

// Streams returns the default global terminal IO instance.
func Streams() *IO {
	return streams
}

// Warnln formats using the default formats for its operands and writes to
// standard error output of the default IO streams. Spaces are always added
// between operands and a newline is appended. If colors are enabled,
// the message is printed in yellow. It stores possible errors within
// the default IO streams.
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

func (s *IO) doPrompt(p promptRequest, buf *bufio.Writer, scanner *bufio.Scanner, flush func()) {
	flush()

	if _, err := buf.WriteString(p.prompt); err != nil {
		s.appendErr(err)
		close(p.response)

		return
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
}

// output is the main loop for the IO, run in its own goroutine. It reads
// the messages from the input channel and writes them to the output channel,
// and also handles prompting the user for input.
//
// TODO: Rename the function to something more descriptive as it's not just
// outputting messages anymore.
func (s *IO) output(ctx context.Context) {
	defer s.wg.Done()

	// In case the context is canceled while the input is being read with
	// scanner.Scan(), close the input to prevent deadlock.
	go func() {
		<-ctx.Done()
		if closer, ok := s.in.(io.Closer); ok {
			closer.Close()
		}
	}()

	buf := bufio.NewWriter(s.out)
	scanner := bufio.NewScanner(s.in)

	flush := func() {
		if err := buf.Flush(); err != nil {
			s.appendErr(err)
		}
	}

	defer flush()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-s.outCh:
			if !ok {
				if err := buf.Flush(); err != nil {
					s.appendErr(err)
				}

				return
			}

			s.writeOut(msg, buf, flush)
		case p, ok := <-s.promptCh:
			if !ok {
				if err := buf.Flush(); err != nil {
					s.appendErr(err)
				}

				continue
			}

			s.doPrompt(p, buf, scanner, flush)
		case ack := <-s.flushCh:
			flush()
			close(ack)
		}
	}
}

func (s *IO) writeOut(msg message, buf *bufio.Writer, flush func()) {
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
		// TODO: Maybe the program should panic or something here.
		err = fmt.Errorf("%w: %v", errInvalidOutput, msg.output)
	}

	if err != nil {
		s.appendErr(err)
	}
}
