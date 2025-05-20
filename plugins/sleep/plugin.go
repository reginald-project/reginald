// This package defines the 'theme' plugin for Reginald. It changes my dotfiles
// to use the color theme that I specify. This plugin is purely for my own
// purposes and might be removed in the future. Right now it is included for
// testing the plugin system.
package main

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/anttikivi/reginald/pkg/rpp/plugin"
)

// Sleep is the command implementation for the sleep plugin.
type Sleep struct{}

// Flags returns the flags supported by this command.
func (s *Sleep) Flags() []rpp.Flag {
	return nil
}

// Run executes the command for the plugin.
func (s *Sleep) Run(_ []string) error {
	return nil
}

func main() {
	p := plugin.New("sleep", &Sleep{})

	if err := p.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "plugin %q is going to exit with an error: %v", "sleep", err)
		os.Exit(1)
	}
}
