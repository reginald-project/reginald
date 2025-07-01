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

package typeconv_test

import (
	"errors"
	"maps"
	"math"
	"testing"

	"github.com/reginald-project/reginald/internal/typeconv"
)

func TestToInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   any
		name    string
		want    int
		wantErr bool
	}{
		{
			name:    "nil",
			input:   nil,
			want:    0,
			wantErr: true,
		},
		{
			name:    "invalid type",
			input:   "foo",
			want:    0,
			wantErr: true,
		},
		{
			name:    "int",
			input:   42,
			want:    42,
			wantErr: false,
		},
		{
			name:    "int negative",
			input:   -5,
			want:    -5,
			wantErr: false,
		},
		{
			name:    "int8",
			input:   int8(127),
			want:    127,
			wantErr: false,
		},
		{
			name:    "int16",
			input:   int16(-32768),
			want:    -32768,
			wantErr: false,
		},
		{
			name:    "int32",
			input:   int32(123456),
			want:    123456,
			wantErr: false,
		},
		{
			name:    "int64 in range",
			input:   int64(math.MaxInt),
			want:    math.MaxInt,
			wantErr: false,
		},
		{
			name:    "int64 out of range",
			input:   uint64(math.MaxInt) + 1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "uint",
			input:   uint(123),
			want:    123,
			wantErr: false,
		},
		{
			name:    "uint out of range",
			input:   uint(math.MaxInt) + 1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "uint8",
			input:   uint8(255),
			want:    255,
			wantErr: false,
		},
		{
			name:    "uint16",
			input:   uint16(65535),
			want:    65535,
			wantErr: false,
		},
		{
			name:    "uint64 in range",
			input:   uint64(math.MaxInt),
			want:    math.MaxInt,
			wantErr: false,
		},
		{
			name:    "uint64 out of range",
			input:   uint64(math.MaxInt) + 1,
			want:    0,
			wantErr: true,
		},
		{
			name:    "float32",
			input:   float32(3.7),
			want:    3,
			wantErr: false,
		},
		{
			name:    "float32 NaN",
			input:   float32(math.NaN()),
			want:    0,
			wantErr: true,
		},
		{
			name:    "float32 Inf",
			input:   float32(math.Inf(1)),
			want:    0,
			wantErr: true,
		},
		{
			name:    "float32 out of range",
			input:   float32(math.MaxInt) * 2,
			want:    0,
			wantErr: true,
		},
		{
			name:    "float64",
			input:   6.9,
			want:    6,
			wantErr: false,
		},
		{
			name:    "float64 negative",
			input:   -2.9,
			want:    -2,
			wantErr: false,
		},
		{
			name:    "float64 NaN",
			input:   math.NaN(),
			want:    0,
			wantErr: true,
		},
		{
			name:    "float64 Inf",
			input:   math.Inf(-1),
			want:    0,
			wantErr: true,
		},
		{
			name:    "float64 out of range",
			input:   float64(math.MaxInt) * 2,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := typeconv.ToInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ToInt(%#v): expected error? %v, got err=%v", tt.input, tt.wantErr, err)
			}

			if err != nil {
				if !errors.Is(err, typeconv.ErrConv) {
					t.Errorf("ToInt(%#v) error = %v, want wrapping ErrConv", tt.input, err)
				}

				return
			}

			if got != tt.want {
				t.Errorf("ToInt(%#v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestToIntMap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		m       map[string]any
		want    map[string]int
		name    string
		wantErr bool
	}{
		{
			name:    "simple",
			m:       map[string]any{"first": 1, "second": 2, "third": 3},
			want:    map[string]int{"first": 1, "second": 2, "third": 3},
			wantErr: false,
		},
		{
			name:    "wrong type",
			m:       map[string]any{"first": 1, "second": "2", "third": 3},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "int64",
			m:       map[string]any{"first": int64(1), "second": int64(2), "third": int64(3)},
			want:    map[string]int{"first": 1, "second": 2, "third": 3},
			wantErr: false,
		},
		{
			name:    "float64",
			m:       map[string]any{"first": float64(1), "second": float64(2), "third": float64(3)},
			want:    map[string]int{"first": 1, "second": 2, "third": 3},
			wantErr: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := typeconv.ToIntMap(tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToIntMap(%+v) failed unexpectedly: %v", tt.m, err)
			}

			if !maps.Equal(got, tt.want) {
				t.Errorf("ToIntMap(%+v) = %+v, want %+v", tt.m, got, tt.want)
			}
		})
	}
}
