//go:build !windows

package fspath_test

import (
	"os"
	"os/user"
	"testing"

	"github.com/anttikivi/reginald/internal/fspath"
)

func TestAbs(t *testing.T) {
	tests := []struct {
		path    fspath.Path
		env     map[string]string
		want    fspath.Path
		wantErr bool
	}{
		{
			"./test/file",
			nil,
			cwd() + "/test/file",
			false,
		},
		{
			"/test/file",
			nil,
			"/test/file",
			false,
		},
		{
			"~/test/file",
			nil,
			home() + "/test/file",
			false,
		},
		{
			"~dontexist/test/file",
			nil,
			"",
			true,
		},
		{
			"$HOME/test/file",
			nil,
			home() + "/test/file",
			false,
		},
		{
			"~/$ENVVAR/file",
			map[string]string{"ENVVAR": "path"},
			home() + "/path/file",
			false,
		},
		{
			"~/$ENVVAR/${SECOND_VAR}",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home() + "/path/file",
			false,
		},
		{
			"/$ENVVAR/${SECOND_VAR}",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			"/path/file",
			false,
		},
		{
			"~/",
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
			"~" + currentUser() + "/",
			nil,
			home(),
			false,
		},
		{
			"~" + currentUser(),
			nil,
			home(),
			false,
		},
		{
			"~/././file",
			map[string]string{"ENVVAR": "path", "SECOND_VAR": "file"},
			home() + "/file",
			false,
		},
		{
			"~/././file/..",
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

func TestExpandUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path    fspath.Path
		want    fspath.Path
		wantErr bool
	}{
		{
			"/test/file",
			"/test/file",
			false,
		},
		{
			"~/test/file",
			home() + "/test/file",
			false,
		},
		{
			"~dontexist/test/file",
			"",
			true,
		},
		{
			"~/",
			home(),
			false,
		},
		{
			"~",
			home(),
			false,
		},
		{
			"~" + currentUser() + "/",
			home(),
			false,
		},
		{
			"~" + currentUser(),
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

func currentUser() fspath.Path {
	u, _ := user.Current()

	return fspath.Path(u.Username)
}
