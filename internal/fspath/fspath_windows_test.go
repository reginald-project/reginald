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

//go:build windows

package fspath_test

import (
	"os"
	"strings"
	"testing"

	"github.com/reginald-project/reginald/internal/fspath"
)

func TestAbs(t *testing.T) {
	drive := cwd()[:strings.IndexByte(string(cwd()), ':')+1]
	tests := []struct {
		path    fspath.Path
		env     map[string]string
		want    fspath.Path
		wantErr bool
	}{
		{
			".\\test\\file",
			nil,
			cwd() + "\\test\\file",
			false,
		},
		{
			"\\test\\file",
			nil,
			drive + "\\test\\file",
			false,
		},
		{
			"~\\test\\file",
			nil,
			home() + "\\test\\file",
			false,
		},
		{
			"~dontexist\\test\\file",
			nil,
			"",
			true,
		},
		{
			"~\\$ENVVAR\\file",
			map[string]string{"ENVVAR": "path"},
			home() + "\\path\\file",
			false,
		},
		{
			"~\\$ENVVAR\\${SECOND_VAR}",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home() + "\\path\\file",
			false,
		},
		{
			"\\$ENVVAR\\${SECOND_VAR}",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			drive + "\\path\\file",
			false,
		},
		{
			"~\\",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home(),
			false,
		},
		{
			"~",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home(),
			false,
		},
		{
			"~\\.\\.\\file",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home() + "\\file",
			false,
		},
		{
			"~\\.\\.\\file\\..",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home(),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.path), func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, gotErr := tt.path.Abs()

			if gotErr == nil && tt.wantErr {
				t.Fatal("Abs() succeeded unexpectedly")
			}

			if gotErr != nil && !tt.wantErr {
				t.Errorf("Abs() failed: %v", gotErr)
			}

			if got != tt.want {
				t.Errorf("Abs(%v) = %v, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		path fspath.Path
		env  map[string]string
		want fspath.Path
	}{
		{
			"some/path/%WITHVAR%/here",
			map[string]string{"WITHVAR": "var"},
			"some/path/var/here",
		},
		{
			"some/path/%WITHVAR%/here",
			map[string]string{"NOTWITHVAR": "var"},
			"some/path//here",
		},
		{
			"C:\\%VAR%\\some/path/%WITHVAR%/here",
			map[string]string{"VAR": "a-value", "WITHVAR": "var"},
			"C:\\a-value\\some/path/var/here",
		},
		{
			"%some/path/%WITHVAR%/here",
			map[string]string{"some/path/%WITHVAR%/here": "not this!", "WITHVAR": "var"},
			"%some/path/var/here",
		},
		{
			"some/path/%%/here",
			map[string]string{"some/path/%WITHVAR%/here": "not this!", "WITHVAR": "var"},
			"some/path/%/here",
		},
		{
			"%some%/path/var/here",
			map[string]string{"some": "var"},
			"var/path/var/here",
		},
		{
			"%some%/path/var/here",
			map[string]string{},
			"/path/var/here",
		},
		{
			"some/path/var/here",
			map[string]string{},
			"some/path/var/here",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.path), func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := tt.path.ExpandEnv()

			if got != tt.want {
				t.Errorf("ExpandEnv(%q) = %v, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path    fspath.Path
		want    fspath.Path
		wantErr bool
	}{
		{
			"~\\test\\file",
			home() + "\\test\\file",
			false,
		},
		{
			"~dontexist\\test\\file",
			"",
			true,
		},
		{
			"~\\",
			home(),
			false,
		},
		{
			"~",
			home(),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.path), func(t *testing.T) {
			t.Parallel()

			got, gotErr := tt.path.Abs()

			if gotErr == nil && tt.wantErr {
				t.Fatal("ExpandUser() succeeded unexpectedly")
			}

			if gotErr != nil && !tt.wantErr {
				t.Errorf("ExpandUser() failed: %v", gotErr)
			}

			if got != tt.want {
				t.Errorf("ExpandUser(%v) = %v, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func cwd() fspath.Path {
	path, _ := os.Getwd()

	return fspath.Path(path)
}

func home() fspath.Path {
	path, _ := os.UserHomeDir()

	return fspath.Path(path)
}
