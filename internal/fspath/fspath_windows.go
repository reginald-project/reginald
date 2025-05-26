//go:build windows

package fspath

import (
	"os"
	"strings"
)

// expandOSEnv replaces %var%, ${var}, or $var in the string according to the
// values of the current environment variables. References to undefined
// variables are replaced by the empty string.
func expandOSEnv(path Path) Path {
	if strings.Contains(string(path), "%") {
		path = expandWinEnv(path)
	}

	path = Path(os.ExpandEnv(string(path)))

	return path
}

func expandWinEnv(s Path) Path {
	var buf []byte
	// %% is all ASCII, so bytes are fine for this operation.
	i := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '%' && j+1 < len(s) {
			if buf == nil {
				buf = make([]byte, 0, 2*len(s))
			}

			buf = append(buf, s[i:j]...)

			var k int
			for k = j + 1; k < len(s) && isAlphaNum(s[k]); k++ {
			}

			if k == j+1 {
				buf = append(buf, '%')
			} else if s[k] == '%' {
				buf = append(buf, os.Getenv(string(s[j+1:k]))...)
			} else {
				buf = append(buf, s[j:k+1]...)
			}

			j = k + 1
			i = j
		}
	}

	if buf == nil {
		return s
	}

	return Path(buf) + s[i:]
}

// isAlphaNum reports whether the byte is an ASCII letter, number, or
// underscore.
func isAlphaNum(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
