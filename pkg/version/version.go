// Package version provides version information of the current binary. Usually
// the version information is set during build time but the package provides a
// fallback value as a default.
package version

import (
	"runtime/debug"
	"strings"

	"github.com/anttikivi/go-semver"
)

// buildVersion is the version number set at build.
var buildVersion = "dev" //nolint:gochecknoglobals // set at build time

// Version is the parsed version number of Reginald.
var version *semver.Version

func init() { //nolint:gochecknoinits // version must be parsed once at the start
	if buildVersion == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			v := info.Main.Version
			i := strings.IndexByte(v, '-')
			v = v[:i+1] + "0.invalid." + v[i+1:]
			version = semver.MustParsePrefix(v, "v")

			return
		}
	}

	version = semver.MustParse(buildVersion)
}

// BuildVersion returns the version string for the program set during the build.
func BuildVersion() string {
	return buildVersion
}

// Revision returns the version control revision this program was built from.
func Revision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		revision := ""
		dirty := ""

		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}

			if s.Key == "vcs.modified" && s.Value == "true" {
				dirty = "-dirty"
			}

			if revision != "" && dirty != "" {
				break
			}
		}

		s := revision + dirty
		if s != "" {
			return s
		}

		return "no-vcs"
	}

	return "no-buildinfo"
}

// Version returns the version number of the program.
func Version() *semver.Version {
	return version
}
