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

func main() {
	p := plugin.New("theme", nil)

	fmt.Fprintln(os.Stderr, "HELLO FROM PLUGIN")

	p.Serve()
}
