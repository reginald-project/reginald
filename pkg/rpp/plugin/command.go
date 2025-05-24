package plugin

import "github.com/anttikivi/reginald/pkg/rpp"

// A Command is an interface that the commands defined by plugins that use the
// provided plugin server should implement. The plugin server uses the values
// returned by this interface to create the required messages and the functions
// to run the command.
type Command interface {
	// Name returns the name of the command as it should be written by the user
	// when they run the command. It must not match any existing commands either
	// within Reginald or other plugins.
	Name() string

	// UsageLine is the one-line usage synopsis of the command.
	UsageLine() string

	// Flags returns the command-line flag definitions for this command. If
	// len(Flags()) == 0, the value will be omitted in the handshake.
	Flags() []rpp.Flag

	Run(args []string) error
}
