//go:build !windows

package pathname_test

import (
	"os"
	"os/user"
	"testing"

	"github.com/anttikivi/reginald/internal/pathname"
)

func TestAbs(t *testing.T) {
	tests := []struct {
		path    pathname.Path
		env     map[string]string
		want    pathname.Path
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

			got, gotErr := pathname.Abs(tt.path)

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
		path    pathname.Path
		want    pathname.Path
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

			got, gotErr := pathname.Abs(tt.path)

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

func cwd() pathname.Path {
	path, _ := os.Getwd()

	return pathname.Path(path)
}

func home() pathname.Path {
	path, _ := os.UserHomeDir()

	return pathname.Path(path)
}

func currentUser() pathname.Path {
	u, _ := user.Current()

	return pathname.Path(u.Username)
}
