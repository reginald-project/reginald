// Copyright 2025 The Reginald Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package version provides version information of the current binary. Usually
// the version information is set during build time but the package provides a
// fallback value as a default.
package version

import (
	"runtime/debug"
	"strings"
	"sync"

	"github.com/anttikivi/semver"
)

// buildVersion is the version number set at build.
var buildVersion = "dev" //nolint:gochecknoglobals // set at build time

// Version is the parsed version number of Reginald.
var version *semver.Version

// initOnce is used to ensure that the global version is initialized only once.
var initOnce sync.Once //nolint:gochecknoglobals // must be global to persist

// Init initializes the version. It should be called from the init function of
// the main package and it takes the information embedded from the version file
// as a parameter. The data read from the version file will be used to create
// the version if no version information was supplied during build time.
func Init(versionFile string) {
	initOnce.Do(func() {
		if buildVersion == "dev" {
			info, ok := debug.ReadBuildInfo()
			if !ok {
				panic("cannot get build info")
			}

			v := info.Main.Version
			if v == "(devel)" {
				v = versionFile + "-0.invalid." + Revision()
			} else {
				i := strings.IndexByte(v, '-')
				v = v[:i+1] + "0.invalid." + v[i+1:]
			}

			version = semver.MustParse(v)

			return
		}

		version = semver.MustParse(buildVersion)
	})
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
