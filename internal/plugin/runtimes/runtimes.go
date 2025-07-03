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

// Package runtimes provides utilities for working with the plugin runtimes.
package runtimes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/anttikivi/semver"
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/logger"
	"github.com/reginald-project/reginald/internal/plugin"
	"github.com/reginald-project/reginald/internal/terminal"
)

// Provider-related errors.
var (
	errManyProviders = errors.New("many providers for runtime")    // multiple possible providers for a runtime
	errNoProvider    = errors.New("no provider found for runtime") // no task provides wanted runtime
)

// runtimes contains the runtimes that are requested for the current
// configuration.
var runtimes map[string]*runtime //nolint:gochecknoglobals // single runtime instances

// A runtime represents a runtime that can run plugins.
type runtime struct {
	name     string
	versions []*semver.Version // all of the requested versions
	aliases  []string          // other names for the runtime, e.g. "python3" for python
	found    bool              // whether the runtime was found on the system
}

// Resolve resolves the plugins that require a runtime. It checks if there is
// another task that provides that runtime and registers it to be run before
// starting the plugins that require the provided runtime. If there is no task
// that provides the required runtime and Reginald was run in interactive mode,
// it asks the user if they want to add a task that provides that runtime.
//
// Resolve modifies store and cfg.
func Resolve(ctx context.Context, store *plugin.Store, cfg *config.Config) error {
	for _, p := range store.Plugins {
		apiRuntime := p.Manifest().Runtime
		if apiRuntime == nil || apiRuntime.Name == "" {
			slog.Log(ctx, slog.Level(logger.LevelTrace), "plugin has no runtime, skipping", "plugin", p.Manifest().Name)

			continue
		}

		if detect(apiRuntime) {
			slog.Log(ctx, slog.Level(logger.LevelTrace), "plugin runtime already present", "plugin", p.Manifest().Name)

			continue
		}

		rt := fromAPI(apiRuntime)
		if rt == nil {
			panic("nil runtime for already accessed one")
		}

		if err := pluginProvider(ctx, rt, p, store, cfg); err != nil {
			return err
		}
	}

	slog.InfoContext(ctx, "resolved all runtimes")

	return nil
}

// addProviderTask creates and adds a task instance of the task type that
// the user has selected for a plugin whose runtime wasn't found and for which
// there was no provider task. It adds it to the configuration and returns
// the ID for the created task instance.
func addProviderTask(ctx context.Context, taskType string, store *plugin.Store, cfg *config.Config) (string, error) {
	baseID := taskType + "-provider"
	id := baseID
	i := 0

	for _, t := range cfg.Tasks {
		if t.ID == id {
			id = baseID + "-" + strconv.Itoa(i)
			i++
		}
	}

	rawCfg := []map[string]any{
		{
			"type": taskType,
			"id":   id,
		},
	}

	cfgs, err := config.ApplyTasks(ctx, rawCfg, config.TaskApplyOptions{
		Store:    store,
		Defaults: cfg.Defaults,
		Dir:      cfg.Directory,
	})
	if err != nil {
		return "", fmt.Errorf("%w: failed to create config for new provider task", err)
	}

	cfg.Tasks = append(cfg.Tasks, cfgs...)

	return cfgs[0].ID, nil
}

// detect reports whether a runtime matching the given API runtime specification
// is found on the system.
func detect(apiRuntime *api.Runtime) bool {
	if apiRuntime == nil {
		panic("nil API runtime in fromAPI")
	}

	r := fromAPI(apiRuntime)
	if r.found {
		return true
	}

	_, err := exec.LookPath(r.name)
	if err != nil {
		return false
	}

	r.found = true

	return true
}

// findProviderTypes finds task types that can provide the given runtime.
func findProviderTypes(tasks []*plugin.Task, rt *runtime) []*plugin.Task {
	if len(tasks) == 0 {
		panic("empty tasks")
	}

	var ts []*plugin.Task

	for _, t := range tasks {
		if rt.name == normalizeName(t.Provides) {
			ts = append(ts, t)
		}
	}

	return ts
}

// fromAPI creates a new runtime for the given API runtime specification or
// looks it up from the runtimes map if it is already registered there.
func fromAPI(apiRuntime *api.Runtime) *runtime {
	if apiRuntime == nil {
		panic("nil API runtime in fromAPI")
	}

	name := normalizeName(apiRuntime.Name)

	r := runtimes[name]
	if r == nil {
		r = &runtime{
			name:     name,
			versions: nil,
			aliases:  []string{name},
			found:    false,
		}
	}

	v, err := semver.ParseLax(apiRuntime.Version)
	if err == nil && !slices.ContainsFunc(r.versions, func(w *semver.Version) bool { return v.Equal(w) }) {
		r.versions = append(r.versions, v)
	}

	if !slices.Contains(r.aliases, apiRuntime.Name) {
		r.aliases = append(r.aliases, apiRuntime.Name)
	}

	return r
}

// normalizeName returns the normalized name for the given runtime name.
// The runtimes are stored and accessed by their normalized names.
func normalizeName(name string) string {
	switch strings.ToLower(name) {
	case "node", "node.js":
		return "node"
	case "python", "python3":
		return "python"
	default:
		return name
	}
}

// pluginProvider resolves the provider for a single plugin and registers it.
//
//nolint:varnamelen
func pluginProvider(ctx context.Context, rt *runtime, p plugin.Plugin, store *plugin.Store, cfg *config.Config) error {
	var providers []string //nolint:prealloc // we don't know the size

	for _, taskCfg := range cfg.Tasks {
		task := store.Task(taskCfg.TaskType)
		if task == nil {
			panic("no task for type " + taskCfg.TaskType)
		}

		if normalizeName(task.Provides) != rt.name {
			continue
		}

		providers = append(providers, taskCfg.ID)
	}

	if len(providers) > 1 {
		return fmt.Errorf("%w: %s for %s", errManyProviders, strings.Join(providers, " "), rt.name)
	}

	if len(providers) == 1 {
		store.AddProvider(providers[0], p)

		return nil
	}

	if !cfg.Interactive {
		return fmt.Errorf("%w: %s for %s", errNoProvider, rt.name, p.Manifest().Name)
	}

	ts := findProviderTypes(store.Tasks, rt)
	if len(ts) == 0 {
		return fmt.Errorf("%w: %s", errNoProvider, rt.name)
	}

	terminal.Printf("Found multiple provider tasks for runtime %q required by %s\n", rt.name, p.Manifest().Name)

	var (
		list    string
		options string
	)

	for i, provider := range providers {
		list = fmt.Sprintf("%d: %s\n", i+1, provider)
		options += strconv.Itoa(i+1) + "/"
	}

	options += "/n"

	var (
		answer string
		err    error
		i      int
	)

	prompt := fmt.Sprintf("Choose which task to use the provider [%s]: ", options)

	for {
		terminal.Print(list)
		terminal.Flush()

		answer, err = terminal.Ask(ctx, prompt)
		if err != nil {
			return fmt.Errorf(
				"%w: failed to ask for provider for %s for %q: %w",
				errNoProvider,
				rt.name,
				p.Manifest().Name,
				err,
			)
		}

		answer = strings.ToLower(answer)
		if answer == "no" || answer == "n" {
			return nil
		}

		i, err = strconv.Atoi(answer)
		if err != nil {
			continue
		}

		if 0 < i && i <= len(providers) {
			i--

			break
		}
	}

	var id string

	if id, err = addProviderTask(ctx, providers[i], store, cfg); err != nil {
		return err
	}

	store.AddProvider(id, p)

	return nil
}
