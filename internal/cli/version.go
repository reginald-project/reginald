package cli

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/version"
)

// printVersion prints the version information of the standard output.
func printVersion(c *CLI) error {
	if _, err := fmt.Fprintf(os.Stdout, "%s %v\n", ProgramName, version.Version()); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
