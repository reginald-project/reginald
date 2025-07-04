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

// Package logger controls the default logger of Reginald.
package logger

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/terminal"
)

// Default values for the logger.
const (
	defaultFilePerm       os.FileMode = 0o600                              // log file permissions
	defaultDirPerm        os.FileMode = 0o700                              // log directory permissions
	defaultJSONTimeFormat             = "2006-01-02T15:04:05.000000-07:00" // time format for JSON output
	defaultTextTimeFormat             = time.DateTime                      // time format for text output
	defaultTimeFormat                 = "2006-01-02T15:04:05.000-07:00"    // default time format in Go
)

// errInvalidFormat is returned when trying to create a logger with an invalid
// format.
var errInvalidFormat = errors.New("invalid log format")

// Init initializes the proper logger of the program and sets it as the default
// logger in [log/slog].
func Init(cfg Config, debug bool) error {
	opts := &slog.HandlerOptions{
		AddSource:   false, // adding the source is done with the custom handler
		Level:       cfg.Level,
		ReplaceAttr: replaceAttr,
	}

	if debug {
		opts.Level = LevelTrace
		slog.SetDefault(slog.New(newHandler(slog.NewJSONHandler(os.Stdout, opts))))

		return nil
	}

	if !cfg.Enabled {
		slog.SetDefault(slog.New(slog.DiscardHandler))

		return nil
	}

	var w io.Writer

	switch strings.ToLower(cfg.Output) {
	case "stderr":
		w = terminal.NewWriter(terminal.Default(), terminal.Stderr)
	case "stdout":
		w = terminal.NewWriter(terminal.Default(), terminal.Stdout)
	default:
		path := fspath.Path(cfg.Output)

		err := os.MkdirAll(string(path.Dir()), defaultDirPerm)
		if err != nil {
			return fmt.Errorf("failed to create directory %q for log output: %w", path.Dir(), err)
		}

		fw, err := os.OpenFile(path.String(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, defaultFilePerm)
		if err != nil {
			return fmt.Errorf("failed to open log file at %s: %w", path.String(), err)
		}

		w = fw
	}

	var h slog.Handler

	switch strings.ToLower(cfg.Format) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	case "text":
		h = slog.NewTextHandler(w, opts)
	default:
		return fmt.Errorf("%w: %s", errInvalidFormat, cfg.Format)
	}

	slog.SetDefault(slog.New(newHandler(h)))

	return nil
}

func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.SourceKey {
		src, ok := a.Value.Any().(*slog.Source)
		// Make a guess whether this is a duplicate source attribute in
		// the logging messages from the plugins.
		if !ok || src == nil || src.Line == 0 {
			return slog.Attr{} //nolint:exhaustruct // empty return value
		}
	}

	if a.Key == slog.LevelKey {
		level, ok := a.Value.Any().(slog.Level)
		if !ok {
			panic(fmt.Sprintf("failed to convert level value to slog.Level: %[1]v (%[1]T)", a.Value.Any()))
		}

		return slog.String(slog.LevelKey, Level(level).String())
	}

	return a
}
