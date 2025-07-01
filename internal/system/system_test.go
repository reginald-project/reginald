package system_test

import (
	"runtime"
	"testing"

	"github.com/reginald-project/reginald/internal/system"
)

func TestOSCurrent(t *testing.T) {
	type testCase struct {
		input system.OS
		want  bool
	}

	var (
		id     string
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
			got := tt.input.Current()
			if got != tt.want {
				t.Errorf("OS(%s) = %t, want %t", tt.input, got, tt.want)
			}
		})
	}
}
