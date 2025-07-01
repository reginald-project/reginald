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

// Package typeconv contains utilities for safely converting between built-in
// types in Go.
package typeconv

import (
	"errors"
	"fmt"
	"math"

	"github.com/reginald-project/reginald/internal/fspath"
)

// ErrConv is returned when a type conversion is invalid.
var ErrConv = errors.New("cannot convert type")

// ToBoolSlice converts a slice with elements of type any to []bool.
func ToBoolSlice(a []any) ([]bool, error) {
	out := make([]bool, len(a))

	for i, v := range a {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: %[2]v (%[2]T) to %T", ErrConv, v, b)
		}

		out[i] = b
	}

	return out, nil
}

// ToInt converts any integer, unsigned integer, or float value to int safely.
//
//nolint:cyclop // need to check all of the types
func ToInt(a any) (int, error) {
	if a == nil {
		return 0, fmt.Errorf("%w: nil to int", ErrConv)
	}

	switch v := a.(type) { //nolint:varnamelen
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		if math.MaxInt < v || v < math.MinInt {
			return 0, fmt.Errorf("%w: %d out of range", ErrConv, v)
		}

		return int(v), nil
	case uint:
		if math.MaxInt < uint64(v) {
			return 0, fmt.Errorf("%w: %d out of range", ErrConv, v)
		}

		return int(v), nil // #nosec G115 -- bounds checked above
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		if math.MaxInt < uint64(v) {
			return 0, fmt.Errorf("%w: %d out of range", ErrConv, v)
		}

		return int(v), nil
	case uint64:
		if math.MaxInt < v {
			return 0, fmt.Errorf("%w: %d out of range", ErrConv, v)
		}

		return int(v), nil // #nosec G115 -- bounds checked above
	case float32:
		if math.IsNaN(float64(v)) {
			return 0, fmt.Errorf("%w: NaN to int", ErrConv)
		}

		if math.IsInf(float64(v), 0) {
			return 0, fmt.Errorf("%w: Inf to int", ErrConv)
		}

		if math.MaxInt < v || v < math.MinInt {
			return 0, fmt.Errorf("%w: %f out of range", ErrConv, v)
		}

		return int(v), nil
	case float64:
		if math.IsNaN(v) {
			return 0, fmt.Errorf("%w: NaN to int", ErrConv)
		}

		if math.IsInf(v, 0) {
			return 0, fmt.Errorf("%w: Inf to int", ErrConv)
		}

		if math.MaxInt < v || v < math.MinInt {
			return 0, fmt.Errorf("%w: %f out of range", ErrConv, v)
		}

		return int(v), nil
	default:
		return 0, fmt.Errorf("%w: invalid type %T", ErrConv, v)
	}
}

// ToIntMap converts a map with elements of type any to map[string]int.
func ToIntMap(a map[string]any) (map[string]int, error) {
	m := make(map[string]int, len(a))

	for k, v := range a {
		n, err := ToInt(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %[2]v (%[2]T) to %T", err, v, n)
		}

		m[k] = n
	}

	return m, nil
}

// ToIntSlice converts a slice with elements of type any to []int.
func ToIntSlice(a []any) ([]int, error) {
	s := make([]int, len(a))

	for i, v := range a {
		n, err := ToInt(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %[2]v (%[2]T) to %T", err, v, n)
		}

		s[i] = n
	}

	return s, nil
}

// ToPathSlice converts a slice with elements of type any to []fspath.Path.
func ToPathSlice(a []any) ([]fspath.Path, error) {
	out := make([]fspath.Path, len(a))

	for i, v := range a {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %[2]v (%[2]T) to %T", ErrConv, v, s)
		}

		out[i] = fspath.Path(s)
	}

	return out, nil
}

// ToStringSlice converts a slice with elements of type any to []string.
func ToStringSlice(a []any) ([]string, error) {
	out := make([]string, len(a))

	for i, v := range a {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %[2]v (%[2]T) to %T", ErrConv, v, s)
		}

		out[i] = s
	}

	return out, nil
}
