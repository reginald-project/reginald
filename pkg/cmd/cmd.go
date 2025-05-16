// Package cmd defines the public interface for creating commands for Reginald.
package cmd

// A Cmd is an interface that the commands defined by plugins that use the
// provided plugin server should implement. The plugin server uses the values
// returned by this interface to create the required messages and the functions
// to run the command.
type Cmd interface {
	Run(args []string) error
}
