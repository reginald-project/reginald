package panichandler

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"github.com/anttikivi/go-semver"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/version"
)

const header = `
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! REGINALD CRASHED !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

Reginald has encountered an unexpected error. This is most likely a bug in the
program. In your bug report, please include the Reginald version and stack trace
shown below and any additional information that may help with resolving the bug
or replicating the issue.

`

const footer = `
Please open an issue at:

	https://github.com/anttikivi/reginald/issues

Thank you for helping Reginald!
`

// panicMu is a mutex used to lock the panic handler in case multiple goroutines
// panic simultaneously. It ensures that only the first one recovers, prints the
// message, and exits the program.
var panicMu sync.Mutex

// Handle recovers the panics of the program and prints the information included
// with them with the stack trace and a helpful message that guides the user to
// report the bug using the issue tracker.
func Handle() {
	v := versionInfo()

	panicMu.Lock()
	defer panicMu.Unlock()

	r := recover()

	panicHandler(r, v, nil)
}

// WithStackTrace returns a function that is similar to Handle but it captures
// the current stack trace to it. This way the panic handler can print the full
// stack trace leading up to creating the panic handler with this function if a
// panic happens outside of the main goroutine.
func WithStackTrace() func() {
	v := versionInfo()
	trace := debug.Stack()

	return func() {
		panicMu.Lock()
		defer panicMu.Unlock()

		r := recover()

		panicHandler(r, v, trace)
	}
}

func versionInfo() string {
	if v, err := semver.Parse(version.Version); err != nil {
		return fmt.Sprintf("Version: %s\nParsing the version failed: %v", version.Version, err)
	} else {
		return fmt.Sprint("Version: ", v)
	}
}

func panicHandler(r any, v string, t []byte) {
	if r == nil {
		return
	}

	var buf bytes.Buffer

	buf.WriteString(header)
	buf.WriteString(v)
	buf.WriteByte('\n')
	buf.WriteString(fmt.Sprintf("Panic: %v\n\n", r))
	buf.WriteString("Stack trace:\n\n")
	buf.Write(debug.Stack())

	if t != nil {
		buf.WriteString("\nWith goroutine called from:\n\n")
		buf.Write(t)
	}

	buf.WriteString(fmt.Sprintf("\n%s", footer))
	iostreams.StdioMu.Lock()
	os.Stderr.Write(buf.Bytes())
	iostreams.StdioMu.Unlock()
	os.Exit(1)
}
