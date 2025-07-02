// Copyright 2025 The Reginald Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Reginald plugin for Go.
*/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/reginald-project/reginald-sdk-go/api"
)

func main() {
	fmt.Fprintln(os.Stderr, "Hello world")

	opts := &ServerOpts{
		Name:        "reginald-go",
		Version:     "0.1.0",
		Domain:      "go",
		Description: "TODO",
		Help:        "TODO",
		Executable:  "reginald-go",
		Config: []api.ConfigEntry{
			{ //nolint:exhaustruct // omit default values
				ConfigValue: api.ConfigValue{ //nolint:exhaustruct // omit default values
					KeyVal: api.KeyVal{
						Value: api.Value{
							Val:  "1.23",
							Type: api.StringValue,
						},
						Key: "version",
					},
				},
			},
		},
	}

	versionsCmd := &Command{ //nolint:exhaustruct // omit default values
		Name:  "versions",
		Usage: "versions [options]",
		Args:  nil,
		Run: func(_ api.KeyValues) error {
			fmt.Fprintln(os.Stderr, "running versions")
			slog.InfoContext(context.TODO(), "running command", "cmd", "versions")

			return nil
		},
	}

	server := NewServer(opts, versionsCmd)

	slog.SetDefault(slog.New(NewRPCHandler(server, nil)))

	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s is going to exit with an error: %v", opts.Name, err)
		os.Exit(1)
	}
}
