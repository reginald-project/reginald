// Copyright 2025 Antti Kivi
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

	"github.com/anttikivi/reginald/internal/fspath"
	"github.com/anttikivi/reginald/pkg/rpp"
	"github.com/spf13/pflag"
)

// Errors returned from flag operations.
var (
	errDefaultValueType = errors.New("failed to cast the plugin flag value to correct type")
	errDuplicateFlag    = errors.New("trying to add a flag that already exists")
	errInvalidFlagType  = errors.New("plugin has a flag with an invalid type")
)

// A FlagSet is a wrapper of [pflag.FlagSet] that includes the [Flag] objects
// that corresponds to the flags in the wrapped flag set. It keeps the sets in
// sync.
type FlagSet struct {
	*pflag.FlagSet

	flags map[string]*Flag
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
		FlagSet: pflag.NewFlagSet(name, errorHandling),
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
}

// AddPluginFlag adds a flag to the flag set according to the given config value
// specification from a plugin.
func (f *FlagSet) AddPluginFlag(entry *rpp.ConfigEntry) error {
	var flag rpp.Flag

	rf, err := entry.RealFlag()
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if rf == nil {
		return nil
	}

	flag = *rf

	if f := f.Lookup(flag.Name); f != nil {
		return fmt.Errorf("%w: %s", errDuplicateFlag, f.Name)
	}

	// TODO: Add inverted flags.
	switch entry.Type {
	case rpp.ConfigBool:
		defVal, ok := entry.Value.(bool)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, entry.Value)
		}

		f.BoolP(flag.Name, flag.Shorthand, defVal, flag.Usage, "")
	case rpp.ConfigInt:
		switch v := entry.Value.(type) {
		case int:
			f.IntP(flag.Name, flag.Shorthand, v, flag.Usage, "")
		case float64:
			// TODO: This is probably the most unsafe way to do this, but it'll
			// be fixed later.
			u := int(v)

			f.IntP(flag.Name, flag.Shorthand, u, flag.Usage, "")
		default:
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, entry.Value)
		}
	case rpp.ConfigString:
		defVal, ok := entry.Value.(string)
		if !ok {
			return fmt.Errorf("%w: %[2]v (%[2]T)", errDefaultValueType, entry.Value)
		}

		f.StringP(flag.Name, flag.Shorthand, defVal, flag.Usage, "")
	default:
		return fmt.Errorf("%w: flag %q: %v (%T)", errInvalidFlagType, flag.Name, entry.Type, entry.Value)
	}

	return nil
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
