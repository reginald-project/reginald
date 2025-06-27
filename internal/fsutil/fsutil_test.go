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

package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reginald-project/reginald/internal/fsutil"
)

func TestID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		setup func(t *testing.T) (path1, path2 string)
		path  string
		want  bool
	}{
		{
			path: "Different files",
			want: false,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				file1 := createTempFile(t, "file1")
				file2 := createTempFile(t, "file2")

				return file1, file2
			},
		},
		{
			path: "Same path",
			want: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				file1 := createTempFile(t, "file1")

				return file1, file1
			},
		},
		{
			path: "Hard link",
			want: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				file1 := createTempFile(t, "file1")
				linkPath := file1 + ".link"
				if err := os.Link(file1, linkPath); err != nil {
					t.Fatalf("Failed to create hard link: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Remove(linkPath); err != nil {
						t.Fatalf("Failed to remove hard: %v", err)
					}
				})

				return file1, linkPath
			},
		},
		{
			path: "Symbolic link",
			want: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				file1 := createTempFile(t, "file1")
				symlinkPath := file1 + ".symlink"
				if err := os.Symlink(file1, symlinkPath); err != nil {
					// if runtime.GOOS == "windows" {
					// 	t.Skipf("Skipping symlink test on Windows: %v", err)
					// }
					t.Fatalf("Failed to create symlink: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Remove(symlinkPath); err != nil {
						t.Fatalf("Failed to remove symlink: %v", err)
					}
				})

				return file1, symlinkPath
			},
		},
		{
			path: "Different directories",
			want: false,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir1 := createTempDir(t, "dir1")
				dir2 := createTempDir(t, "dir2")

				return dir1, dir2
			},
		},
		{
			path: "Same directory path",
			want: true,
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir1 := createTempDir(t, "dir1")

				return dir1, dir1
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			path1, path2 := tt.setup(t)

			// Call our simplified helper which now calls createID directly.
			id1 := createID(t, path1)
			id2 := createID(t, path2)

			got := (id1 == id2)

			if got != tt.want {
				t.Errorf("Got %v, want %v", got, tt.want)
				t.Logf("Path 1: %s -> ID: %+v", path1, id1)
				t.Logf("Path 2: %s -> ID: %+v", path2, id2)
			}
		})
	}
}

func createID(t *testing.T, path string) fsutil.FileID {
	t.Helper()

	id, err := fsutil.ID(path)
	if err != nil {
		t.Fatalf("ID(%q) failed: %v", path, err)
	}

	return id
}

func createTempFile(t *testing.T, name string) string {
	t.Helper()
	dir := createTempDir(t, "file-parent")
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	return path
}

func createTempDir(t *testing.T, name string) string {
	t.Helper()

	parentDir := t.TempDir()
	path := filepath.Join(parentDir, name)

	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	return path
}
