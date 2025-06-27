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

// Package log defines utilities for logging within Reginald. The program uses
// the [log/slog] package for logging, and this package contains the function
// for setting up the logging.
package log

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/reginald-project/reginald-sdk-go/logs"
)

// logCallerDepth is the depth of the stack trace to skip when logging.
// The skipped stack is [runtime.Callers, the function, the function's caller].
const logCallerDepth = 3

// IgnorePC controls whether to invoke runtime.Callers to get the pc in
// the logging functions. This is solely for making the logging function
// analogous with the logging functions in the standard library.
var ignorePC = false //nolint:gochecknoglobals // can be set at compile time

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
