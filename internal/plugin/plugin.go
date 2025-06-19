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
	Manifest() api.Manifest
}

// A Store stores the plugins, provides information on them, and has functions
// for using the plugins within the program.
type Store struct {
	// Plugins is the list of plugins.
	Plugins []Plugin

	// cmdCache is a cache of the commands for the plugins. The commands are
	// stored as they are accessed for the first time. The key in the map is
	// the parent commands and the command name, starting from the plugin domain
	// and separated by colons. It should match the list "names" that is passed
	// to Command, but the elements are joined with colons. Whenever a command
	// is accessed by its alias, the command is stored in the cache using that
	// alias in the place it was used when accessing through Command.
	cmdCache map[string]*api.Command
}

// NewStore creates a new Store that contains the plugins from the given
// manifests.
func NewStore(manifests []api.Manifest) *Store {
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
func Search(ctx context.Context, wd fspath.Path, paths []fspath.Path) ([]api.Manifest, error) {
	var (
		mu        sync.Mutex
		manifests []api.Manifest
	)

	// The built-in plugins should be added first as they are already included
	// with the program. The external plugins are validated while they are being
	// loaded so by loading the built-in plugins first, we can make sure that no
	// external plugin collides with them.
	manifests = append(manifests, builtin.Manifests()...)

	eg, gctx := errgroup.WithContext(ctx)

	for _, path := range paths {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			var err error

			if !path.IsAbs() {
				if strings.HasPrefix(path.String(), "~") {
					path, err = path.Abs()
				} else {
					path, err = fspath.NewAbs(string(wd), string(path))
				}

				if err != nil {
					return fmt.Errorf("%w", err)
				}
			}

			logging.TraceContext(gctx, "checking plugin search path", "path", path)

			var dir []os.DirEntry

			dir, err = path.Clean().ReadDir()
			if err != nil {
				return fmt.Errorf("failed to read directory %q: %w", path, err)
			}

			g2, ctx2 := errgroup.WithContext(gctx)

			for _, entry := range dir {
				handlePanic := panichandler.WithStackTrace()

				g2.Go(func() error {
					defer handlePanic()

					logging.TraceContext(
						ctx2,
						"checking dir entry",
						"path",
						path,
						"name",
						entry.Name(),
					)

					manifest, err := load(ctx2, path, entry)
					if err != nil {
						if errors.Is(err, errNoManifestFile) {
							logging.TraceContext(
								ctx2,
								"no manifest file found",
								"path",
								path,
								"name",
								entry.Name(),
							)

							return nil
						}

						return fmt.Errorf("%w", err)
					}

					logging.TraceContext(ctx2, "loaded manifest", "manifest", manifest)
					mu.Lock()
					defer mu.Unlock()

					if err = checkDuplicates(ctx2, manifest, manifests); err != nil {
						return fmt.Errorf("%w", err)
					}

					logging.TraceContext(
						ctx2,
						"appending manifest",
						"manifest",
						manifest,
						"path",
						path,
					)

					manifests = append(manifests, manifest)

					return nil
				})
			}

			if err = g2.Wait(); err != nil {
				return fmt.Errorf("%w", err)
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	// Maybe not necessary, but it's nice to create the log values only if
	// they're used.
	if slog.Default().Enabled(ctx, slog.Level(logs.LevelDebug)) {
		names := make([]string, 0, len(manifests))
		domains := make([]string, 0, len(manifests))

		for _, m := range manifests {
			names = append(names, m.Name)
			domains = append(domains, m.Domain)
		}

		logging.DebugContext(ctx, "loaded plugin manifests", "names", names, "domains", domains)
	}

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

// check validates the given Manifest and normalizes its values, for example
// setting the name of the plugin as its domain if the plugin doesn't provide
// one.
func check(manifest api.Manifest, path fspath.Path) (api.Manifest, error) {
	if manifest.Name == "" {
		return api.Manifest{}, fmt.Errorf(
			"%w: manifest at %q did not specify a name",
			errInvalidManifest,
			path,
		)
	}

	if manifest.Domain == "" {
		manifest.Domain = manifest.Name
	}

	if manifest.Executable == "" {
		return api.Manifest{}, fmt.Errorf(
			"%w: manifest at %q did not specify executable",
			errInvalidManifest,
			path,
		)
	}

	execPath, err := fspath.NewAbs(string(path.Dir()), manifest.Executable)
	if err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	}

	if ok, err := execPath.IsFile(); err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	} else if !ok {
		return api.Manifest{}, fmt.Errorf("%w: executable at %q is not a file", errInvalidManifest, execPath)
	}

	manifest.Executable = string(execPath)

	return manifest, nil
}

// checkDuplicates checks if the manifest has duplicate fields with manifests
// that are already defined. A manifest may not have the same name, domain, or
// executable as some other manifest. As the function reads the manifests slice,
// the lock protecting the slice should be locked before calling this function.
func checkDuplicates(ctx context.Context, manifest api.Manifest, manifests []api.Manifest) error {
	for _, m := range manifests {
		if m.Name == manifest.Name {
			logging.TraceContext(ctx, "conflicting manifests", "new", manifest, "old", m)

			return fmt.Errorf("%w: duplicate plugin name %q", errInvalidManifest, m.Name)
		}

		if m.Domain == manifest.Domain {
			logging.TraceContext(ctx, "conflicting manifests", "new", manifest, "old", m)

			return fmt.Errorf("%w: duplicate plugin domain %q", errInvalidManifest, m.Domain)
		}

		if m.Executable == manifest.Executable {
			logging.TraceContext(ctx, "conflicting manifests", "new", manifest, "old", m)

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
func load(ctx context.Context, path fspath.Path, dirEntry os.DirEntry) (api.Manifest, error) {
	if !dirEntry.IsDir() {
		logging.TraceContext(ctx, "entry is not directory", "path", path, "name", dirEntry.Name())

		return api.Manifest{}, fmt.Errorf("%w: %s", errNoManifestFile, path)
	}

	manifestPath := path.Join(dirEntry.Name(), "manifest.json")
	if ok, err := manifestPath.IsFile(); err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	} else if !ok {
		return api.Manifest{}, fmt.Errorf("%w: %s", errNoManifestFile, manifestPath)
	}

	data, err := manifestPath.Clean().ReadFile()
	if err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	var manifest api.Manifest
	if err := d.Decode(&manifest); err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	}

	manifest, err = check(manifest, manifestPath)
	if err != nil {
		return api.Manifest{}, fmt.Errorf("%w", err)
	}

	return manifest, nil
}
