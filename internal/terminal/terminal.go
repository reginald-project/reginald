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

// Package terminal defines the terminal utilities for the Reginald terminal
// user interface. Most importantly, it defines the global instance that should
// be used for input and output in the program.
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

	"github.com/chzyer/readline"
	"golang.org/x/term"
)

// Message output destinations.
const (
	Buffered OutputMode = iota
	Stdout
	Stderr
)

// ASCII control characters.
const (
	escape = '\x1b'
)

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

// defaultPrintWidth is the default width returned by Width if the width of
// the terminal cannot be determined.
const defaultWidth = 80

// ErrQuietPrompt is returned when a prompt is requested from the user in quiet
// mode.
var ErrQuietPrompt = errors.New("cannot prompt for input in quiet mode")

// terminal is the global terminal instance for the program. It must be
// initialized before use.
var terminal *Terminal //nolint:gochecknoglobals // global Terminal instance

// errInvalidOutput is appended to the terminal errors when a message has
// an invalid output value.
var errInvalidOutput = errors.New("invalid message output")

// errNoResponse is returned when the Terminal functions do not receive
// a response.
var errNoResponse = errors.New("no response received")

// OutputMode is the property of a message that tells whether the message should
// be buffered for output or printed to output or error output.
type OutputMode int

// A Terminal is used to interact with the terminal, and it is used for the user
// interface. It ensures sequential reading and writing of messages, and it also
// handles prompting the user for input. If the reading or writing operations
// using this type return an error, it will be stored within the struct.
type Terminal struct {
	in            io.ReadCloser
	out           io.Writer
	errOut        io.Writer
	promptCh      chan promptRequest
	outCh         chan message
	flushCh       chan chan struct{}
	err           *asyncError // stores the asynchronous errors
	quiet         bool
	verbose       bool //nolint:unused // TODO: Will be used soon.
	interactive   bool
	colorsEnabled bool
	wg            sync.WaitGroup
}

// code is the type for the ANSI color codes.
type code int

// message is the type for the output messages that are sent to the Terminal.
type message struct {
	msg  string
	mode OutputMode
}

// A promptRequest is the type for the prompts that are sent to the Terminal. It
// signals that the program should wait for user input.
type promptRequest struct {
	response chan promptResponse
	prompt   string
}

// A promptResponse is the type for the responses to prompts.
type promptResponse struct {
	err      error  // any error that occurred during the prompt
	response string // the response to the prompt
}

// New returns a new Terminal.
func New(ctx context.Context) *Terminal {
	s := &Terminal{
		promptCh: make(chan promptRequest),
		outCh:    make(chan message),
		flushCh:  make(chan chan struct{}),
		in:       readline.NewCancelableStdin(os.Stdin),
		out:      os.Stdout,
		errOut:   os.Stderr,
		wg:       sync.WaitGroup{},
		err: &asyncError{
			errs: make([]error, 0),
			mu:   sync.Mutex{},
		},
		quiet:         false,
		verbose:       false,
		interactive:   false,
		colorsEnabled: false,
	}

	s.wg.Add(1)
	go s.doIO(ctx)

	return s
}

// Ask asks the user for input. It returns the input that the user entered as
// a string and any errors that occurred during the process.
func (s *Terminal) Ask(ctx context.Context, prompt string) (string, error) {
	if s.quiet {
		return "", ErrQuietPrompt
	}

	responseCh := make(chan promptResponse, 1)

	s.promptCh <- promptRequest{
		prompt:   prompt,
		response: responseCh,
	}

	select {
	case resp, ok := <-responseCh:
		if !ok {
			return "", errNoResponse
		}

		if resp.err != nil {
			return "", resp.err
		}

		return resp.response, nil
	case <-ctx.Done():
		return "", fmt.Errorf("%w: %w", errNoResponse, ctx.Err())
	}
}

// Close closes the Terminal. It waits for the output goroutine to finish and
// then closes the input and output channels. It also implements [io.Closer].
func (s *Terminal) Close() error {
	close(s.outCh)
	close(s.promptCh)
	s.wg.Wait()

	err := s.err.joined()
	if err != nil {
		return err
	}

	return nil
}

// Confirm asks the user for a boolean input. It returns the input that the user
// entered as a boolean. If the function ecounters an error, it returns false.
// Errors are stored within s. If the program is not interactive, the default
// value is returned.
func (s *Terminal) Confirm(ctx context.Context, prompt string, defaultChoice bool) bool {
	confirmed, err := s.ConfirmE(ctx, prompt, defaultChoice)
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
func (s *Terminal) ConfirmE(ctx context.Context, prompt string, defaultChoice bool) (bool, error) {
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
		answer, err := s.Ask(ctx, fullPrompt)
		if err != nil {
			// TODO: Should some errors be tolerated?
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
			s.PrintErrf("Invalid input. Please enter \"y\", \"yes\", \"n\", or \"no\".\n")
		}
	}
}

// Flush flushes the underlying buffer.
func (s *Terminal) Flush() {
	ack := make(chan struct{})
	s.flushCh <- ack

	<-ack
}

// Init initializes s for by propagating the config values.
func (s *Terminal) Init(quiet, verbose, interactive bool, colors ColorMode) {
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
		panic(fmt.Sprintf("invalid Terminal color mode: %v", colors))
	}
}

// Errorf formats according to a format specifier and writes to standard error
// output of s. If colors are enabled, the message is printed in red. It stores
// possible errors within s.
func (s *Terminal) Errorf(format string, a ...any) {
	s.outCh <- message{
		msg:  s.colorf(red, format, a...),
		mode: Stderr,
	}
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of s. It stores possible errors within s.
func (s *Terminal) PrintErrf(format string, a ...any) {
	s.outCh <- message{
		msg:  fmt.Sprintf(format, a...),
		mode: Stderr,
	}
}

// Print formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are added between operands when neither
// is a string. It stores possible errors within s.
func (s *Terminal) Print(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:  fmt.Sprint(a...),
		mode: Buffered,
	}
}

// Printf formats according to a format specifier and writes to standard output
// buffer of s. It stores possible errors within s.
func (s *Terminal) Printf(format string, a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:  fmt.Sprintf(format, a...),
		mode: Buffered,
	}
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of s. Spaces are always added between operands and
// a newline is appended. It stores possible errors within s.
func (s *Terminal) Println(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:  fmt.Sprintln(a...),
		mode: Buffered,
	}
}

// Warnln formats using the default formats for its operands and writes to
// standard error output of s. Spaces are always added between operands and
// a newline is appended. If colors are enabled, the message is printed in
// yellow. It stores possible errors within s.
func (s *Terminal) Warnln(a ...any) {
	if s.quiet {
		return
	}

	s.outCh <- message{
		msg:  s.colorln(yellow, a...),
		mode: Stderr,
	}
}

// Ask asks the user for input. It returns the input that the user entered as
// a string and any errors that occurred during the process.
func Ask(ctx context.Context, prompt string) (string, error) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	return Default().Ask(ctx, prompt)
}

// Confirm asks the user for a boolean input. It returns the input that the user
// entered as a boolean. If the function ecounters an error, it returns false.
// Errors are stored within the default Terminal. If the program is not value is
// returned.
func Confirm(ctx context.Context, prompt string, defaultChoice bool) bool {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	return Default().Confirm(ctx, prompt, defaultChoice)
}

// ConfirmE asks the user for a boolean input. It returns the input that
// the user entered as a boolean and any errors that occurred during
// the process. If the program is not interactive, the default value is
// returned.
func ConfirmE(ctx context.Context, prompt string, defaultChoice bool) (bool, error) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	return Default().ConfirmE(ctx, prompt, defaultChoice)
}

// Errorf formats according to a format specifier and writes to standard error
// output of [Default]. If colors are enabled, the message is printed in red. It
// stores possible errors within [Default].
func Errorf(format string, a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Errorf(format, a...)
}

// Default returns the default terminal instance.
func Default() *Terminal {
	return terminal
}

// Flush flushes the underlying buffer of [Default].
func Flush() {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Flush()
}

// PrintErrf formats according to a format specifier and writes to standard
// error output of [Default]. It stores possible errors within [Default].
func PrintErrf(format string, a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.PrintErrf(format, a...)
}

// Print formats using the default formats for its operands and writes to
// standard output buffer of [Default]. Spaces are added between operands when
// neither is a string. It stores possible errors within [Default].
func Print(a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Print(a...)
}

// Printf formats according to a format specifier and writes to standard output
// buffer of [Default]. It stores possible errors within [Default].
func Printf(format string, a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Printf(format, a...)
}

// Println formats using the default formats for its operands and writes to
// standard output buffer of [Default]. Spaces are always added between operands
// and a newline is appended. It stores possible errors within [Default].
func Println(a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Println(a...)
}

// Set sets the default Terminal instance.
func Set(s *Terminal) {
	terminal = s
}

// Warnln formats using the default formats for its operands and writes to
// standard error output of the default Terminal. Spaces are always added
// between operands and a newline is appended. If colors are enabled,
// the message is printed in yellow. It stores possible errors within
// the default Terminal.
func Warnln(a ...any) {
	if terminal == nil {
		panic("tried to call nil Terminal")
	}

	terminal.Warnln(a...)
}

// Width returns the current terminal width (in characters) or a default of 80
// if it cannot be determined.
func Width() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}

	return defaultWidth
}

func (s *Terminal) appendErr(err error) {
	s.err.append(err)
}

func (s *Terminal) colorf(c code, format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)

	if !s.colorsEnabled {
		return msg
	}

	return fmt.Sprintf("%c[%dm%s%c[%dm", escape, c, msg, escape, reset)
}

func (s *Terminal) colorln(c code, a ...any) string {
	if !s.colorsEnabled {
		return fmt.Sprintln(a...)
	}

	msg := fmt.Sprintln(a...)
	msg = strings.TrimSuffix(msg, "\n")

	return fmt.Sprintf("%c[%dm%s%c[%dm\n", escape, c, msg, escape, reset)
}

// doIO is the main loop for the IO, run in its own goroutine.
func (s *Terminal) doIO(ctx context.Context) {
	defer s.wg.Done()

	// In case the context is canceled while the input is being read with
	// scanner.Scan(), close the input to prevent deadlock.
	go func() {
		<-ctx.Done()

		if err := s.in.Close(); err != nil {
			s.appendErr(err)
		}
	}()

	buf := bufio.NewWriter(s.out)

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

			flush()

			s.doPrompt(p)
		case ack := <-s.flushCh:
			flush()
			close(ack)
		}
	}
}

func (s *Terminal) doPrompt(p promptRequest) {
	rlCfg := &readline.Config{ //nolint:exhaustruct // use default values
		Prompt:                 p.prompt,
		DisableAutoSaveHistory: true,
		Stdin:                  s.in,
		Stdout:                 s.out,
		Stderr:                 s.errOut,
	}

	rl, err := readline.NewEx(rlCfg)
	if err != nil {
		p.response <- promptResponse{
			response: "",
			err:      err,
		}
		close(p.response)

		return
	}

	defer func() {
		if closeErr := rl.Close(); closeErr != nil {
			s.appendErr(closeErr)
		}
	}()
	rl.CaptureExitSignal()

	line, err := rl.Readline()
	if err != nil {
		p.response <- promptResponse{
			response: "",
			err:      err,
		}
		close(p.response)

		return
	}

	p.response <- promptResponse{
		response: line,
		err:      nil,
	}
}

func (s *Terminal) writeOut(msg message, buf *bufio.Writer, flush func()) {
	var err error

	switch msg.mode {
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
		err = fmt.Errorf("%w: %v", errInvalidOutput, msg.mode)
	}

	if err != nil {
		s.appendErr(err)
	}
}
