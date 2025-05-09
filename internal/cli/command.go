package cli

// A Command is a CLI command. All commands, including the root command and the
// subcommands, must be implemented as commands.
type Command struct {
	// UsageLine is the one-line usage synopsis for the command. It should start
	// with the command name without including the parent commands.
	UsageLine string
}
