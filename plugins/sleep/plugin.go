// This package defines the 'theme' plugin for Reginald. It changes my dotfiles
// to use the color theme that I specify. This plugin is purely for my own
// purposes and might be removed in the future. Right now it is included for
// testing the plugin system.
package main

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp/plugin"
)

// Sleep is the command implementation for the sleep plugin.
type Sleep struct{}

func (s *Sleep) Run(args []string) error {
	return nil
}

func main() {
	p := plugin.New("sleep", &Sleep{})

	fmt.Fprintln(os.Stderr, "HELLO FROM PLUGIN")

	p.Serve()
}
