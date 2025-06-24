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

// Package logger controls the default logger of Reginald. It is a separate
// package to avoid import cycles.
package logger

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/debugging"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log/logconfig"
	"github.com/reginald-project/reginald/internal/log/logwriter"
	"github.com/reginald-project/reginald/internal/terminal"
)

// Default values for the logger.
const (
	defaultFilePerm       os.FileMode = 0o644                              // log file permissions
	defaultDirPerm        os.FileMode = 0o755                              // log directory permissions
	defaultJSONTimeFormat             = "2006-01-02T15:04:05.000000-07:00" // time format for JSON output
	defaultTextTimeFormat             = time.DateTime                      // time format for text output
	defaultTimeFormat                 = "2006-01-02T15:04:05.000-07:00"    // default time format in Go
)

// Errors for logger.
var (
	errInvalidFormat = errors.New("given log format not supported")
)

// InitBootstrap initializes the bootstrap logger and sets it as the default
// logger in [log/slog].
func InitBootstrap() error {
	// The logger is set to discard during checking if the debug mode is
	// enabled.
	slog.SetDefault(slog.New(slog.DiscardHandler))

	isDebug := debugging.IsDebug()

	if !isDebug {
		// TODO: Come up with a reasonable default resolving maybe using
		// `XDG_CACHE_HOME` and some other directory on Windows.
		path, err := fspath.New("~/.cache/reginald/bootstrap.log").Abs()
		if err != nil {
			return fmt.Errorf("failed to create path to bootstrap log file: %w", err)
		}

		logwriter.BootstrapWriter = logwriter.NewBufferedFileWriter(path)

		slog.SetDefault(
			slog.New(
				slog.NewJSONHandler(
					logwriter.BootstrapWriter,
					&slog.HandlerOptions{
						AddSource:   true,
						Level:       logs.LevelTrace,
						ReplaceAttr: replaceAttrFunc(""),
					},
				),
			),
		)

		return nil
	}

	slog.SetDefault(slog.New(debugHandler()).With("bootstrap", "true"))

	return nil
}

// Init initializes the proper logger of the program and sets it as the default
// logger in [log/slog].
func Init(cfg logconfig.Config) error {
	if debugging.IsDebug() {
		slog.SetDefault(slog.New(debugHandler()))

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

		err := path.Dir().MkdirAll(defaultDirPerm)
		if err != nil {
			return fmt.Errorf("failed to create directory %q for log output: %w", path.Dir(), err)
		}

		fw, err := os.OpenFile(path.String(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, defaultFilePerm)
		if err != nil {
			return fmt.Errorf("failed to open log file at %s: %w", path.String(), err)
		}

		w = fw
	}

	opts := &slog.HandlerOptions{
		AddSource:   true,
		Level:       cfg.Level,
		ReplaceAttr: replaceAttrFunc(""),
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

	slog.SetDefault(slog.New(h))

	return nil
}

// debugHandler returns a handler that should be used when debugging is enabled.
func debugHandler() slog.Handler {
	return slog.NewTextHandler(
		terminal.NewWriter(terminal.Default(), terminal.Stderr),
		&slog.HandlerOptions{
			AddSource:   true,
			Level:       logs.LevelTrace,
			ReplaceAttr: replaceAttrFunc(""),
		},
	)
}

func replaceAttrFunc(timeFormat string) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if timeFormat != "" && a.Key == slog.TimeKey {
			return slog.String(slog.TimeKey, a.Value.Time().Format(timeFormat))
		}

		if a.Key == slog.LevelKey {
			level, ok := a.Value.Any().(slog.Level)
			if !ok {
				panic(
					fmt.Sprintf(
						"failed to convert level value to slog.Level: %[1]v (%[1]T)",
						a.Value.Any(),
					),
				)
			}

			return slog.String(slog.LevelKey, logs.Level(level).String())
		}

		return a
	}
}
