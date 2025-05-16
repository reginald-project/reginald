// Package plugin implements an RPP client in Reginald to run plugins.
package plugin

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// A Plugin represents a plugin that acts as an RPP server and is run from this
// client.
type Plugin struct {
	cmd    *exec.Cmd      // command struct used to run the server
	stdin  io.WriteCloser // stdin pipe of the plugin command
	stdout io.ReadCloser  // stdout pipe of the plugin command
	r      *bufio.Reader
	w      *bufio.Writer
}

func Collect(files []string) ([]Plugin, error) {
	plugins := []Plugin{}

	for _, f := range files {
		c := exec.Command(f)

		stdin, err := c.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create standard input pipe for %s: %w", filepath.Base(f), err)
		}

		stdout, err := c.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create standard output pipe for %s: %w", filepath.Base(f), err)
		}

		// TODO: Do we need this?
		c.Stderr = os.Stderr
		p := Plugin{
			cmd:    c,
			stdin:  stdin,
			stdout: stdout,
		}
		p.r = bufio.NewReader(p.stdout)
		p.w = bufio.NewWriter(p.stdin)
		plugins = append(plugins, p)
	}

	return plugins, nil
}
