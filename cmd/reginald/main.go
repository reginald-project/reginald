// Package main is the entry point for Reginald, the personal workstation valet.
// TODO: Add a comment describing the actual command when there is something to
// describe.
package main

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/internal/version"
)

func main() {
	fmt.Fprintln(os.Stdout, "Hello, world!", version.Version)
}
