package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/anttikivi/reginald/internal/iostreams"
	"github.com/anttikivi/reginald/internal/panichandler"
	"golang.org/x/sync/errgroup"
)

// Load creates the processes for the plugins, performs the handshakes with
// them, returns a slice of the valid plugins.
func Load(ctx context.Context, files []string) ([]*Plugin, error) {
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
				slog.ErrorContext(ctx, "plugin quit unexpectedly", "plugin", p.name, "err", err)
				iostreams.Errorf("Plugin %q quit unexpectedly", p.name)
				iostreams.PrintErrf("Error: %v\n", err)
			}
		}()
	}

	slog.InfoContext(ctx, "plugins loaded", "plugins", plugins)

	return plugins, nil
}

// ShutdownAll tries to gracefully shut down all of the plugins.
func ShutdownAll(ctx context.Context, plugins []*Plugin) error {
	slog.InfoContext(ctx, "shutting down plugins")

	eg, gctx := errgroup.WithContext(ctx)

	for _, p := range plugins {
		handlePanic := panichandler.WithStackTrace()

		eg.Go(func() error {
			defer handlePanic()

			if err := p.shutdown(gctx); err != nil {
				return fmt.Errorf("%w", err)
			}

			slog.DebugContext(gctx, "shutdown successful", "plugin", p.name)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%w", err)
	}

	slog.InfoContext(ctx, "all plugins shut down successfully")

	return nil
}

// loadAll creates and starts all of the plugin processes and performs the
// handshake with them. If ignoreErrors is true, the function simply drops
// plugins that cause errors when starting or fail the handshake. Otherwise the
// function fails fast.
func loadAll(ctx context.Context, files []string, ignoreErrors bool) ([]*Plugin, error) {
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
					slog.ErrorContext(tctx, "failed to start plugin", "path", f, "err", err)
					iostreams.Errorf("Failed to start plugin %q\n", f)
					iostreams.PrintErrf("Error: %v\n", err)

					return nil
				}

				return fmt.Errorf("failed to start plugin %s: %w", p.name, err)
			}

			if err := p.handshake(tctx); err != nil {
				if ignoreErrors {
					slog.ErrorContext(tctx, "handshake failed", "path", f, "err", err)
					iostreams.Errorf("Handshake with %q failed\n", f)
					iostreams.PrintErrf("Error: %v\n", err)

					return nil
				}

				return fmt.Errorf("handshake with plugin %q failed: %w", p.name, err)
			}

			// I'm not sure about using locks but it's simple and gets the job
			// done.
			mu.Lock()

			plugins = append(plugins, p)

			mu.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return plugins, nil
}
