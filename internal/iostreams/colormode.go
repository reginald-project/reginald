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

package iostreams

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Possible values for [ColorMode].
const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

// errColorMode is returned when an invalid value is parsed into [ColorMode].
var errColorMode = errors.New("invalid color mode")

// ColorMode represent the color output setting of the program.
type ColorMode int //nolint:recvcheck // needs different receiver types

// String returns the string representation of c.
func (c ColorMode) String() string {
	switch c {
	case ColorAlways:
		return "always"
	case ColorAuto:
		return "auto"
	case ColorNever:
		return "never"
	default:
		return "invalid"
	}
}

// Set sets the value of c from the given string s.
func (c *ColorMode) Set(s string) error {
	switch s = strings.ToLower(s); s {
	case "true", "always", "yes", "1":
		*c = ColorAlways
	case "false", "never", "no", "0":
		*c = ColorNever
	case "auto", "":
		*c = ColorAuto
	default:
		return fmt.Errorf("%w: %q", errColorMode, s)
	}

	return nil
}

// Type returns type of c as a string for command-line flags.
func (*ColorMode) Type() string {
	return "ColorMode"
}

// MarshalJSON encodes c as a JSON value.
func (c ColorMode) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(c.String())
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return data, nil
}

// UnmarshalJSON assign the value from the given JSON representation to c.
func (c *ColorMode) UnmarshalJSON(data []byte) error {
	var (
		err error
		s   string
	)

	if err = json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to unmarshal ColorMode: %w", err)
	}

	if err = c.Set(s); err != nil {
		return fmt.Errorf("failed to set ColorMode: %w", err)
	}

	return nil
}

// MarshalText encodes c in a textual form.
func (c ColorMode) MarshalText() ([]byte, error) { //nolint:unparam // implements interface
	return []byte(c.String()), nil
}

// UnmarshalText assigns the value from the given textual representation to c.
func (c *ColorMode) UnmarshalText(data []byte) error {
	if err := c.Set(string(data)); err != nil {
		return fmt.Errorf("failed to set ColorMode: %w", err)
	}

	return nil
}
