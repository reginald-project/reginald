package rpp

import (
	"bufio"
	"fmt"
	"os"
)

// A Plugin is a plugin server that implements RPP. Plugins written in Go can
// use it instead of implementing the protocol.
type Plugin struct{}

func (p *Plugin) Serve() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	msg, err := Read(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error while reading:", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Received a message:", msg)
}
