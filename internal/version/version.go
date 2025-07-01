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

	"github.com/anttikivi/semver"
)

// buildVersion is the version number set at build.
var buildVersion = "dev" //nolint:gochecknoglobals // set at build time

// Version is the parsed version number of Reginald.
var version *semver.Version

func init() { //nolint:gochecknoinits // version must be parsed once at the start
	if buildVersion == "dev" {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			panic("cannot get build info")
		}

		v := info.Main.Version
		if v == "(devel)" {
			// TODO: The version number should be read from the file.
			v = "0.1.0-0.invalid." + Revision()
		} else {
			i := strings.IndexByte(v, '-')
			v = v[:i+1] + "0.invalid." + v[i+1:]
		}

		version = semver.MustParse(v)

		return
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
