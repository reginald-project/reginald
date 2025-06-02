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

	"github.com/anttikivi/reginald/pkg/version"
)

// printVersion prints the version information of the standard output.
func printVersion() error {
	if _, err := fmt.Fprintf(os.Stdout, "%s %v\n", ProgramName, version.Version()); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}
