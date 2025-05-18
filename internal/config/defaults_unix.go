//go:build !windows

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anttikivi/reginald/internal/pathname"
)

func defaultPluginsDir() (string, error) {
	if env := os.Getenv("XDG_DATA_HOME"); env != "" {
		path := filepath.Join(env, defaultFileName, "plugins")

		path, err := pathname.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
		}

		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get the user home directory: %w", err)
	}

	path := filepath.Join(home, ".local", "share", defaultFileName, "plugins")

	path, err = pathname.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to convert plugins directory to absolute path: %w", err)
	}

	return path, nil
}
