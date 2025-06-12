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

package plugins

import (
	"context"
	"fmt"
	"sync"

	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/logging"
	"github.com/reginald-project/reginald/internal/panichandler"
	"github.com/reginald-project/reginald/internal/terminal"
	"github.com/reginald-project/reginald/pkg/rpp"
	"golang.org/x/sync/errgroup"
)

// Initialize calls the "initialize" method on all plugins.
func Initialize(ctx context.Context, plugins []*Plugin, cfgs map[string]any) error {
	eg, gctx := errgroup.WithContext(ctx)

	for _, p := range plugins {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			// TODO: Allow configuring the timeout.
			tctx, cancel := context.WithTimeout(gctx, DefaultHandshakeTimeout)
			defer cancel()

			var cfg []rpp.ConfigEntry

			if c, ok := cfgs[p.Name].(map[string]any); ok {
				for k, v := range c {
					cfgVal, err := rpp.NewConfigEntry(k, v)
					if err != nil {
						return fmt.Errorf("%w", err)
					}

					cfg = append(cfg, cfgVal)
				}
			}

			logging.TraceContext(ctx, "cfgs", "cfgs", cfgs)

			if err := p.initialize(tctx, cfg); err != nil {
				// if ignoreErrors {
				// 	logging.ErrorContext(tctx, "failed to initialize plugin", "path", f, "err", err)
				// 	iostreams.Errorf("Failed to initialize plugin %q\n", f)
				// 	iostreams.PrintErrf("Error: %v\n", err)
				//
				// 	return nil
				// }
				return fmt.Errorf("failed to initialize plugin %s: %w", p.Name, err)
			}

			logging.DebugContext(gctx, "plugin initialized", "plugin", p)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

// Load creates the processes for the plugins, performs the handshakes with
// them, returns a slice of the valid plugins.
func Load(ctx context.Context, files []fspath.Path) ([]*Plugin, error) {
	// TODO: Add a config options for ignoring the errors.
	plugins, err := loadAll(ctx, files, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load the plugins: %w", err)
	}

	for _, p := range plugins {
		handlePanic := panichandler.WithStackTrace()
		go func() {
			defer handlePanic()

			if err := <-p.doneCh; err != nil {
				logging.ErrorContext(ctx, "plugin quit unexpectedly", "plugin", p.Name, "err", err)
				terminal.Errorf("Plugin %q quit unexpectedly", p.Name)
				terminal.PrintErrf("Error: %v\n", err)
			}
		}()
	}

	logging.InfoContext(ctx, "plugins loaded", "plugins", len(plugins))

	return plugins, nil
}

// ShutdownAll tries to gracefully shut down all of the plugins.
func ShutdownAll(ctx context.Context, plugins []*Plugin) error {
	logging.InfoContext(ctx, "shutting down plugins")

	eg, gctx := errgroup.WithContext(ctx)

	for _, p := range plugins {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			if err := p.shutdown(gctx); err != nil {
				return fmt.Errorf("%w", err)
			}

			logging.DebugContext(gctx, "shutdown successful", "plugin", p.Name)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%w", err)
	}

	logging.InfoContext(ctx, "all plugins shut down successfully")

	return nil
}

// loadAll creates and starts all of the plugin processes and performs the
// handshake with them. If ignoreErrors is true, the function simply drops
// plugins that cause errors when starting or fail the handshake. Otherwise the
// function fails fast.
func loadAll(ctx context.Context, files []fspath.Path, ignoreErrors bool) ([]*Plugin, error) {
	var (
		mu      sync.Mutex
		plugins []*Plugin
	)

	eg, gctx := errgroup.WithContext(ctx)

	// TODO: See if the error messages should be warnings instead of errors if
	// errors are ignored (not really that big of a difference).
	for _, f := range files {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			p, err := New(ctx, f)
			if err != nil {
				return fmt.Errorf("failed to create a new plugin for path %s; %w", f, err)
			}

			// TODO: Allow configuring the timeout.
			tctx, cancel := context.WithTimeout(gctx, DefaultHandshakeTimeout)
			defer cancel()

			if err := p.start(tctx); err != nil {
				if ignoreErrors {
					logging.ErrorContext(tctx, "failed to start plugin", "path", f, "err", err)
					terminal.Errorf("Failed to start plugin %q\n", f)
					terminal.PrintErrf("Error: %v\n", err)

					return nil
				}

				return fmt.Errorf("failed to start plugin %s: %w", p.Name, err)
			}

			if err := p.handshake(tctx); err != nil {
				if ignoreErrors {
					logging.ErrorContext(tctx, "handshake failed", "path", f, "err", err)
					terminal.Errorf("Handshake with %q failed\n", f)
					terminal.PrintErrf("Error: %v\n", err)

					return nil
				}

				return fmt.Errorf("handshake with plugin %q failed: %w", p.Name, err)
			}

			logging.DebugContext(gctx, "plugin loaded", "plugin", p)

			// I'm not sure about using locks but it's simple and gets the job
			// done.
			mu.Lock()
			defer mu.Unlock()

			plugins = append(plugins, p)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return plugins, nil
}
