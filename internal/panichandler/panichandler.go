package panichandler

import (
	"bytes"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/anttikivi/go-semver"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/version"
	"golang.org/x/term"
)

const (
	header    = "!!! REGINALD CRASHED !%s"
	panicInfo = `
Reginald has encountered an unexpected error. This is most likely a bug in the program. In your bug report, please include the Reginald version and stack trace shown below and any additional information that may help with resolving the bug or replicating the issue.
`
	footer = `
Please open an issue at:

	https://github.com/anttikivi/reginald/issues

Thank you for helping Reginald!
`
)

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

	buf.WriteByte('\n')

	width := termWidth()

	buf.WriteString(fmt.Sprintf(header, strings.Repeat("!", width-len(header)+1)))
	buf.WriteString("\n\n")
	buf.WriteString(wrap(panicInfo, width))
	buf.WriteByte('\n')
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

func wrap(s string, width int) string {
	var sb strings.Builder

	for p := range strings.SplitSeq(s, "\n\n") {
		words := strings.Fields(p)
		l := 0

		for i, w := range words {
			addForSpace := 0

			if l > 0 {
				addForSpace = 1
			}

			if l+len(w)+addForSpace > width {
				sb.WriteByte('\n')
				l = 0
			}

			if l > 0 {
				sb.WriteByte(' ')
				l++
			}

			sb.WriteString(w)

			l += len(w)

			if i == len(words)-1 {
				sb.WriteString("\n\n")
			}
		}
	}

	result := sb.String()

	if strings.HasSuffix(result, "\n\n") {
		result = result[:len(result)-1]
	}

	return result
}

// termWidth returns the current terminal width (in characters) or a default of
// 80 if it cannot be determined.
func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}

	return 80
}
