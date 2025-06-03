// Copyright 2025 Antti Kivi
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

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/anttikivi/reginald/pkg/version"
)

// printVersion prints the version information of the standard output.
func printVersion(cmd *Command) error {
	var sb strings.Builder

	if cmd != nil && cmd.plugin != nil {
		sb.WriteString(
			fmt.Sprintf("%s (plugin %s) %s\n\n", cmd.Name, cmd.plugin.Name, cmd.plugin.Version),
		)
	}

	sb.WriteString(fmt.Sprintf("%s %v\n", ProgramName, version.Version()))
	sb.WriteString("Copyright (c) 2025 Antti Kivi\n")
	sb.WriteString("Licensed under Apache-2.0: <http://www.apache.org/licenses/LICENSE-2.0>")

	_, err := fmt.Fprintln(os.Stdout, sb.String())
	if err != nil {
		return fmt.Errorf("failed to write the version information: %w", err)
	}

	return nil
}
