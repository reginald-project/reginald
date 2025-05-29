// Package main defines an example plugin for Reginald using the provided Go
// functions and types.
package main

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/anttikivi/reginald/pkg/rpp/plugin"
)

// SleepCommand is the command implementation for the sleep plugin.
type SleepCommand struct{}

// Name returns the name of the command as it should be written by the user when
// they run the command. It must not match any existing commands either within
// Reginald or other plugins.
func (s *SleepCommand) Name() string {
	return "sleep"
}

// UsageLine returns the one-line usage synopsis for the command. It should
// start with the command name.
func (s *SleepCommand) UsageLine() string {
	return "sleep [options]"
}

// Flags returns the flags supported by this command.
func (s *SleepCommand) Configs() []rpp.ConfigValue {
	return []rpp.ConfigValue{
		{
			Key:   "time",
			Value: 5,
			Type:  rpp.ConfigInt,
			Flag: rpp.Flag{
				Shorthand: "t",
				Usage:     "time to sleep in seconds (default 5s)",
			},
		},
	}
}

// Run executes the command for the plugin.
func (s *SleepCommand) Run(_ []string) error {
	return nil
}

func main() {
	p := plugin.New("sleep", &SleepCommand{})

	if err := p.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "plugin %q is going to exit with an error: %v", "sleep", err)
		os.Exit(1)
	}
}
