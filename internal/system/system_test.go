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

package system_test

import (
	"runtime"
	"testing"

	"github.com/reginald-project/reginald/internal/system"
)

func TestOSCurrent(t *testing.T) {
	t.Parallel()

	type testCase struct {
		input system.OS
		want  bool
	}

	var (
		id     string //nolint:varnamelen
		idLike []string
		err    error
	)

	if runtime.GOOS == "linux" {
		id, idLike, err = system.OSRelease()
	}

	if err != nil {
		t.Fatal(err)
	}

	tests := map[string][]testCase{
		"linux": {
			{
				"unix",
				true,
			},
			{
				"not-real",
				false,
			},
			{
				"linux",
				true,
			},
			{
				system.OS(id),
				true,
			},
			{
				"darwin",
				false,
			},
			{
				"macos",
				false,
			},
			{
				"osx",
				false,
			},
			{
				"windows",
				false,
			},
		},
		"darwin": {
			{
				"unix",
				true,
			},
			{
				"not-real",
				false,
			},
			{
				"darwin",
				true,
			},
			{
				"macos",
				true,
			},
			{
				"osx",
				true,
			},
			{
				"linux",
				false,
			},
			{
				"ubuntu",
				false,
			},
			{
				"debian",
				false,
			},
			{
				"windows",
				false,
			},
		},
		"windows": {
			{
				"unix",
				false,
			},
			{
				"not-real",
				false,
			},
			{
				"linux",
				false,
			},
			{
				system.OS(id),
				false,
			},
			{
				"darwin",
				false,
			},
			{
				"macos",
				false,
			},
			{
				"osx",
				false,
			},
			{
				"windows",
				true,
			},
		},
	}

	ts, ok := tests[runtime.GOOS]
	if !ok {
		return
	}

	if runtime.GOOS == "linux" {
		for _, l := range idLike {
			ts = append(ts, testCase{
				input: system.OS(l),
				want:  true,
			})
		}
	}

	for _, tt := range ts {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()

			got := tt.input.Current()
			if got != tt.want {
				t.Errorf("OS(%s) = %t, want %t", tt.input, got, tt.want)
			}
		})
	}
}
