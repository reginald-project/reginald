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

// Package plugin implements the plugin client of Reginald.
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/builtin"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/panichandler"
	"golang.org/x/sync/errgroup"
)

// Errors returned by the plugin functions.
var (
	errInvalidManifest = errors.New("invalid plugin manifest")
	errNoManifestFile  = errors.New("no manifest file found")
)

// A Plugin is a plugin that Reginald recognizes.
type Plugin interface {
	// Manifest returns the loaded manifest for the plugin.
	Manifest() *api.Manifest
}

// A Store stores the plugins, provides information on them, and has functions
// for using the plugins within the program.
type Store struct {
	// cmdCache is a cache of the commands for the plugins. The commands are
	// stored as they are accessed for the first time. The key in the map is
	// the parent commands and the command name, starting from the plugin domain
	// and separated by colons. It should match the list "names" that is passed
	// to Command, but the elements are joined with colons. Whenever a command
	// is accessed by its alias, the command is stored in the cache using that
	// alias in the place it was used when accessing through Command.
	cmdCache map[string]*api.Command

	// Plugins is the list of plugins.
	Plugins []Plugin
}

type searchOptions struct {
	mu           *sync.Mutex
	manifests    *api.Manifests
	panicHandler func()
	path         fspath.Path
	wd           fspath.Path
}

type pathEntryOptions struct {
	mu           *sync.Mutex
	manifests    *api.Manifests
	panicHandler func()
	dir          os.DirEntry
	path         fspath.Path
}

// NewStore creates a new Store that contains the plugins from the given
// manifests.
func NewStore(manifests api.Manifests) *Store {
	plugins := make([]Plugin, 0, len(manifests))

	// TODO: These need to be properly handled.
	for _, m := range manifests {
		// Built-in plugins don't require any complex setups.
		if m.Domain == "builtin" {
			plugins = append(plugins, newBuiltin(m))

			continue
		}

		plugins = append(plugins, newExternal(m))
	}

	store := &Store{
		Plugins:  plugins,
		cmdCache: map[string]*api.Command{},
	}

	return store
}

// Command returns the command for the given names. It looks for the commands by
// the names in the order the names would be given on the command-line. This
// means that the first part of names should be the first parent command which
// is usually the plugin domain. If no command is found, Command return nil.
func (s *Store) Command(names ...string) *api.Command {
	if len(names) < 1 {
		return nil
	}

	if cmd, ok := s.cmdCache[strings.Join(names, ":")]; ok {
		return cmd
	}

	var parent *api.Command

	if len(names) > 1 {
		parent = s.Command(names[0 : len(names)-1]...)
	}

	cmd := s.findCmd(parent, names[len(names)-1])
	if cmd != nil {
		s.cmdCache[strings.Join(names, ":")] = cmd
	}

	return cmd
}

// Len returns the number of plugins in the store.
func (s *Store) Len() int {
	return len(s.Plugins)
}

// Search finds the available plugins by their "manifest.json" files and loads
// the manifest information.
func Search(ctx context.Context, wd fspath.Path, paths []fspath.Path) (api.Manifests, error) {
	var mu sync.Mutex

	// The built-in plugins should be added first as they are already included
	// with the program. The external plugins are validated while they are being
	// loaded so by loading the built-in plugins first, we can make sure that no
	// external plugin collides with them.
	manifests := slices.Clone(builtin.Manifests())

	eg, gctx := errgroup.WithContext(ctx)

	for _, path := range paths {
		opts := searchOptions{
			path:         path,
			wd:           wd,
			mu:           &mu,
			manifests:    &manifests,
			panicHandler: panichandler.WithStackTrace(),
		}

		eg.Go(func() error {
			return searchPath(gctx, opts)
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	logLoadedManifest(ctx, manifests)

	return manifests, nil
}

// findCmd finds the command for the given name in the given parent command. If
// parent is nil, the function looks for name in the core commands and
// the plugin domains. If the name resolves to a plugin domain, the function
// returns a new command for that domain.
func (s *Store) findCmd(parent *api.Command, name string) *api.Command {
	if parent == nil {
		for _, plugin := range s.Plugins {
			manifest := plugin.Manifest()

			if manifest.Domain == "core" {
				for _, c := range manifest.Commands {
					if c.Name == name || slices.Contains(c.Aliases, name) {
						return &c
					}
				}
			}

			if manifest.Domain == name {
				return &api.Command{
					Name: manifest.Domain,
					// TODO: Generate some kind of actually useful usage line.
					Usage:       manifest.Domain + " [options]",
					Description: manifest.Description,
					Aliases:     nil,
					Config:      manifest.Config,
					Commands:    manifest.Commands,
				}
			}
		}

		return nil
	}

	for _, c := range parent.Commands {
		if c.Name == name || slices.Contains(c.Aliases, name) {
			return &c
		}
	}

	return nil
}

// checkDuplicates checks if the manifest has duplicate fields with manifests
// that are already defined. A manifest may not have the same name, domain, or
// executable as some other manifest. As the function reads the manifests slice,
// the lock protecting the slice should be locked before calling this function.
func checkDuplicates(ctx context.Context, manifest *api.Manifest, manifests api.Manifests) error {
	for _, m := range manifests {
		if m.Name == manifest.Name {
			logging.Trace(ctx, "conflicting manifests", "new", manifest, "old", m)

			return fmt.Errorf("%w: duplicate plugin name %q", errInvalidManifest, m.Name)
		}

		if m.Domain == manifest.Domain {
			logging.Trace(ctx, "conflicting manifests", "new", manifest, "old", m)

			return fmt.Errorf("%w: duplicate plugin domain %q", errInvalidManifest, m.Domain)
		}

		if m.Executable == manifest.Executable {
			logging.Trace(ctx, "conflicting manifests", "new", manifest, "old", m)

			return fmt.Errorf(
				"%w: duplicate plugin executable path %q",
				errInvalidManifest,
				m.Executable,
			)
		}
	}

	return nil
}

// load loads the manifest from the search path for the DirEntry.
func load(ctx context.Context, path fspath.Path, dirEntry os.DirEntry) (*api.Manifest, error) {
	if !dirEntry.IsDir() {
		logging.Trace(ctx, "entry is not directory", "path", path, "name", dirEntry.Name())

		return nil, fmt.Errorf("%w: %s", errNoManifestFile, path)
	}

	manifestPath := path.Join(dirEntry.Name(), "manifest.json")
	if ok, err := manifestPath.IsFile(); err != nil {
		return nil, fmt.Errorf("%w", err)
	} else if !ok {
		return nil, fmt.Errorf("%w: %s", errNoManifestFile, manifestPath)
	}

	data, err := manifestPath.Clean().ReadFile()
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	var manifest *api.Manifest
	if err = d.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	if err = revise(manifest, manifestPath); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return manifest, nil
}

func logLoadedManifest(ctx context.Context, manifests api.Manifests) {
	// Maybe not necessary, but it's nice to create the log values only if
	// they're used.
	if slog.Default().Enabled(ctx, slog.Level(logs.LevelDebug)) {
		names := make([]string, len(manifests))
		domains := make([]string, len(manifests))

		for i, m := range manifests {
			names[i] = m.Name
			domains[i] = m.Domain
		}

		logging.Debug(ctx, "loaded plugin manifests", "names", names, "domains", domains)
	}
}

// revise validates the given Manifest and normalizes its values, for example
// setting the name of the plugin as its domain if the plugin doesn't provide
// one. It modifies the given manifest in place.
func revise(manifest *api.Manifest, path fspath.Path) error {
	if manifest.Name == "" {
		return fmt.Errorf(
			"%w: manifest at %q did not specify a name",
			errInvalidManifest,
			path,
		)
	}

	if manifest.Domain == "" {
		manifest.Domain = manifest.Name
	}

	if manifest.Executable == "" {
		return fmt.Errorf(
			"%w: manifest at %q did not specify executable",
			errInvalidManifest,
			path,
		)
	}

	execPath, err := fspath.NewAbs(string(path.Dir()), manifest.Executable)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if ok, err := execPath.IsFile(); err != nil {
		return fmt.Errorf("%w", err)
	} else if !ok {
		return fmt.Errorf("%w: executable at %q is not a file", errInvalidManifest, execPath)
	}

	manifest.Executable = string(execPath)

	return nil
}

func searchPath(ctx context.Context, opts searchOptions) error {
	defer opts.panicHandler()

	var err error

	if !opts.path.IsAbs() {
		if strings.HasPrefix(opts.path.String(), "~") {
			opts.path, err = opts.path.Abs()
		} else {
			opts.path, err = fspath.NewAbs(string(opts.wd), string(opts.path))
		}

		if err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	logging.Trace(ctx, "checking plugin search path", "path", opts.path)

	var dir []os.DirEntry

	dir, err = opts.path.Clean().ReadDir()
	if err != nil {
		return fmt.Errorf("failed to read directory %q: %w", opts.path, err)
	}

	eg, gctx := errgroup.WithContext(ctx)

	for _, entry := range dir {
		entryOpts := pathEntryOptions{
			path:         opts.path,
			dir:          entry,
			mu:           opts.mu,
			manifests:    opts.manifests,
			panicHandler: panichandler.WithStackTrace(),
		}

		eg.Go(func() error {
			return searchPathEntry(gctx, entryOpts)
		})
	}

	if err = eg.Wait(); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func searchPathEntry(ctx context.Context, opts pathEntryOptions) error {
	defer opts.panicHandler()

	logging.Trace(
		ctx,
		"checking dir entry",
		"path",
		opts.path,
		"name",
		opts.dir.Name(),
	)

	manifest, err := load(ctx, opts.path, opts.dir)
	if err != nil {
		if errors.Is(err, errNoManifestFile) {
			logging.Trace(
				ctx,
				"no manifest file found",
				"path",
				opts.path,
				"name",
				opts.dir.Name(),
			)

			return nil
		}

		return fmt.Errorf("%w", err)
	}

	logging.Trace(ctx, "loaded manifest", "manifest", manifest)
	opts.mu.Lock()
	defer opts.mu.Unlock()

	if err = checkDuplicates(ctx, manifest, *opts.manifests); err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.Trace(
		ctx,
		"appending manifest",
		"manifest",
		manifest,
		"path",
		opts.path,
	)

	*opts.manifests = append(*opts.manifests, manifest)

	logging.Trace(
		ctx,
		"manifest loaded",
		"manifest",
		manifest,
		"manifests",
		*opts.manifests,
	)

	return nil
}
