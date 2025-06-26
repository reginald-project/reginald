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

// Package fsutil implements basic utility routines for interacting with the files and file system.
package fsutil

import "errors"

// errSysStat is returned when the file info cannot be converted to the system
// file stat type.
var errSysStat = errors.New("could not get system stat info")

// A FileID is a unique identifier for a file that maybe used as a value. It is
// a hashable alternative for [os.FileInfo].
type FileID struct {
	fileID //nolint:unused // field is used for comparison and hashing
}

// ID creates a new platform-specific FileID from the given [os.FileInfo].
func ID(path string) (FileID, error) {
	return createID(path)
}
