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

// Abs returns an absolute representation of path. Relative paths will be joined
// with the current working directory. Abs calls Clean on the result. Abs also
// resolves user home directories and environment variables.
func Abs(path string) (string, error) {
	path = ExpandEnv(path)

	var err error

	path, err = ExpandUser(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand user home directory: %w", err)
	}

	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return path, nil
}

// ExpandEnv replaces ${var} or $var and even %var% on Windows in the string
// according to the values of the current environment variables. References to
// undefined variables are replaced by an empty string.
func ExpandEnv(path string) string {
	return expandOSEnv(path)
}

// ExpandUser tries to replace "~" or "~username" in the string to match the
// correspending user's home directory. If the wanted user does not exist, this
// function returns an error.
func ExpandUser(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get the user home dir: %w", err)
		}

		return home, nil
	}

	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get the user home dir: %w", err)
		}

		// Using the current user's home directory.
		if path[1] == '/' || path[1] == os.PathSeparator {
			return filepath.Join(home, path[1:]), nil
		}

		path, err = expandOtherUser(path)
		if err != nil {
			return "", fmt.Errorf("%w", err)
		}
	}

	return path, nil
}

// IsFile reports whether the file name exists and is a file.
func IsFile(name string) (bool, error) {
	info, err := os.Stat(name)
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

// expandOtherUser tries to replace "~username" in path to match the
// correspending user's home directory. If the wanted user does not exist, this
// function returns an error.
func expandOtherUser(path string) (string, error) {
	var (
		i        int
		username string
	)

	if i = strings.IndexByte(path, os.PathSeparator); i != -1 {
		username = path[1:i]
	} else if i = strings.IndexByte(path, '/'); i != -1 {
		username = path[1:i]
	} else {
		username = path[1:]
	}

	u, err := user.Lookup(username)
	if err != nil {
		return "", fmt.Errorf("failed to look up user %q: %w", username, err)
	}

	if i == -1 {
		return u.HomeDir, nil
	}

	return filepath.Join(u.HomeDir, path[i:]), nil
}
