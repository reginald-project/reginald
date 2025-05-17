// This package defines the 'theme' plugin for Reginald. It changes my dotfiles
// to use the color theme that I specify. This plugin is purely for my own
// purposes and might be removed in the future. Right now it is included for
// testing the plugin system.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anttikivi/reginald/pkg/rpp"
)

func main() {
	fmt.Fprintln(os.Stderr, "HELLO FROM PLUGIN")

	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)

Loop:
	for {
		msg, err := rpp.Read(in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERROR IN PLUGIN: ", err.Error())

			os.Exit(1)
		}

		switch msg.Method {
		case rpp.Handshake:
			result := rpp.HandshakeResult{
				Protocol:        rpp.Name,
				ProtocolVersion: rpp.Version,
				Kind:            "command",
				Name:            "theme",
			}
			if err := sendResponse(out, msg.ID, result); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
			}

			break Loop
		}
	}
}

func sendResponse(w *bufio.Writer, id rpp.ID, result any) error {
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal call results: %w", err)
	}

	err = rpp.Write(w, &rpp.Message{
		JSONRCP: rpp.JSONRCPVersion,
		ID:      id,
		Result:  rawResult,
	})
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	if err = w.Flush(); err != nil {
		return fmt.Errorf("flushing the output buffer failed: %w", err)
	}

	return nil
}
