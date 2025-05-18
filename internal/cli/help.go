package cli

import (
	"fmt"
	"os"
)

// printHelp prints the printHelp message to the standard output.
func printHelp() error {
	if _, err := fmt.Fprintln(os.Stdout, "HELP MESSAGE"); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
