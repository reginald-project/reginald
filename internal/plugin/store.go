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

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/builtin"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/log"
	"github.com/reginald-project/reginald/internal/panichandler"
	"golang.org/x/sync/errgroup"
)

// A Store stores the plugins, provides information on them, and has functions
// for using the plugins within the program.
type Store struct {
	// Plugins is the list of plugins.
	Plugins []Plugin

	// Commands is the list of commands that are defined in the plugins.
	Commands []*Command
}

// Search finds the available plugins by their "manifest.json" files and loads
// the manifest information.
func NewStore(ctx context.Context, wd fspath.Path, paths []fspath.Path) (*Store, error) {
	// The built-in plugins should be added first as they are already included
	// with the program. The external plugins are validated while they are being
	// loaded so by loading the built-in plugins first, we can make sure that no
	// external plugin collides with them.
	manifests := slices.Clone(builtin.Manifests())
	plugins := make([]Plugin, 0, len(manifests))

	for _, m := range manifests {
		plugins = append(plugins, &builtinPlugin{
			manifest: m,
		})
	}

	var pathErrs PathErrors

	external, err := readAllSearchPaths(ctx, wd, paths)
	if err != nil && !errors.As(err, &pathErrs) {
		return nil, err
	}

	plugins = append(plugins, external...)

	if slog.Default().Enabled(ctx, slog.Level(logs.LevelTrace)) {
		for _, p := range plugins {
			log.Trace(ctx, "loaded name-domain pair", "name", p.Manifest().Name, "domain", p.Manifest().Domain)
		}
	}

	var commands []*Command

	for _, p := range plugins {
		cmds := newCommands(p)
		if cmds != nil {
			commands = append(commands, cmds...)
		}
	}

	log.Debug(ctx, "created plugin commands", "cmds", logCmds(commands))

	store := &Store{
		Plugins:  plugins,
		Commands: commands,
	}

	if len(pathErrs) > 0 {
		return store, pathErrs
	}

	return store, nil
}

// Init loads the required plugins and performs a handshake with them.
// The function uses the command that was run to determine which plugins should
// be loaded.
func (*Store) Init(ctx context.Context, cmd *Command) error {
	// TODO: If the command uses tasks or there is some other reason for it,
	// load more plugins.
	plugins := []Plugin{cmd.Plugin}
	eg, _ := errgroup.WithContext(ctx)

	for _, plugin := range plugins {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			if err := plugin.Start(ctx); err != nil {
				return err
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to init plugins: %w", err)
	}

	return nil
}

// Len returns the number of plugins in the store.
func (s *Store) Len() int {
	return len(s.Plugins)
}

// LogValue implements [slog.LogValuer] for Store. It returns a group value for
// logging a Store.
func (s *Store) LogValue() slog.Value {
	var attrs []slog.Attr

	names := make([]string, len(s.Plugins))
	for i, p := range s.Plugins {
		names[i] = p.Manifest().Name
	}

	attrs = append(attrs, slog.Any("plugins", names), slog.Any("commands", logCmds(s.Commands)))

	return slog.GroupValue(attrs...)
}

// Command returns the command with the given name from the store. If prev is
// nil, the command is looked up from the store root. Otherwise, it is looked up
// from the subcommands of prev.
func (s *Store) Command(prev *Command, name string) *Command {
	var cmds []*Command

	if prev == nil {
		cmds = s.Commands
	} else {
		cmds = prev.Commands
	}

	for _, cmd := range cmds {
		if cmd.Name == name || (len(cmd.Aliases) > 0 && slices.Contains(cmd.Aliases, name)) {
			return cmd
		}
	}

	return nil
}

// validatePlugins checks the created plugins for conflicts. Specifically,
// the plugins may not have duplicate names, domains, or executables.
func validatePlugins(plugins []Plugin) error {
	for _, p1 := range plugins {
		m1 := p1.Manifest()

		for _, p2 := range plugins {
			if p1 == p2 {
				continue
			}

			m2 := p2.Manifest()

			if m1.Name == m2.Name {
				return fmt.Errorf("%w: duplicate plugin name %q", errInvalidManifest, m1.Name)
			}

			if m1.Domain == m2.Domain {
				return fmt.Errorf("%w: duplicate plugin domain %q", errInvalidManifest, m1.Domain)
			}

			if m1.Executable == m2.Executable {
				return fmt.Errorf("%w: duplicate plugin executable path %q", errInvalidManifest, m1.Executable)
			}
		}
	}

	return nil
}

// readAllSearchPaths loads plugins from all of the given search paths.
func readAllSearchPaths(ctx context.Context, wd fspath.Path, paths []fspath.Path) ([]Plugin, error) {
	var (
		mu       sync.Mutex
		errMu    sync.Mutex
		pathErrs PathErrors
		plugins  []Plugin
	)

	g, ctx := errgroup.WithContext(ctx)

	for _, path := range paths {
		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			var err error

			if !path.IsAbs() {
				// TODO: Is this sufficient?
				if strings.HasPrefix(path.String(), "~") {
					path, err = path.Abs()
				} else {
					path, err = fspath.NewAbs(string(wd), string(path))
				}

				if err != nil {
					return fmt.Errorf("failed to create absolute path from %q: %w", path, err)
				}
			}

			path := path.Clean()

			log.Trace(ctx, "checking plugin search path", "path", path)

			if ok, err := path.IsDir(); err != nil {
				return fmt.Errorf("cannot check if %q is a directory: %w", path, err)
			} else if !ok {
				errMu.Lock()
				defer errMu.Unlock()
				pathErrs = append(pathErrs, &PathError{Path: path})
				return nil
			}

			result, err := readSearchPath(ctx, path)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			plugins = append(plugins, result...)

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed to read plugin search paths: %w", err)
	}

	if pathErrs != nil {
		return plugins, pathErrs
	}

	return plugins, nil
}

// readSearchPath reads one search path, checks all of the directories in it and
// creates plugins for all of the found manifests.
func readSearchPath(ctx context.Context, path fspath.Path) ([]Plugin, error) {
	var (
		mu      sync.Mutex
		plugins []Plugin
	)

	dir, err := path.Clean().ReadDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", path, err)
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, dirEntry := range dir {
		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			log.Trace(ctx, "checking dir entry", "path", path, "name", dirEntry.Name())

			if !dirEntry.IsDir() {
				log.Warn(ctx, "dir entry in the plugins directory is not directory", "path", path, "name", dirEntry.Name())

				return nil
			}

			// TODO: Possibly allow using other file formats.
			manifestPath := path.Join(dirEntry.Name(), "manifest.json").Clean()

			plugin, err := readExternalPlugin(ctx, manifestPath)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			plugins = append(plugins, plugin)

			log.Trace(ctx, "loaded external plugin", "plugin", plugin, "manifest", plugin.Manifest())

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("searching plugins from %q failed: %w", path, err)
	}

	return plugins, nil
}

// readExternalPlugin reads a plugin's manifest from path, decodes and validates
// it, and returns an external plugin created from it.
func readExternalPlugin(ctx context.Context, path fspath.Path) (*externalPlugin, error) {
	data, err := path.ReadFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	var manifest *api.Manifest
	if err = d.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode the manifest at %q: %w", path, err)
	}

	log.Trace(ctx, "manifest file decoded", "path", path, "manifest", manifest)

	if manifest.Name == "" {
		return nil, fmt.Errorf("%w: manifest at %q did not specify a name", errInvalidManifest, path)
	}

	if manifest.Domain == "" {
		manifest.Domain = manifest.Name
	}

	if manifest.Executable == "" {
		return nil, fmt.Errorf("%w: manifest at %q did not specify executable", errInvalidManifest, path)
	}

	execPath, err := fspath.NewAbs(string(path.Dir()), manifest.Executable)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create absolute executable path from %q for plugin %q: %w",
			manifest.Executable,
			manifest.Name,
			err,
		)
	}

	if ok, err := execPath.IsFile(); err != nil {
		return nil, fmt.Errorf("failed to check if %q is a file: %w", execPath, err)
	} else if !ok {
		return nil, fmt.Errorf("%w: executable at %q is not a file", errInvalidManifest, execPath)
	}

	manifest.Executable = string(execPath)

	// We need to make sure that there are no nil commands as we decided to
	// panic later if we find them.
	i := 0

	for _, cmd := range manifest.Commands {
		if cmd != nil {
			manifest.Commands[i] = cmd
			i++
		}
	}

	manifest.Commands = manifest.Commands[:i]

	log.Trace(ctx, "manifest for external plugin loaded", "path", path, "manifest", manifest)

	return &externalPlugin{
		manifest: manifest,
		conn:     nil,
		loaded:   false,
	}, nil
}
