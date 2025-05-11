// Package version provides version information of the current binary. Usually
// the version information is set during build time but the package provides a
// fallback value as a default.
package version

// The default values for the version info variables that should be set during
// build.
const (
	defaultVersion = "invalid"
	defaultCommitSHA
	defaultBuildTime
)

// Version and build information set during the build.
//
//nolint:gochecknoglobals
var (
	Version   = defaultVersion   // version number of the binary
	CommitSHA = defaultCommitSHA // sha of the build commit
	BuildTime = defaultBuildTime // time of the build
)
