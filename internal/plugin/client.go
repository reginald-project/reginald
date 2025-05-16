package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anttikivi/reginald/pkg/rpp"
)

func ResolvePlugins(files []string) {
	plugins, err := Collect(files)
	if err != nil {
		panic(fmt.Sprintf("failed to find the plugins: %v", err))
	}

	for _, p := range plugins {
		slog.Debug("executing plugin", "plugin", p.cmd.Path)

		if err := p.cmd.Start(); err != nil {
			panic(fmt.Sprintf("failed to start plugin at %s", p.cmd.Path))
		}

		handshake(&p)
	}
}

func handshake(p *Plugin) {
	params := rpp.HandshakeParams{
		Protocol:        "rpp",
		ProtocolVersion: 0,
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal handshake params: %v", err))
	}

	req := rpp.Message{
		JSONRCP: "2.0",
		ID:      1,
		Method:  "handshake",
		Params:  rawParams,
	}

	if err = rpp.Write(p.w, req); err != nil {
		panic(err)
	}

	p.w.Flush()
}
