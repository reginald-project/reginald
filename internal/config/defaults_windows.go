//go:build windows

package config

import (
	"fmt"
	"path/filepath"

	"github.com/anttikivi/reginald/internal/pathname"
)

func defaultPluginsDir() (string, error) {
	path := filepath.Join("%LOCALAPPDATA%", defaultDirName, "plugins")

	path, err := pathname.Abs(path)
	if err != nil {
		return "", fmt.Errorf(
			"failed to convert Windows plugins directory to absolute path: %w",
			err,
		)
	}

	return path, nil
}
