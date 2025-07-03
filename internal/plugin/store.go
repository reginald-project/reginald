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
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/fsutil"
	"github.com/reginald-project/reginald/internal/logger"
	"github.com/reginald-project/reginald/internal/panichandler"
	"golang.org/x/sync/errgroup"
)

// A Store stores the plugins, provides information on them, and has functions
// for using the plugins within the program.
type Store struct {
	// providers contains the resolved and required provider tasks for
	// the plugins for this run. The keys of the map are the plugin names and
	// the values are the task IDs that provide the runtimes for those plugins.
	providers map[string]string

	// Plugins is the list of plugins.
	Plugins []Plugin

	// Commands is the list of commands that are defined in the plugins.
	Commands []*Command

	// Tasks is the list of tasks that are defined in the plugins.
	Tasks []*Task

	// runtimes contains the registered runtimes for the plugins.
	runtimes []runtime

	// deferredStart is a list of plugin names whose startup should be postponed
	// due to a missing runtime.
	deferredStart []string

	// sortedTasks contains the tasks sorted into the correct execution order.
	// Each member slice of the slice contains tasks that can be executed in
	// parallel after the tasks in the slice before them are executed.
	sortedTasks [][]*taskNode
}

// NewStore finds the available built-in and external plugin manifests from
// the given search paths, loads and decodes them, and returns a new Store with
// the plugins created from them.
func NewStore(ctx context.Context, builtin []*api.Manifest, wd fspath.Path, paths []fspath.Path) (*Store, error) {
	// The built-in plugins should be added first as they are already included
	// with the program. The external plugins are validated while they are being
	// loaded so by loading the built-in plugins first, we can make sure that no
	// external plugin collides with them.
	manifests := slices.Clone(builtin)
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

	slog.Log(ctx, slog.Level(logger.LevelTrace), "created commands", "cmds", logCmds(commands))
	slog.Log(ctx, slog.Level(logger.LevelTrace), "created tasks", "tasks", logTasks(tasks))

	store := &Store{
		Plugins:       plugins,
		Commands:      commands,
		Tasks:         tasks,
		deferredStart: nil,
		providers:     nil,
		runtimes:      nil,
		sortedTasks:   nil,
	}

	if len(pathErrs) > 0 {
		return store, pathErrs
	}

	return store, nil
}

// AddProvider registers a provider task for the given plugin.
func (s *Store) AddProvider(taskID string, plugin Plugin) {
	if s.providers == nil {
		s.providers = make(map[string]string)
	}

	if _, ok := s.providers[plugin.Manifest().Name]; ok {
		panic("provider already registered for " + plugin.Manifest().Name)
	}

	s.providers[plugin.Manifest().Name] = taskID
	s.deferStart(plugin)
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

// Init loads the required plugins and performs a handshake with them. It uses
// the command that should be run and the tasks to determine which plugins
// should be loaded. It also resolves the execution order for the tasks, taking
// the tasks that install the required runtimes into account.
func (s *Store) Init(ctx context.Context, cmd *Command, tasks []TaskConfig) error {
	plugins := []Plugin{cmd.Plugin}

	// TODO: This is a hack, there might be a better way.
	if cmd.Name == "attend" && cmd.Plugin.Manifest().Domain == "core" {
		for _, tcfg := range tasks {
			task := s.Task(tcfg.TaskType)
			if task == nil {
				panic("task config has non-existent task type: " + tcfg.TaskType)
			}

			plugin := task.Plugin
			if !slices.ContainsFunc(
				plugins,
				func(p Plugin) bool { return plugin.Manifest().Name == p.Manifest().Name },
			) {
				plugins = append(plugins, plugin)
			}
		}
	}

	// The provider tasks must also be started.
	plugins = append(plugins, s.neededForProvider(plugins, tasks)...)

	g, gctx := errgroup.WithContext(ctx)

	var err error

	defer func() {
		if err == nil {
			return
		}

		slog.ErrorContext(ctx, "error when initializing the store, shutting down plugins")

		if err = s.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error when shutting down plugins: %v\n", err)
		}
	}()

	for _, plugin := range plugins {
		if slices.Contains(s.deferredStart, plugin.Manifest().Name) {
			slog.DebugContext(ctx, "plugin startup deferred", "plugin", plugin.Manifest().Name)

			continue
		}

		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			if err2 := plugin.start(ctx); err2 != nil {
				return fmt.Errorf("failed to start %q: %w", plugin.Manifest().Name, err2)
			}

			if err2 := handshake(gctx, plugin); err2 != nil {
				return fmt.Errorf("handshake with %q failed: %w", plugin.Manifest().Name, err2)
			}

			slog.InfoContext(gctx, "plugin started", "plugin", plugin.Manifest().Name)

			return nil
		})
	}

	if err = g.Wait(); err != nil {
		return fmt.Errorf("failed to init plugins: %w", err)
	}

	var graph taskGraph

	if graph, err = newTaskGraph(tasks); err != nil {
		return err
	}

	if s.sortedTasks, err = graph.sorted(); err != nil {
		return err
	}

	slog.Log(ctx, slog.Level(logger.LevelTrace), "task execution order computed")

	// Stupidly wasteful but provides nicer messages.
	for i, s := range s.sortedTasks {
		ids := make([]string, len(s))

		for j, n := range s {
			ids[j] = n.id
		}

		slog.Log(ctx, slog.Level(logger.LevelTrace), "task stage", "n", i+1, "id", ids)
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

// Shutdown requests all of the started plugins to shut down and notfies them to
// exit. It will ultimately kill the processes for the plugins that fail to shut
// down gracefully.
func (s *Store) Shutdown(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, plugin := range s.Plugins {
		if !plugin.External() {
			slog.Log(
				ctx,
				slog.Level(logger.LevelTrace),
				"nothing to shut down for built-in plugin",
				"plugin",
				plugin.Manifest().Name,
			)

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
			slog.DebugContext(ctx, "skipping plugin shutdown as it was never started", "plugin", external.manifest.Name)

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

	return nil
}

// Task returns that task with the given task type from the store. The task type
// must be the full-qualified task type meaning that it must be specified as
// "<domain>/<task>".
func (s *Store) Task(tt string) *Task {
	for _, t := range s.Tasks {
		if t.TaskType == tt {
			return t
		}
	}

	return nil
}

// deferStart marks a plugin to be started later because it requires a runtime
// that is not present.
func (s *Store) deferStart(plugin Plugin) {
	if !slices.Contains(s.deferredStart, plugin.Manifest().Name) {
		s.deferredStart = append(s.deferredStart, plugin.Manifest().Name)
	}
}

// neededForProvider returns the list plugins that are required by the given
// plugins for providers. The return value only includes the newly resolved
// plugins and not the ones that were passed in.
func (s *Store) neededForProvider(plugins []Plugin, tasks []TaskConfig) []Plugin {
	var result []Plugin //nolint:prealloc // we don't know the size

	for _, plugin := range plugins {
		tID, ok := s.providers[plugin.Manifest().Name]
		if !ok {
			continue
		}

		var tt string

		for _, cfg := range tasks {
			if cfg.ID == tID {
				tt = cfg.TaskType

				break
			}
		}

		if tt == "" {
			panic(fmt.Sprintf("provider task %q with no corresponding config", tID))
		}

		task := s.Task(tt)
		p := task.Plugin

		ok = slices.ContainsFunc(
			append(plugins, result...),
			func(o Plugin) bool { return o.Manifest().Name == p.Manifest().Name },
		)
		if ok {
			continue
		}

		result = append(result, p)
	}

	if len(result) == 0 {
		return nil
	}

	next := s.neededForProvider(result, tasks)
	if len(next) > 0 {
		result = append(result, next...)
	}

	return result
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

			slog.Log(ctx, slog.Level(logger.LevelTrace), "checking plugin search path", "path", path)

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

	dir, err := os.ReadDir(string(path.Clean()))
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", path, err)
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, dirEntry := range dir {
		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			slog.Log(ctx, slog.Level(logger.LevelTrace), "checking dir entry", "path", path, "name", dirEntry.Name())

			if !dirEntry.IsDir() {
				slog.DebugContext(
					ctx,
					"skipping dir entry that is not a directory",
					"path",
					path,
					"name",
					dirEntry.Name(),
				)

				return nil
			}

			// TODO: Possibly allow using other file formats.
			manifestPath := path.Join(dirEntry.Name(), "manifest.json").Clean()

			plugin, err := readExternalPlugin(manifestPath)
			if err != nil {
				return err
			}

			mu.Lock()
			defer mu.Unlock()

			plugins = append(plugins, plugin)

			slog.Log(
				ctx,
				slog.Level(logger.LevelTrace),
				"loaded external plugin manifest",
				"plugin",
				plugin,
				"manifest",
				plugin.Manifest(),
			)

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
func readExternalPlugin(path fspath.Path) (*externalPlugin, error) {
	data, err := os.ReadFile(string(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}

	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	var manifest *api.Manifest
	if err = d.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode the manifest at %q: %w", path, err)
	}

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
