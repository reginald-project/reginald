//go:build windows

package config

import (
	"fmt"

	"github.com/anttikivi/reginald/internal/fspath"
)

func defaultPluginsDir() (fspath.Path, error) {
	path, err := fspath.NewAbs("%LOCALAPPDATA%", defaultFileName, "plugins")
	if err != nil {
		return "", fmt.Errorf(
			"failed to convert Windows plugins directory to absolute path: %w",
			err,
		)
	}

	return path, nil
}
