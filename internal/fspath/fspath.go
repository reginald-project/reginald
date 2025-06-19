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

// Package fspath implements utility routines for manipulating filename paths in
// a way compatible with the target operating system-defined file paths through
// the [Path] type. It also implements basic utility routines for interacting
// with the files and file system through [Path].
package fspath

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// A Path is a file system path.
type Path string

// New returns a new path by joining the given string using [filepath.Join].
// Clean is called on the result.
func New(elem ...string) Path {
	return Path(filepath.Join(elem...))
}

// NewAbs returns a new path by joining the given string using [filepath.Join]
// and by converting the result to an absolute path. Clean is called on
// the result.
func NewAbs(elem ...string) (Path, error) {
	p, err := New(elem...).Abs()
	if err != nil {
		return "", fmt.Errorf("failed to create Path: %w", err)
	}

	return p, nil
}

// Abs returns an absolute representation of path. Relative paths will be joined
// with the current working directory. Abs calls Clean on the result. Abs also
// resolves user home directories and environment variables.
func (p Path) Abs() (Path, error) {
	p = p.ExpandEnv()

	var err error

	p, err = p.ExpandUser()
	if err != nil {
		return "", fmt.Errorf("failed to expand user home directory: %w", err)
	}

	var absPath string

	absPath, err = filepath.Abs(string(p))
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	p = Path(absPath)

	return p, nil
}

// Base returns the last element of path. Trailing path separators are removed
// before extracting the last element. If the path is empty, Base returns ".".
// If the path consists entirely of separators, Base returns a single separator.
func (p Path) Base() Path {
	return Path(filepath.Base(string(p)))
}

// Clean returns the shortest path name equivalent to path by eliminating
// redundant separators and resolving `.` and `..` elements. It wraps
// [filepath.Clean].
func (p Path) Clean() Path {
	return Path(filepath.Clean(string(p)))
}

// Dir returns all but the last element of path, typically the path's directory.
// After dropping the final element, Dir calls [filepath.Clean] on the path and
// trailing slashes are removed. If the path is empty, Dir returns ".". If
// the path consists entirely of separators, Dir returns a single separator.
// The returned path does not end in a separator unless it is the root
// directory.
func (p Path) Dir() Path {
	return Path(filepath.Dir(string(p)))
}

// ExpandEnv replaces ${var} or $var and even %var% on Windows in the string
// according to the values of the current environment variables. References to
// undefined variables are replaced by an empty string.
func (p Path) ExpandEnv() Path {
	return expandOSEnv(p)
}

// ExpandUser tries to replace "~" or "~username" in the string to match the
// correspending user's home directory. If the wanted user does not exist, this
// function returns an error.
func (p Path) ExpandUser() (Path, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get the user home dir: %w", err)
	}

	if p == "~" {
		return Path(home), nil
	}

	if strings.HasPrefix(string(p), "~") {
		// Using the current user's home directory.
		if p[1] == '/' || p[1] == os.PathSeparator {
			return New(home, string(p[1:])), nil
		}

		p, err = expandOtherUser(p)
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}
	}

	return p, nil
}

// IsAbs reports whether the path is absolute.
func (p Path) IsAbs() bool {
	return filepath.IsAbs(string(p))
}

// IsFile reports whether the file name exists and is a file.
func (p Path) IsFile() (bool, error) {
	info, err := os.Stat(string(p))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("%w", err)
	}

	return !info.IsDir(), nil
}

// Join joins any number of path elements into a single path, starting with
// Path p and separating the elements with an OS specific [os.PathSeparator].
// Empty elements are ignored. The result is Cleaned. However, if the argument
// list is empty or all its elements are empty, Join returns an empty string. On
// Windows, the result will only be a UNC path if the first non-empty element is
// a UNC path.
//
// Join wraps [filepath.Join].
func (p Path) Join(elem ...string) Path {
	all := make([]string, len(elem)+1)
	all[0] = string(p)

	copy(all[1:], elem)

	return Path(filepath.Join(all...))
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm (before
// umask) are used for all directories that MkdirAll creates. If path is already
// a directory, MkdirAll does nothing and returns nil.
//
// MkdirAll wraps [os.MkdirAll].
func (p Path) MkdirAll(perm os.FileMode) error {
	if err := os.MkdirAll(string(p), perm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", p, err)
	}

	return nil
}

// OpenFile opens the named file at p with specified flag (O_RDONLY etc.). If
// the file does not exist, and the O_CREATE flag is passed, it is created with
// mode perm (before umask); the containing directory must exist. If successful,
// methods on the returned File can be used for I/O. If there is an error, it
// will be of type *os.PathError.
//
// OpenFile wraps [os.OpenFile], and the caller must call
// [os.File.Close] on the returned file.
func (p Path) OpenFile(flag int, perm os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(string(p), flag, perm) // #nosec G304 -- utility function
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %w", p, err)
	}

	return f, nil
}

// ReadDir reads the named directory, returning all its directory entries sorted
// by filename.
//
// ReadDir wraps [os.ReadDir].
func (p Path) ReadDir() ([]os.DirEntry, error) {
	list, err := os.ReadDir(string(p))
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return list, nil
}

// ReadFile reads the file at p and returns the contents. A successful call
// returns err == nil, not err == EOF. Because ReadFile reads the whole file, it
// does not treat an EOF from Read as an error to be reported.
//
// ReadFile wraps [os.ReadFile].
func (p Path) ReadFile() ([]byte, error) {
	data, err := os.ReadFile(string(p))
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return data, nil
}

// String returns p as a string and implements [fmt.Stringer] for [Path].
func (p Path) String() string {
	return string(p)
}

// expandOtherUser tries to replace "~username" in path to match the
// correspending user's home directory. If the wanted user does not exist, this
// function returns an error.
func expandOtherUser(path Path) (Path, error) {
	var (
		i        int
		username string
	)

	if i = strings.IndexByte(string(path), os.PathSeparator); i != -1 {
		username = string(path[1:i])
	} else if i = strings.IndexByte(string(path), '/'); i != -1 {
		username = string(path[1:i])
	} else {
		username = string(path[1:])
	}

	u, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("failed to look up user %q: %w", username, err)
	}

	if i == -1 {
		return Path(u.HomeDir), nil
	}

	return New(u.HomeDir, string(path[i:])), nil
}
