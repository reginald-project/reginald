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

//go:build linux

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

type fileID struct {
	dev uint64 //nolint:unused // field is used for comparison and hashing
	ino uint64 //nolint:unused // field is used for comparison and hashing
}

func createID(path string) (FileID, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileID{}, fmt.Errorf("failed to get info for %q: %w", path, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileID{}, fmt.Errorf("%w: %q", errSysStat, info.Name())
	}

	return FileID{
		fileID{
			dev: stat.Dev,
			ino: stat.Ino,
		},
	}, nil
}
