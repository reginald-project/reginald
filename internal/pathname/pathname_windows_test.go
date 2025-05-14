//go:build windows

package pathname_test

import (
	"os"
	"strings"
	"testing"

	"github.com/anttikivi/reginald/internal/pathname"
)

func TestAbs(t *testing.T) {
	drive := cwd()[:strings.IndexByte(cwd(), ':')+1]
	tests := []struct {
		path    string
		env     map[string]string
		want    string
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
		t.Run(tt.path, func(t *testing.T) {
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

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		path string
		env  map[string]string
		want string
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
		t.Run(tt.path, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := pathname.ExpandEnv(tt.path)

			if got != tt.want {
				t.Errorf("ExpandEnv(%q) = %v, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path    string
		want    string
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
		t.Run(tt.path, func(t *testing.T) {
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

func cwd() string {
	path, _ := os.Getwd()

	return path
}

func home() string {
	path, _ := os.UserHomeDir()

	return path
}
