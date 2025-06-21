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

//go:build windows

package config

import (
	"fmt"
	"os"

	"github.com/reginald-project/reginald/internal/fspath"
)

func defaultPlatformLogFile() (fspath.Path, error) {
	if env := os.Getenv("XDG_STATE_HOME"); env != "" {
		path, err := fspath.NewAbs(env, defaultFileName, defaultLogFileName)
		if err != nil {
			return "", fmt.Errorf("failed to convert log file to absolute path: %w", err)
		}

		return path, nil
	}

	path, err := fspath.NewAbs("%LOCALAPPDATA%", defaultFileName, defaultLogFileName)
	if err != nil {
		return "", fmt.Errorf(
			"failed to convert Windows plugins directory to absolute path: %w",
			err,
		)
	}

	return path, nil
}

func defaultPlatformPluginPaths() ([]fspath.Path, error) {
	if env := os.Getenv("XDG_DATA_HOME"); env != "" {
		path, err := fspath.NewAbs(env, defaultFileName, "plugins")
		if err != nil {
			return nil, fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
		}

		return []fspath.Path{path}, nil
	}

	path, err := fspath.NewAbs("%LOCALAPPDATA%", defaultFileName, "plugins")
	if err != nil {
		return nil, fmt.Errorf(
			"failed to convert Windows plugins directory to absolute path: %w",
			err,
		)
	}

	return []fspath.Path{path}, nil
}
