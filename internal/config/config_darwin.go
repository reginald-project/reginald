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

package config

import (
	"fmt"
	"os"

	"github.com/reginald-project/reginald/internal/fspath"
)

func defaultOSPluginPaths() ([]fspath.Path, error) {
	path, err := xdgPluginPath()
	if err != nil {
		return nil, err
	}

	if path != "" {
		return []fspath.Path{path}, nil
	}

	var home string

	home, err = os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get the user home directory: %w", err)
	}

	// This might be a stupid default but use the same default on macOS as on
	// Linux if it exists. ~/.local/share is not _really_ a macOS thing so this
	// default is kinda opt-in by design.
	path, err = fspath.NewAbs(home, ".local", "share", defaultPrefix, "plugins")
	if err != nil {
		return nil, fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
	}

	var ok bool

	ok, err = path.IsDir()
	if err != nil {
		return nil, fmt.Errorf("failed to check if %q is a directory: %w", path, err)
	}

	if ok {
		return []fspath.Path{path}, nil
	}

	path, err = fspath.NewAbs(home, "Application Support", defaultPrefix, "plugins")
	if err != nil {
		return nil, fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
	}

	return []fspath.Path{path}, nil
}
