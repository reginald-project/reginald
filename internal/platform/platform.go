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

// Package platform provides utilities for working with different platforms and
// operating systems.
package platform

import "strings"

// A Platform represents a platform that Reginald can run on.
type Platform string

// Platforms is a list of platforms for convenience. As there are config options
// that define a list of Platforms but can accept a single string as their
// value, Platforms can be unmarshaled from a single string.
type Platforms []Platform

// UnmarshalText implements [encoding.TextUnmarshaler]. It decodes a single
// string into a slice of Platforms.
func (p *Platforms) UnmarshalText(data []byte) error { //nolint:unparam // implements interface
	if len(data) == 0 {
		*p = make(Platforms, 0)

		return nil
	}

	parts := strings.Split(string(data), ",")
	out := make(Platforms, len(parts))

	for i, s := range parts {
		out[i] = Platform(s)
	}

	*p = out

	return nil
}
