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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/anttikivi/reginald/internal/config"
	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/pathname"
)

// Default values for the logger.
const (
	defaultFilePerm       os.FileMode = 0o644                              // log file permissions
	defaultJSONTimeFormat             = "2006-01-02T15:04:05.000000-07:00" // time format for JSON output
	defaultTextTimeFormat             = "2006-01-02 15:04:05"              // time format for text output
	defaultTimeFormat                 = "2006-01-02T15:04:05.000-07:00"    // default time format in Go
)

// BootstrapWriter is the writer used by the bootstrap logger. It is global so
// that in case of errors the final handler of the error can check if its type
// is [BufferedFileWriter] and flush its contents to the given file if that is
// the case.
var BootstrapWriter io.Writer //nolint:gochecknoglobals

// Errors for logging.
var (
	errInvalidFormat = errors.New("given log format not supported")
)

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
		path, err := pathname.Abs("~/.cache/reginald/bootstrap.log")
		if err != nil {
			return fmt.Errorf("failed to create path to bootstrap log file: %w", err)
		}

		BootstrapWriter = NewBufferedFileWriter(path)

		slog.SetDefault(
			slog.New(
				slog.NewJSONHandler(
					BootstrapWriter,
					&slog.HandlerOptions{ //nolint:exhaustruct
						AddSource: true,
						Level:     slog.LevelDebug,
					},
				),
			),
		)

		return nil
	}

	//nolint:exhaustruct
	slog.SetDefault(
		slog.New(
			slog.NewTextHandler(
				iostreams.NewLockedWriter(os.Stderr),
				&slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug},
			),
		),
	)

	return nil
}

// Init initializes the proper logger of the program and sets it as the default
// logger in [log/slog].
func Init(cfg config.LoggingConfig) error {
	if !cfg.Enabled {
		slog.SetDefault(slog.New(slog.DiscardHandler))

		return nil
	}

	var w io.Writer

	switch strings.ToLower(cfg.Output) {
	case "stderr":
		w = iostreams.NewLockedWriter(os.Stderr)
	case "stdout":
		w = iostreams.NewLockedWriter(os.Stdout)
	default:
		fw, err := os.OpenFile(cfg.Output, os.O_WRONLY|os.O_APPEND|os.O_CREATE, defaultFilePerm)
		if err != nil {
			return fmt.Errorf("failed to open log file at %s: %w", cfg.Output, err)
		}

		w = fw
	}

	timeFormat := defaultJSONTimeFormat
	if strings.ToLower(cfg.Format) == "text" {
		timeFormat = defaultTextTimeFormat
	}

	opts := &slog.HandlerOptions{ //nolint:exhaustruct
		Level: cfg.Level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, a.Value.Time().Format(timeFormat))
			}

			return a
		},
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
