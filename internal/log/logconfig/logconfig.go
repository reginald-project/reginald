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

// Package logconfig defines the configuration options for the logger. It is
// a separate pckage to avoid import cycles.
package logconfig

import (
	"fmt"

	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/fspath"
)

const (
	defaultPrefix      = "reginald"
	defaultLogFileName = defaultPrefix + ".log"
)

// Config contains the configuration options for the logger.
type Config struct {
	Format  string     `mapstructure:"format"`                    // format of the logs, "json" or "text"
	Output  string     `mapstructure:"output"`                    // destination of the logs
	Level   logs.Level `mapstructure:"level"`                     // logging level
	Enabled bool       `flag:"log,no-log" mapstructure:"enabled"` // whether logging is enabled
}

// Default returns the default logging configuration.
func Default() Config {
	logOutput, err := DefaultLogOutput()
	if err != nil {
		panic(fmt.Sprintf("failed to get the default log output: %v", err))
	}

	return Config{
		Enabled: true,
		Format:  "json",
		Level:   logs.LevelInfo,
		Output:  string(logOutput),
	}
}

// DefaultLogOutput returns the default logging output file to use.
func DefaultLogOutput() (fspath.Path, error) {
	path, err := defaultPlatformLogFile()
	if err != nil {
		return "", err
	}

	return path, nil
}
