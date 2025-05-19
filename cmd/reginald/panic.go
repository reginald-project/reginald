package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/anttikivi/go-semver"
	"github.com/anttikivi/reginald/internal/version"
)

const header = `
=============================== REGINALD CRASHED ===============================

Reginald crashed! This almost always indicates a bug within the program. Please
report the crash with Reginald so that we can fix this.

In your bug report, please always include the Reginald version, the stack trace
shown below, and any additional information that may help with resolving the bug
or replicating the issue.
`

const hl = `
================================================================================
`

const footer = `
Please open an issue at:
	https://github.com/anttikivi/reginald/issues

Thank you!
`

// panicHandler recovers the panics of the program and prints the information
// included with them with the stack trace and a helpful message that guides the
// user to report the bug using the issue tracker.
func panicHandler() {
	r := recover()
	if r == nil {
		return
	}

	fmt.Fprint(os.Stderr, header)
	fmt.Fprint(os.Stderr, hl)
	fmt.Fprintln(os.Stderr, "")

	if v, err := semver.Parse(version.Version); err != nil {
		fmt.Fprintf(os.Stderr, "Version: %s\n", version.Version)
		fmt.Fprintf(os.Stderr, "FAILED TO PARSE VERSION: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Version: %v\n", v)
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", r)
	fmt.Fprintln(os.Stderr, "Stack trace:")
	fmt.Fprintf(os.Stderr, "\n%s", debug.Stack())
	// debug.PrintStack()
	fmt.Fprint(os.Stderr, hl)
	fmt.Fprintf(os.Stderr, "\n%s", footer)

	os.Exit(2)
}
