// Package version provides version information of the current binary. Usually
// the version information is set during build time but the package provides a
// fallback value as a default.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/anttikivi/go-semver"
)

// The default values for the version info variables that should be set during
// build.
const (
	defaultBuildVersion = "invalid"
	defaultBuildCommit
	defaultBuildTime
)

// Version and build information set during the build.
//
//nolint:gochecknoglobals
var (
	buildVersion = defaultBuildVersion // version number of the binary
	buildCommit  = defaultBuildCommit  // sha of the build commit
	buildTime    = defaultBuildTime    // time of the build
)

// Version is the parsed version number of Reginald.
var version *semver.Version

func init() {
	if buildVersion == defaultBuildVersion {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			v := info.Main.Version
			i := strings.IndexByte(v, '-')
			v = v[:i+1] + "0.dev.gobuild." + v[i+1:]
			version = semver.MustParsePrefix(v, "v")

			return
		}
	}

	version = semver.MustParse(buildVersion)
}

func BuildVersion() string {
	return buildVersion
}

func BuildCommit() string {
	return buildCommit
}

func BuildTime() time.Time {
	t, err := time.Parse(time.RFC3339, buildTime)
	if err != nil {
		panic(fmt.Sprintf("failed to parse build time: %v", err))
	}

	return t
}

func Version() *semver.Version {
	return version
}
