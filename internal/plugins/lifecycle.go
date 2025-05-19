package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/anttikivi/reginald/internal/panichandler"
	"golang.org/x/sync/errgroup"
)

// Load creates the processes for the plugins, performs the handshakes with
// them, returns a slice of the valid plugins.
func Load(ctx context.Context, files []string) ([]*Plugin, error) {
	// TODO: Provide the config value for the create function.
	plugins, err := loadAll(ctx, files, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load the plugins: %w", err)
	}

	ph := panichandler.WithStackTrace()
	for _, p := range plugins {
		go func(p *Plugin) {
			defer ph()

			panic("test")

			if err := <-p.doneCh; err != nil {
				// TODO: Better logging or something.
				fmt.Fprintf(os.Stderr, "plugin %q quit unexpectedly: %v\n", p.name, err)
			}
		}(p)
	}

	return plugins, nil
}

// ShutdownAll tries to gracefully shut down all of the plugins.
func ShutdownAll(ctx context.Context, plugins []*Plugin) error {
	slog.Info("shutting down plugins")

	eg, egctx := errgroup.WithContext(ctx)

	for _, p := range plugins {
		eg.Go(func() error {
			if err := p.shutdown(egctx); err != nil {
				return fmt.Errorf("%w", err)
			}

			slog.Info("shutdown successful", "plugin", p.name)

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("%w", err)
	}

	slog.Info("all plugins shut down successfully")

	return nil
}

// loadAll creates and starts all of the plugin processes and performs the
// handshake with them. If ignoreErrors is true, the function simply drops
// plugins that cause errors when starting or fail the handshake. Otherwise the
// function fails fast.
func loadAll(ctx context.Context, files []string, ignoreErrors bool) ([]*Plugin, error) {
	var (
		lock    sync.Mutex
		plugins []*Plugin
	)

	eg, egctx := errgroup.WithContext(ctx)

	// TODO: Print the errors to actual output if they are ignored.
	for _, f := range files {
		eg.Go(func() error {
			p, err := New(ctx, f)
			if err != nil {
				return fmt.Errorf("failed to create a new plugin for path %s; %w", f, err)
			}

			if err := p.start(ctx); err != nil {
				if ignoreErrors {
					slog.Warn("failed to start plugin", "path", f, "err", err)

					return nil
				}

				return fmt.Errorf("failed to start plugin %s: %w", p.name, err)
			}

			if err := p.handshake(egctx); err != nil {
				if ignoreErrors {
					slog.Warn("handshake failed", "path", f, "err", err)

					return nil
				}

				return fmt.Errorf("handshake for plugin %s failed: %w", p.name, err)
			}

			// I'm not sure about using locks but it's simple and gets the job
			// done.
			lock.Lock()

			plugins = append(plugins, p)

			lock.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return plugins, nil
}
