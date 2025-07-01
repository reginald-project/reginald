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

//go:build script

package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run prints and executes the given command.
func Run(args ...string) error {
	exe, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	announce(args...)

	cmd := exec.Command(exe, args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func announce(args ...string) {
	fmt.Println(quote(args))
}

func quote(args []string) string {
	fmtArgs := make([]string, len(args))

	for i, arg := range args {
		if strings.ContainsAny(arg, " \t'\"") {
			fmtArgs[i] = fmt.Sprintf("%q", arg)
		} else {
			fmtArgs[i] = arg
		}
	}

	return strings.Join(fmtArgs, " ")
}
