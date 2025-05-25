// Package pathname contains generic filepath-related utilities that complement
// the standard library [path/filepath].
package pathname

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// A Path is a file system path.
type Path string

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
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get the user home dir: %w", err)
		}

		return Path(home), nil
	}

	if strings.HasPrefix(string(p), "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get the user home dir: %w", err)
		}

		// Using the current user's home directory.
		if p[1] == '/' || p[1] == os.PathSeparator {
			return Path(filepath.Join(home, string(p[1:]))), nil
		}

		p, err = expandOtherUser(p)
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}
	}

	return p, nil
}

// IsFile reports whether the file name exists and is a file.
func (p Path) IsFile() (bool, error) {
	info, err := os.Stat(string(p))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("%w", err)
	}

	if info.IsDir() {
		return false, nil
	}

	return true, nil
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

	return Path(filepath.Join(u.HomeDir, string(path[i:]))), nil
}
