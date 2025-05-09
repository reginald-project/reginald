package cli

// The name of command-line tool.
const name = "reginald"

// New returns the root command for the command-line interface. It adds all of
// the necessary global options to the command and creates the subcommands and
// registers them to the root commands.
func New() *Command {
	c := &Command{
		UsageLine: name,
	}

	return c
}
