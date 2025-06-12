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
	"strings"
	"time"

	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/iostreams"
	"github.com/reginald-project/reginald/pkg/logs"
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
				iostreams.NewLockedWriter(os.Stderr),
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
		w = iostreams.NewWriter(iostreams.Streams, iostreams.Stderr)
	case "stdout":
		w = iostreams.NewWriter(iostreams.Streams, iostreams.Stdout)
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

// Trace calls [slog.Logger.Log] with level set to trace on the default logger.
func Trace(msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(context.Background(), logs.LevelTrace.Level(), msg, args...)
}

// TraceContext calls [slog.Logger.Log] with level set to trace on the default
// logger.
func TraceContext(ctx context.Context, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, logs.LevelTrace.Level(), msg, args...)
}

// Debug calls [slog.Logger.Debug] on the default logger.
func Debug(msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(context.Background(), logs.LevelDebug.Level(), msg, args...)
}

// DebugContext calls [slog.Logger.DebugContext] on the default logger.
func DebugContext(ctx context.Context, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, logs.LevelDebug.Level(), msg, args...)
}

// Info calls [slog.Logger.Info] on the default logger.
func Info(msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(context.Background(), logs.LevelInfo.Level(), msg, args...)
}

// InfoContext calls [slog.Logger.InfoContext] on the default logger.
func InfoContext(ctx context.Context, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, logs.LevelInfo.Level(), msg, args...)
}

// Warn calls [slog.Logger.Warn] on the default logger.
func Warn(msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(context.Background(), logs.LevelWarn.Level(), msg, args...)
}

// WarnContext calls [slog.Logger.WarnContext] on the default logger.
func WarnContext(ctx context.Context, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, logs.LevelWarn.Level(), msg, args...)
}

// Error calls [slog.Logger.Error] on the default logger.
func Error(msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(context.Background(), logs.LevelError.Level(), msg, args...)
}

// ErrorContext calls [slog.Logger.ErrorContext] on the default logger.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, logs.LevelError.Level(), msg, args...)
}

// Log calls [slog.Logger.Log] on the default logger.
func Log(ctx context.Context, level logs.Level, msg string, args ...any) {
	//nolint:sloglint // logging function cannot have constant message
	slog.Log(ctx, level.Level(), msg, args...)
}

// LogAttrs calls [slog.Logger.LogAttrs] on the default logger.
func LogAttrs(ctx context.Context, level logs.Level, msg string, attrs ...slog.Attr) {
	//nolint:sloglint // logging function cannot have constant message
	slog.LogAttrs(ctx, level.Level(), msg, attrs...)
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
