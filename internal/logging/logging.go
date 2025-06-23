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

// Package logging defines utilities for logging within Reginald. The program
// uses the [log/slog] package for logging, and this package contains the
// function for setting up the logging.
//
// At the first phase before parsing the configuration, logging is done using
// the bootstrap logger that is set as the default logger first. After the
// bootstrapping, the default logger should be replaced with the actual logger
// that is set up according to the user's configuration. The bootstrap logger is
// configured using environment variables if customizing it is needed.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/fspath"
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

// BootstrapWriter is the writer used by the bootstrap logger. It is global so
// that in case of errors the final handler of the error can check if its type
// is [BufferedFileWriter] and flush its contents to the given file if that is
// the case.
var BootstrapWriter io.Writer //nolint:gochecknoglobals // needed by the panic handler

// Errors for logging.
var (
	errInvalidFormat = errors.New("given log format not supported")
)

// Config contains the configuration options for logging.
type Config struct {
	Format  string     `mapstructure:"format"`                    // format of the logs, "json" or "text"
	Output  string     `mapstructure:"output"`                    // destination of the logs
	Level   logs.Level `mapstructure:"level"`                     // logging level
	Enabled bool       `flag:"log,no-log" mapstructure:"enabled"` // whether logging is enabled
}

// InitBootstrap initializes the bootstrap logger and sets it as the default
// logger in [log/slog].
func InitBootstrap() error {
	// TODO: Document this: logs are printed when `REGINALD_DEBUG` is set to `1`
	// or `true`, the logs are buffered when no value is given, and the logs are
	// explicitly discarded when `REGINALD_DEBUG` is `0` or `false`.
	debugVar := os.Getenv("REGINALD_DEBUG")
	debugVar = strings.ToLower(debugVar)

	if debugVar == "false" || debugVar == "0" {
		slog.SetDefault(slog.New(slog.DiscardHandler))

		return nil
	}

	if debugVar == "" || (debugVar != "true" && debugVar != "1") {
		// TODO: Come up with a reasonable default resolving maybe using
		// `XDG_CACHE_HOME` and some other directory on Windows.
		path, err := fspath.New("~/.cache/reginald/bootstrap.log").Abs()
		if err != nil {
			return fmt.Errorf("failed to create path to bootstrap log file: %w", err)
		}

		BootstrapWriter = NewBufferedFileWriter(path)

		slog.SetDefault(
			slog.New(
				slog.NewJSONHandler(
					BootstrapWriter,
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

	slog.SetDefault(
		slog.New(
			slog.NewTextHandler(
				terminal.NewWriter(terminal.Default(), terminal.Stderr),
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

// Init initializes the proper logger of the program and sets it as the default
// logger in [log/slog].
func Init(cfg Config) error {
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

// Trace calls [log] with level set to trace on the default logger.
func Trace(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), logs.LevelTrace, msg, args...)
}

// Debug calls [log] with level set to debug on the default logger.
func Debug(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), logs.LevelDebug, msg, args...)
}

// Info calls [log] with level set to info on the default logger.
func Info(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), logs.LevelInfo, msg, args...)
}

// Warn calls [log] with level set to warn on the default logger.
func Warn(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), logs.LevelWarn, msg, args...)
}

// Error calls [log] with level set to error on the default logger.
func Error(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), logs.LevelError, msg, args...)
}

// log is the low-level logging method for methods that take ...any. It must
// always be called directly by an exported logging method or function, because
// it uses a fixed call depth to obtain the pc.
func log(ctx context.Context, l *slog.Logger, level logs.Level, msg string, args ...any) {
	if !l.Enabled(ctx, slog.Level(level)) {
		return
	}

	var pc uintptr

	if !ignorePC {
		var pcs [1]uintptr

		runtime.Callers(logCallerDepth, pcs[:])

		pc = pcs[0]
	}

	r := slog.NewRecord(time.Now(), slog.Level(level), msg, pc)

	r.Add(args...)

	if ctx == nil {
		panic("logging context is nil")
	}

	if err := l.Handler().Handle(ctx, r); err != nil {
		// TODO: Handle this better.
		panic(err)
	}
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
