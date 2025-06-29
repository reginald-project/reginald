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

// Package flags contains the command-line flag types of Reginald. They are
// wrappers of [pflag] package's flags that contain additional functionality
// needed by the program. Parsing the flags and storing their values is still
// done by [pflag].
package flags

import (
	"errors"
	"fmt"
	"strings"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/spf13/pflag"
)

// Errors returned from flag operations.
var (
	errDefaultValueType  = errors.New("failed to cast the plugin flag value to correct type")
	errDuplicateFlag     = errors.New("trying to add a flag that already exists")
	errInvalidFlagType   = errors.New("plugin has a flag with an invalid type")
	errMutuallyExclusive = errors.New("two mutually exclusive flags set at the same time")
)

// A FlagSet is a wrapper of [pflag.FlagSet] that includes the [Flag] objects
// that corresponds to the flags in the wrapped flag set. It keeps the sets in
// sync.
type FlagSet struct {
	*pflag.FlagSet

	flags map[string]*Flag

	// mutuallyExclusive is the list of flag names that are marked as
	// mutually exclusive. Each element of the slice is a slice that contains
	// the full names of the mutually exclusive flags in that group.
	mutuallyExclusive [][]string
}

// A Flag is a wrapper of [pflag.Flag] that extends the flag type by including
// a longer documentation used on, for example, manual pages.
type Flag struct {
	*pflag.Flag

	Doc string
}

// NewFlagSet returns a new, empty flag set with the specified name, error
// handling property, and SortFlags set to true.
func NewFlagSet(name string, errorHandling pflag.ErrorHandling) *FlagSet {
	f := &FlagSet{ //nolint:exhaustruct // flags is initialized when needed
		FlagSet:           pflag.NewFlagSet(name, errorHandling),
		mutuallyExclusive: [][]string{},
	}

	return f
}

// AddFlag will add the flag to the FlagSet.
func (f *FlagSet) AddFlag(flag *Flag) {
	if f.flags == nil {
		f.flags = make(map[string]*Flag)
	}

	f.flags[flag.Name] = flag

	// At this point, it might be good to double check that the underlying flag
	// is also in the wrapped flag set.
	if f.Lookup(flag.Name) == nil {
		f.FlagSet.AddFlag(flag.Flag)
	}
}

// AddFlagSet adds one FlagSet to another. If a flag is already present in f
// the flag from newSet will be ignored.
func (f *FlagSet) AddFlagSet(newSet *FlagSet) {
	if newSet == nil {
		return
	}

	f.FlagSet.AddFlagSet(newSet.FlagSet)

	for k, v := range newSet.flags {
		if f.WrapperLookup(k) == nil {
			f.AddFlag(v)
		}
	}

	f.mutuallyExclusive = append(f.mutuallyExclusive, newSet.mutuallyExclusive...)
}

// AddPluginFlag adds a flag to the flag set according to the given ConfigEntry
// specification from a plugin. If the flag in the config entry does not define
// a name, the name will be generated from prefix and the key of cfg.
//
//nolint:cyclop // need for complexity when checking the config type
func (f *FlagSet) AddPluginFlag(cfg *api.ConfigEntry, prefix string) error {
	if cfg == nil {
		panic("nil config entry in AddPluginFlag")
	}

	if cfg.Flag == nil {
		return nil
	}

	flag := *cfg.Flag

	name := flag.Name
	if name == "" {
		name = prefix + "-" + strings.ToLower(cfg.Key)
	}

	if f := f.Lookup(name); f != nil {
		return fmt.Errorf("%w: %s", errDuplicateFlag, f.Name)
	}

	description := flag.Description
	if description == "" {
		description = cfg.Description
	}

	// TODO: Add inverted flags.
	switch cfg.Type {
	case api.BoolListValue:
		defVal, ok := cfg.Value.([]bool)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.BoolSliceP(name, flag.Shorthand, defVal, description)
	case api.BoolValue:
		defVal, ok := cfg.Value.(bool)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.BoolP(name, flag.Shorthand, defVal, description, "")
	case api.IntListValue:
		defVal, ok := cfg.Value.([]int)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.IntSliceP(name, flag.Shorthand, defVal, description)
	case api.IntValue:
		defVal, err := cfg.Int()
		if err != nil {
			return fmt.Errorf("failed to convert default value of %q: %w", cfg.Key, err)
		}

		f.IntP(name, flag.Shorthand, defVal, description, "")
	case api.MapValue:
		// no-op
	case api.PathListValue:
		defVal, ok := cfg.Value.([]fspath.Path)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.PathSliceP(name, flag.Shorthand, defVal, description, "")
	case api.PathValue:
		defVal, ok := cfg.Value.(fspath.Path)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.PathP(name, flag.Shorthand, defVal, description, "")
	case api.StringListValue:
		defVal, ok := cfg.Value.([]string)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.StringSliceP(name, flag.Shorthand, defVal, description)
	case api.StringValue:
		defVal, ok := cfg.Value.(string)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, cfg.Value)
		}

		f.StringP(name, flag.Shorthand, defVal, description, "")
	default:
		return fmt.Errorf("%w: flag %q: %v (%T)", errInvalidFlagType, name, cfg.Type, cfg.Value)
	}

	return nil
}

// CheckMutuallyExclusive checks if two flags marked as mutually exclusive are
// set at the same time by the user. The function returns an error if two
// mutually exclusive flags are set. The function panics if it is called before
// parsing the flags or if any of the flags marked as mutually exclusive is not
// present in the flag set.
func (f *FlagSet) CheckMutuallyExclusive() error {
	if !f.Parsed() {
		panic("calling CheckMutuallyExclusive before parsing the flags")
	}

	for _, a := range f.mutuallyExclusive {
		var set string

		for _, s := range a {
			f := f.Lookup(s)
			if f == nil {
				panic("nil flag in the set of mutually exclusive flags: " + s)
			}

			if f.Changed {
				if set != "" {
					return fmt.Errorf("%w: --%s and --%s (or their shorthands)", errMutuallyExclusive, set, s)
				}

				set = s
			}
		}
	}

	return nil
}

// MarkMutuallyExclusive marks two or more flags as mutually exclusive so that
// the program returns an error if the user tries to set them at the same time.
// This function panics on errors.
func (f *FlagSet) MarkMutuallyExclusive(a ...string) {
	if len(a) < 2 { //nolint:mnd // obvious
		panic("only one flag cannot be marked as mutually exclusive")
	}

	for _, s := range a {
		if f := f.Lookup(s); f == nil {
			panic(fmt.Sprintf("failed to find flag %q while marking it as mutually exclusive", s))
		}
	}

	if f.mutuallyExclusive == nil {
		f.mutuallyExclusive = [][]string{}
	}

	f.mutuallyExclusive = append(f.mutuallyExclusive, a)
}

// MutuallyExclusive returns the list of mutually exclusive flags.
func (f *FlagSet) MutuallyExclusive() [][]string {
	return f.mutuallyExclusive
}

// WrapperLookup returns the Flag structure of the named flag, returning nil if
// none exists.
func (f *FlagSet) WrapperLookup(name string) *Flag {
	return f.flags[name]
}

// Bool defines a bool flag with specified name, default value, and usage
// string. The return value is the address of a bool variable that stores the
// value of the flag.
func (f *FlagSet) Bool(name string, value bool, usage, doc string) *bool {
	return f.BoolP(name, "", value, usage, doc)
}

// BoolP is like Bool, but accepts a shorthand letter that can be used after
// a single dash.
func (f *FlagSet) BoolP(name, shorthand string, value bool, usage, doc string) *bool {
	p := f.FlagSet.BoolP(name, shorthand, value, usage)

	flag := f.Lookup(name)
	if flag == nil {
		panic(fmt.Sprintf("received nil flag %q from wrapped flag set", name))
	}

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})

	return p
}

// Int defines a bool flag with specified name, default value, and usage string.
// The return value is the address of a bool variable that stores the value of
// the flag.
func (f *FlagSet) Int(name string, value int, usage, doc string) *int {
	return f.IntP(name, "", value, usage, doc)
}

// IntP is like Int, but accepts a shorthand letter that can be used after
// a single dash.
func (f *FlagSet) IntP(name, shorthand string, value int, usage, doc string) *int {
	p := f.FlagSet.IntP(name, shorthand, value, usage)

	flag := f.Lookup(name)
	if flag == nil {
		panic(fmt.Sprintf("received nil flag %q from wrapped flag set", name))
	}

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})

	return p
}

// Path defines a path flag with specified name, default value, and usage
// string. The return value is the address of a path variable that stores
// the value of the flag.
func (f *FlagSet) Path(name string, value fspath.Path, usage, doc string) *fspath.Path {
	return f.PathP(name, "", value, usage, doc)
}

// PathP is like Path, but accepts a shorthand letter that can be used after
// a single dash.
func (f *FlagSet) PathP(name, shorthand string, value fspath.Path, usage, doc string) *fspath.Path {
	p := f.FlagSet.StringP(name, shorthand, string(value), usage)

	flag := f.Lookup(name)
	if flag == nil {
		panic(fmt.Sprintf("received nil flag %q from wrapped flag set", name))
	}

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})

	path := fspath.Path(*p)

	return &path
}

// PathSlice defines a paths flag with specified name, default value, and usage
// string. The flag is given as a string that has comma-separated paths, and
// the flag can be specified multiple times. The return value is the address of
// a path variable that stores the value of the flag.
func (f *FlagSet) PathSlice(name string, value []fspath.Path, usage, doc string) *[]string {
	return f.PathSliceP(name, "", value, usage, doc)
}

// PathSliceP is like Path, but accepts a shorthand letter that can be used
// after a single dash.
func (f *FlagSet) PathSliceP(name, shorthand string, value []fspath.Path, usage, doc string) *[]string {
	s := make([]string, 0, len(value))

	for _, p := range value {
		s = append(s, string(p))
	}

	p := f.StringSliceP(name, shorthand, s, usage)

	flag := f.Lookup(name)
	if flag == nil {
		panic(fmt.Sprintf("received nil flag %q from wrapped flag set", name))
	}

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})

	return p
}

// String defines a string flag with specified name, default value, and usage
// string. The return value is the address of a string variable that stores the
// value of the flag.
func (f *FlagSet) String(name, value, usage, doc string) *string {
	return f.StringP(name, "", value, usage, doc)
}

// StringP is like String, but accepts a shorthand letter that can be used after
// a single dash.
func (f *FlagSet) StringP(name, shorthand, value, usage, doc string) *string {
	p := f.FlagSet.StringP(name, shorthand, value, usage)

	flag := f.Lookup(name)
	if flag == nil {
		panic(fmt.Sprintf("received nil flag %q from wrapped flag set", name))
	}

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})

	return p
}

// Var defines a flag with the specified name and usage string. The type and
// value of the flag are represented by the first argument, of type
// [pflag.Value], which typically holds a user-defined implementation of
// [pflag.Value]. For instance, the caller could create a flag that turns
// a comma-separated string into a slice of strings by giving the slice
// the methods of [pflag.Value]; in particular, Set would decompose
// the comma-separated string into the slice.
func (f *FlagSet) Var(value pflag.Value, name, usage, doc string) {
	f.VarP(value, name, "", usage, doc)
}

// VarP is like Var, but accepts a shorthand letter that can be used after
// a single dash.
func (f *FlagSet) VarP(value pflag.Value, name, shorthand, usage, doc string) {
	flag := f.VarPF(value, name, shorthand, usage)

	f.AddFlag(&Flag{
		Flag: flag,
		Doc:  doc,
	})
}

// GetPath returns the string value of a flag with the given name and converts
// it to [fspath.Path].
func (f *FlagSet) GetPath(name string) (fspath.Path, error) {
	val, err := f.GetString(name)
	if err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return fspath.Path(val), nil
}

// GetPathSlice returns the string slice value of a flag with the given name and
// converts it to [fspath.Path] slice.
func (f *FlagSet) GetPathSlice(name string) ([]fspath.Path, error) {
	val, err := f.GetStringSlice(name)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	result := make([]fspath.Path, 0, len(val))

	for _, v := range val {
		result = append(result, fspath.Path(v))
	}

	return result, nil
}
