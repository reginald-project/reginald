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

package logconfig

import (
	"bytes"
	"strings"
	"testing"
)

func TestLevelString(t *testing.T) {
	t.Parallel()

	//nolint:govet // don't care about this in tests
	for _, test := range []struct {
		in   Level
		want string
	}{
		{0, "INFO"},
		{LevelError, "ERROR"},
		{LevelError + 2, "ERROR+2"},
		{LevelError - 2, "WARN+2"},
		{LevelWarn, "WARN"},
		{LevelWarn - 1, "INFO+3"},
		{LevelInfo, "INFO"},
		{LevelInfo + 1, "INFO+1"},
		{LevelInfo - 3, "DEBUG+1"},
		{LevelDebug, "DEBUG"},
		{LevelDebug - 2, "TRACE+2"},
		{LevelDebug - 3, "TRACE+1"},
		{LevelTrace, "TRACE"},
		{LevelTrace - 2, "TRACE-2"},
	} {
		got := test.in.String()
		if got != test.want {
			t.Errorf("%d: got %s, want %s", test.in, got, test.want)
		}
	}
}

func TestLevelMarshalJSON(t *testing.T) {
	t.Parallel()

	want := LevelWarn - 3
	wantData := []byte(`"INFO+1"`)

	data, err := want.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, wantData) {
		t.Errorf("got %s, want %s", string(data), string(wantData))
	}

	var got Level
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLevelMarshalText(t *testing.T) {
	t.Parallel()

	want := LevelWarn - 3
	wantData := []byte("INFO+1")

	data, err := want.MarshalText()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, wantData) {
		t.Errorf("got %s, want %s", string(data), string(wantData))
	}

	var got Level
	if err := got.UnmarshalText(data); err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLevelAppendText(t *testing.T) {
	t.Parallel()

	buf := make([]byte, 4, 16)
	want := LevelWarn - 3
	wantData := []byte("\x00\x00\x00\x00INFO+1")

	data, err := want.AppendText(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, wantData) {
		t.Errorf("got %s, want %s", string(data), string(wantData))
	}
}

func TestLevelParse(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		in   string
		want Level
	}{
		{"DEBUG", LevelDebug},
		{"INFO", LevelInfo},
		{"WARN", LevelWarn},
		{"ERROR", LevelError},
		{"debug", LevelDebug},
		{"iNfo", LevelInfo},
		{"INFO+87", LevelInfo + 87},
		{"Error-18", LevelError - 18},
		{"Error-8", LevelInfo},
	} {
		var got Level
		if err := got.parse(test.in); err != nil {
			t.Fatalf("%q: %v", test.in, err)
		}

		if got != test.want {
			t.Errorf("%q: got %s, want %s", test.in, got, test.want)
		}
	}
}

func TestLevelParseError(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		in   string
		want string // error string should contain this
	}{
		{"", "unknown name"},
		{"dbg", "unknown name"},
		{"INFO+", "invalid syntax"},
		{"INFO-", "invalid syntax"},
		{"ERROR+23x", "invalid syntax"},
	} {
		var l Level

		err := l.parse(test.in)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%q: got %v, want string containing %q", test.in, err, test.want)
		}
	}
}
