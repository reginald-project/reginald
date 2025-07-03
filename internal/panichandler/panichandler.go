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

// Package panichandler defines the panic handler functions for Reginald. They
// need to be deferred at the beginning of each goroutine. The functions print
// a helpful message that guides to issue a bug report in case the program
// crashes.
package panichandler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/reginald-project/reginald/internal/log/writer"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/internal/text"
	"github.com/reginald-project/reginald/internal/version"
)

const (
	header = "!!! REGINALD CRASHED !%s"
	//nolint:lll
	panicInfo = `
Reginald has encountered an unexpected error. This is most likely a bug in the program. In your bug report, please include the Reginald version and stack trace shown below and any additional information that may help with resolving the bug or replicating the issue.
`
	footer = `
Please open an issue at:

	https://github.com/reginald-project/reginald/issues

Thank you for helping Reginald!
`
)

// panicMu is a mutex used to lock the panic handler in case multiple goroutines
// panic simultaneously. It ensures that only the first one recovers, prints the
// message, and exits the program.
var panicMu sync.Mutex //nolint:gochecknoglobals // used be multiple goroutines

// cancel is the cancel function for the program context. It should be set at
// the beginning of the program. It must be run before exiting the program.
var cancel context.CancelFunc //nolint:gochecknoglobals // global cancel for the context

// cancelOnce is used to ensure that cancel is only set once.
var cancelOnce sync.Once //nolint:gochecknoglobals // global cancel for the context

// Handle recovers the panics of the program and prints the information included
// with them with the stack trace and a helpful message that guides the user to
// report the bug using the issue tracker.
func Handle() {
	panicMu.Lock()
	defer panicMu.Unlock()

	//revive:disable-next-line:defer This is a deferred function.
	r := recover()

	handlePanic(r, nil)
}

// WithStackTrace returns a function that is similar to Handle but it captures
// the current stack trace to it. This way the panic handler can print the full
// stack trace leading up to creating the panic handler with this function if a
// panic happens outside of the main goroutine.
func WithStackTrace() func() {
	trace := debug.Stack()

	return func() {
		panicMu.Lock()
		defer panicMu.Unlock()

		//revive:disable-next-line:defer This is a deferred function.
		r := recover()

		handlePanic(r, trace)
	}
}

// SetCancel sets the cancel function for the program context.
func SetCancel(c context.CancelFunc) {
	cancelOnce.Do(func() {
		cancel = c
	})
}

func handlePanic(r any, t []byte) {
	if r == nil {
		return
	}

	if cancel == nil {
		panic("cancel function not set")
	}

	cancel()

	var buf bytes.Buffer

	buf.WriteByte('\n')

	width := terminal.Width()

	buf.WriteString(fmt.Sprintf(header, strings.Repeat("!", width-len(header)+1)))
	buf.WriteString("\n\n")
	buf.WriteString(text.Wrap(panicInfo, width))
	buf.WriteByte('\n')
	buf.WriteString(fmt.Sprintf("Version: %s\n", version.Version()))
	buf.WriteString(fmt.Sprintf("Panic: %v\n\n", r))
	buf.WriteString("Stack trace:\n\n")
	buf.Write(debug.Stack())

	if t != nil {
		buf.WriteString("\nWith goroutine called from:\n\n")
		buf.Write(t)
	}

	if w, ok := writer.BootstrapWriter.(*writer.BufferedFileWriter); ok {
		if err := w.Flush(); err != nil {
			buf.WriteString(fmt.Sprintf("\nFailed to write the boostrap log to file: %v\n\n", err))
			buf.WriteString("The bootstrap logs:\n")
			buf.Write(w.Bytes())
		} else {
			buf.WriteString(fmt.Sprintf("\nBootstrap log is written to %s\n", w.File()))
			buf.WriteString("Consider including it when opening an issue.")
		}
	}

	buf.WriteString("\n" + footer)

	if _, err := os.Stderr.Write(buf.Bytes()); err != nil {
		// This is absolutely stupid but, if we get here, all is lost anyway.
		buf.WriteString(fmt.Sprintf("FAILED TO WRITE BYTES TO STDERR: %v\n", err))
	}

	//revive:disable-next-line:deep-exit Panic handler has to exit with error.
	os.Exit(1)
}
