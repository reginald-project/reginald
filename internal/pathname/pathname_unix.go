//go:build !windows

package pathname

import "os"

// expandOSEnv replaces ${var} or $var in the string according to the values of
// the current environment variables. References to undefined variables are
// replaced by empty string.
func expandOSEnv(path string) string {
	return os.ExpandEnv(path)
}
