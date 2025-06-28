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
	"sync/atomic"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald-sdk-go/logs"
	"github.com/reginald-project/reginald/internal/builtin"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/fsutil"
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

	// Tasks is the list of tasks that are defined in the plugins.
	Tasks []*Task
}

// NewStore finds the available built-in and external plugin manifests from
// the given search paths, loads and decodes them, and returns a new Store with
// the plugins created from them.
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

	if err := validate(plugins); err != nil {
		return nil, err
	}

	var (
		commands []*Command
		tasks    []*Task
	)

	for _, p := range plugins {
		cmds := newCommands(p)
		if cmds != nil {
			commands = append(commands, cmds...)
		}

		t := newTasks(p)
		if t != nil {
			tasks = append(tasks, t...)
		}
	}

	log.Debug(ctx, "created plugin commands", "cmds", logCmds(commands))
	log.Debug(ctx, "created plugin tasks", "tasks", logTasks(tasks))

	store := &Store{
		Plugins:  plugins,
		Commands: commands,
		Tasks:    tasks,
	}

	if len(pathErrs) > 0 {
		return store, pathErrs
	}

	return store, nil
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

			if err := plugin.start(ctx); err != nil {
				return fmt.Errorf("failed to start %q: %w", plugin.Manifest().Name, err)
			}

			if err := handshake(ctx, plugin); err != nil {
				return fmt.Errorf("handshake with %q failed: %w", plugin.Manifest().Name, err)
			}

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to init plugins: %w", err)
	}

	log.Debug(ctx, "plugins started")

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

// Shutdown requests all of the started plugins to shut down and notfies them to
// exit. It will ultimately kill the processes for the plugins that fail to shut
// down gracefully.
func (s *Store) Shutdown(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, plugin := range s.Plugins {
		if !plugin.External() {
			log.Trace(ctx, "shutting down built-in plugin", "no-op", true, "plugin", plugin.Manifest().Name)

			continue
		}

		external, ok := plugin.(*externalPlugin)
		if !ok {
			return fmt.Errorf(
				"%w: plugin %q cannot be converted to *externalPlugin",
				errInvalidCast,
				plugin.Manifest().Name,
			)
		}

		if external.cmd == nil {
			log.Trace(ctx, "skipping shutdown as process was not started", "plugin", external.manifest.Name)

			continue
		}

		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			if err := shutdown(gctx, external); err != nil {
				return err
			}

			if err := exit(ctx, external); err != nil {
				return err
			}

			select {
			case err := <-external.doneCh:
				if err != nil {
					return fmt.Errorf("process for plugin %q returned error: %w", external.manifest.Name, err)
				}
			case <-ctx.Done():
				if err := external.kill(ctx); err != nil {
					return fmt.Errorf("failed to kill plugin %q: %w", external.manifest.Name, err)
				}

				return fmt.Errorf("shutting down plugin %q halted: %w", external.manifest.Name, ctx.Err())
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to shut down plugins: %w", err)
	}

	log.Debug(ctx, "plugins shut down")

	return nil
}

// Task returns that task with the given task type from the store. The task type
// must be the full-qualified task type meaning that it must be specified as
// "<domain>/<task>".
func (s *Store) Task(tt string) *Task {
	i := strings.IndexByte(tt, '/')
	if i == -1 {
		return nil
	}

	domain := tt[:i]
	taskType := tt[i+1:]

	for _, t := range s.Tasks {
		if t.Plugin.Manifest().Domain == domain && t.Type == taskType {
			return t
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

			path = path.Clean()

			log.Trace(ctx, "checking plugin search path", "path", path)

			var ok bool

			if ok, err = path.IsDir(); err != nil {
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
				log.Warn(
					ctx,
					"dir entry in the plugins directory is not directory",
					"path",
					path,
					"name",
					dirEntry.Name(),
				)

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
		conn:     nil,
		cmd:      nil,
		doneCh:   make(chan error),
		lastID:   atomic.Int64{},
		manifest: manifest,
		queue: &responseQueue{
			q:  make(map[string]chan api.Response),
			mu: sync.Mutex{},
		},
	}, nil
}

// validate checks the created plugins for conflicts. Specifically, the plugins
// may not have duplicate names, domains, or executables.
func validate(plugins []Plugin) error {
	seenNames := make(map[string]struct{})
	seenDomains := make(map[string]struct{})
	seenExecutables := make(map[fsutil.FileID]string)

	for _, p := range plugins {
		m := p.Manifest()

		if _, ok := seenNames[m.Name]; ok {
			return fmt.Errorf("%w: duplicate plugin name %q", errInvalidManifest, m.Name)
		}

		seenNames[m.Name] = struct{}{}

		if _, ok := seenDomains[m.Domain]; ok {
			return fmt.Errorf("%w: duplicate plugin domain %q", errInvalidManifest, m.Domain)
		}

		seenDomains[m.Domain] = struct{}{}

		if !p.External() {
			continue
		}

		id, err := fsutil.ID(m.Executable)
		if err != nil {
			return fmt.Errorf("failed create file ID: %w", err)
		}

		if firstPath, ok := seenExecutables[id]; ok {
			return fmt.Errorf(
				"%w: executable for plugin %q (%s) is the same as the executable for another plugin (%q)",
				errInvalidManifest,
				m.Name,
				m.Executable,
				firstPath,
			)
		}

		seenExecutables[id] = m.Executable
	}

	return nil
}
