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

// Package logs defines the public types used in Reginald in logging.
package logs

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Names for common levels.
const (
	LevelTrace Level = -8
	LevelDebug       = Level(slog.LevelDebug)
	LevelInfo        = Level(slog.LevelInfo)
	LevelWarn        = Level(slog.LevelWarn)
	LevelError       = Level(slog.LevelError)
)

// Errors for the log utilities.
var (
	errUnknownName = errors.New("level has unknown name")
)

// A Level is the importance or severity of a log event. The higher the level,
// the more important or severe the event.
type Level slog.Level //nolint:recvcheck // TODO: Can the receivers have the same type?

// Level returns the [slog.Level] for l.
func (l Level) Level() slog.Level {
	return slog.Level(l)
}

// String returns a name for the level. If the level has a name, then that name
// in uppercase is returned. If the level is between named values, then
// an integer is appended to the uppercased name.
func (l Level) String() string {
	str := func(base string, val Level) string {
		if val == 0 {
			return base
		}

		return fmt.Sprintf("%s%+d", base, val)
	}

	switch {
	case l < LevelDebug:
		return str("TRACE", l-LevelTrace)
	case l < LevelInfo:
		return str("DEBUG", l-LevelDebug)
	case l < LevelWarn:
		return str("INFO", l-LevelInfo)
	case l < LevelError:
		return str("WARN", l-LevelWarn)
	default:
		return str("ERROR", l-LevelError)
	}
}

// MarshalJSON implements [encoding/json.Marshaler] by quoting the output of
// [Level.String].
func (l Level) MarshalJSON() ([]byte, error) { //nolint:unparam // implements interface
	return strconv.AppendQuote(nil, l.String()), nil
}

// UnmarshalJSON implements [encoding/json.Unmarshaler] It accepts any string
// produced by [Level.MarshalJSON], ignoring case. It also accepts numeric
// offsets that would result in a different string on output. For example,
// "Error-8" would marshal as "INFO".
func (l *Level) UnmarshalJSON(data []byte) error {
	s, err := strconv.Unquote(string(data))
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	return l.parse(s)
}

// AppendText implements [encoding.TextAppender] by calling [Level.String].
func (l Level) AppendText(b []byte) ([]byte, error) {
	return append(b, l.String()...), nil
}

// MarshalText implements [encoding.TextMarshaler] by calling [Level.AppendText].
func (l Level) MarshalText() ([]byte, error) {
	return l.AppendText(nil)
}

// UnmarshalText implements [encoding.TextUnmarshaler]. It accepts any string
// produced by [Level.MarshalText], ignoring case. It also accepts numeric
// offsets that would result in a different string on output. For example,
// "Error-8" would marshal as "INFO".
func (l *Level) UnmarshalText(data []byte) error {
	return l.parse(string(data))
}

func (l *Level) parse(s string) error {
	name := s
	offset := 0

	if i := strings.IndexAny(s, "+-"); i >= 0 {
		name = s[:i]

		var err error

		offset, err = strconv.Atoi(s[i:])
		if err != nil {
			return fmt.Errorf("logs: level string %q: %w", s, err)
		}
	}

	switch strings.ToUpper(name) {
	case "TRACE":
		*l = LevelTrace
	case "DEBUG":
		*l = LevelDebug
	case "INFO":
		*l = LevelInfo
	case "WARN":
		*l = LevelWarn
	case "ERROR":
		*l = LevelError
	default:
		return fmt.Errorf("%w: %s", errUnknownName, name)
	}

	*l += Level(offset)

	return nil
}
