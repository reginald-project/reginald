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
	// pluginRuntimes contains the resolved runtimes for the plugins. The keys
	// of the map are the plugin names, and a value is the runtime the plugin
	// requires. All plugins should be registered to this map and if a plugin
	// does not need a runtime, its value here should be nil.
	pluginRuntimes map[string]runtime

	// providers contains the resolved provider tasks for the runtimes for this
	// run. The keys of the map are the runtime names and the values are the
	// task IDs that provide those runtimes.
	providers map[string]string

	// Plugins is the list of plugins.
	Plugins []Plugin

	// Commands is the list of commands that are defined in the plugins.
	Commands []*Command

	// Tasks is the list of tasks that are defined in the plugins.
	Tasks []*Task

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
		Plugins:        plugins,
		Commands:       commands,
		Tasks:          tasks,
		pluginRuntimes: nil,
		providers:      nil,
		sortedTasks:    nil,
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

// Init loads the required plugins and performs a handshake with them. It uses
// the command that should be run and the tasks to determine which plugins
// should be loaded. It also resolves the execution order for the tasks, taking
// the tasks that install the required runtimes into account.
func (s *Store) Init(ctx context.Context, tasks []TaskConfig) error {
	var (
		err   error
		graph taskGraph
	)

	if graph, err = newTaskGraph(tasks); err != nil {
		return err
	}

	// TODO: Should the task order take the required provider tasks into
	// account?
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

// RegisterPluginRuntime registers a runtime for the given plugin. It panics if
// the plugin already has a runtime registered for it.
func (s *Store) RegisterPluginRuntime(rt runtime, plugin Plugin) {
	if s.pluginRuntimes == nil {
		s.pluginRuntimes = make(map[string]runtime)
	}

	if _, ok := s.pluginRuntimes[plugin.Manifest().Name]; ok {
		panic("runtime already registered for " + plugin.Manifest().Name)
	}

	if rt == nil {
		s.pluginRuntimes[plugin.Manifest().Name] = nil
	} else {
		s.pluginRuntimes[plugin.Manifest().Name] = rt
	}
}

// RegisterProvider registers a provider task for the given runtime.
func (s *Store) RegisterProvider(taskID string, runtime runtime) {
	if runtime == nil {
		return
	}

	if s.providers == nil {
		s.providers = make(map[string]string)
	}

	// TODO: I think is definitely not what we want.
	if _, ok := s.providers[runtime.Name()]; ok {
		panic("provider already registered for " + runtime.Name())
	}

	s.providers[runtime.Name()] = taskID
}

// ShutdownAll requests all of the started plugins to shut down and notfies them
// to exit. It will ultimately kill the processes for the plugins that fail to
// shut down gracefully.
func (s *Store) ShutdownAll(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, plugin := range s.Plugins {
		handlePanic := panichandler.WithStackTrace()

		g.Go(func() error {
			defer handlePanic()

			return shutdown(gctx, plugin)
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

// resolveRuntime resolves a missing runtime by finding the providing task and
// installing the runtime using it.
func (s *Store) resolveRuntime(ctx context.Context, rt runtime, tasks []TaskConfig) error {
	if rt == nil {
		return nil
	}

	tID, ok := s.providers[rt.Name()]
	if !ok {
		return fmt.Errorf("%w: %s", errNoProvider, rt.Name())
	}

	var cfg TaskConfig

	ok = false

	for _, tcfg := range tasks {
		if tcfg.ID == tID {
			cfg = tcfg
			ok = true

			break
		}
	}

	if !ok {
		return fmt.Errorf("%w: %s with task ID %s", errNoProvider, rt.Name(), tID)
	}

	task := s.Task(cfg.TaskType)
	if task == nil {
		return fmt.Errorf("%w: %s with task ID %s", errNoProvider, rt.Name(), tID)
	}

	return RunTask(ctx, s, &cfg, tasks)
}

// start resolves the runtime for the given plugin, starts its process, and
// performs the handshake with it.
func (s *Store) start(ctx context.Context, plugin Plugin, tasks []TaskConfig) error {
	slog.InfoContext(ctx, "starting plugin", "plugin", plugin.Manifest().Name)

	if e, ok := plugin.(*externalPlugin); ok && e.cmd != nil {
		// TODO: This might leave some cases where starting the executable does
		// not succeed but those cases might anyway return errors so there might
		// be no point in trying again.
		slog.DebugContext(ctx, "external plugin already started", "plugin", plugin.Manifest().Name)

		return nil
	}

	var (
		ok bool
		rt runtime
	)

	rt, ok = s.pluginRuntimes[plugin.Manifest().Name]
	if !ok {
		panic("runtime not registered for " + plugin.Manifest().Name)
	}

	if rt != nil && !rt.Present() {
		if err := s.resolveRuntime(ctx, rt, tasks); err != nil {
			return err
		}
	}

	var err error

	defer func() {
		if err == nil {
			return
		}

		slog.ErrorContext(ctx, "error when initializing the store, shutting down plugins")

		if err = shutdown(ctx, plugin); err != nil {
			fmt.Fprintf(os.Stderr, "Error when shutting down plugins: %v\n", err)
		}
	}()

	if err = plugin.start(ctx); err != nil {
		return fmt.Errorf("failed to start %q: %w", plugin.Manifest().Name, err)
	}

	if err = callHandshake(ctx, plugin); err != nil {
		return fmt.Errorf("handshake with %q failed: %w", plugin.Manifest().Name, err)
	}

	slog.InfoContext(ctx, "plugin started", "plugin", plugin.Manifest().Name)

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

// shutdown requests the given plugin to shut down and notifies it to exit. It
// will ultimately kill the process if the plugin fails to shut down gracefully
// and the context is canceled.
func shutdown(ctx context.Context, plugin Plugin) error {
	if !plugin.External() {
		slog.Log(
			ctx,
			slog.Level(logger.LevelTrace),
			"nothing to shut down for built-in plugin",
			"plugin",
			plugin.Manifest().Name,
		)

		return nil
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

		return nil
	}

	if err := callShutdown(ctx, external); err != nil {
		return err
	}

	if err := callExit(ctx, external); err != nil {
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
