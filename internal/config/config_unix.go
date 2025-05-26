//go:build !windows

package config

import (
	"fmt"
	"os"

	"github.com/anttikivi/reginald/internal/fspath"
)

func defaultPluginsDir() (fspath.Path, error) {
	if env := os.Getenv("XDG_DATA_HOME"); env != "" {
		path, err := fspath.NewAbs(env, defaultFileName, "plugins")
		if err != nil {
			return "", fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
		}

		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get the user home directory: %w", err)
	}

	path, err := fspath.NewAbs(home, ".local", "share", defaultFileName, "plugins")
	if err != nil {
		return "", fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
	}

	return path, nil
}
