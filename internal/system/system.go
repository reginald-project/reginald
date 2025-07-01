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

// Package system provides utilities for working with different platforms and
// operating systems.
package system

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/reginald-project/reginald/internal/fspath"
)

// Linux is the name of the Linux GOOS for convenience.
const Linux = "linux"

// errNoID is returned by osRelease when it cannot find ID.
var errNoID = errors.New("no OS ID")

// An OS represents an operating system that Reginald can run on.
type OS string

// OSes is a list of operating systems for convenience. As there are config
// options that define a list of OSes but can accept a single string as their
// value, OSes can be unmarshaled from a single string.
type OSes []OS

// Current reports whether o matches the current platform.
func (o OS) Current() bool {
	t := strings.ToLower(strings.TrimSpace(string(o)))
	goos := runtime.GOOS

	switch goos {
	case Linux:
		if t == "unix" {
			return true
		}

		if t == Linux {
			return true
		}

		id, idLike, err := OSRelease()
		if err != nil {
			return false
		}

		if t == id {
			return true
		}

		return slices.Contains(idLike, t)
	case "darwin":
		switch t {
		case "unix", "darwin", "macos", "osx":
			return true
		default:
			return false
		}
	case "windows":
		return t == "windows"
	default:
		return t == goos
	}
}

// String returns the string representation of the platform.
func (o OS) String() string {
	return string(o)
}

// UnmarshalText implements [encoding.TextUnmarshaler]. It decodes a single
// string into a slice of Platforms.
func (o *OSes) UnmarshalText(data []byte) error { //nolint:unparam // implements interface
	if len(data) == 0 {
		*o = make(OSes, 0)

		return nil
	}

	parts := strings.Split(string(data), ",")
	out := make(OSes, len(parts))

	for i, s := range parts {
		out[i] = OS(s)
	}

	*o = out

	return nil
}

// OSRelease detects the Linux OS type or distribution. It returns the ID,
// the ID_LIKE, and any encountered error.
func OSRelease() (string, []string, error) {
	id, idLike, err := checkOSRelease(fspath.Path("/etc/os-release"))
	if err != nil {
		id, idLike, err = checkOSRelease(fspath.Path("/usr/lib/os-release"))
	}

	if err != nil {
		return "", nil, err
	}

	return id, idLike, nil
}

// This returns the current operating system.
func This() OS {
	goos := runtime.GOOS
	if goos != Linux {
		return OS(goos)
	}

	id, _, err := OSRelease()
	if err != nil || id == "" {
		return OS(goos)
	}

	return OS(id)
}

// checkOSRelease detects the Linux OS type or distribution from the given
// os-release file. It returns the ID, the ID_LIKE, and any encountered error.
func checkOSRelease(path fspath.Path) (string, []string, error) {
	f, err := os.Open(string(path))
	if err != nil {
		return "", nil, fmt.Errorf("failed to open %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // no need to check

	scanner := bufio.NewScanner(f)

	var (
		id     string
		idLike []string
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = strings.ToLower(strings.Trim(line[len("ID="):], "\""))
		} else if strings.HasPrefix(line, "ID_LIKE=") {
			idLike = append(idLike, strings.Fields(strings.ToLower(strings.Trim(line[len("ID_LIKE="):], "\"")))...)
		}
	}

	if err = scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("failed to scan %q: %w", path, err)
	}

	if id == "" {
		return "", nil, fmt.Errorf("%w: %s", errNoID, path)
	}

	return id, idLike, nil
}
