package adapter

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
)

// Container holds all adapter-layer event stores.
type Container struct {
	ArrowES   asynxModels.Store
	RuntimeES asynxModels.Store
	QuiverES  asynxModels.Store
	closers   []io.Closer
}

// Close closes all event store database connections, checkpointing WAL files and
// releasing file handles. Must be called during shutdown before temp directories
// or process-level cleanup runs.
func (c *Container) Close() error {
	var errs []error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type adapterOpts struct{ homeDir string }

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var.
func WithHomeDir(dir string) Option {
	return func(o *adapterOpts) { o.homeDir = dir }
}

// New constructs all adapter event stores.
func New(opts ...Option) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	var eventsPath string
	var err error
	if cfg.homeDir != "" {
		eventsPath, err = paths.EventsAt(cfg.homeDir)
	} else {
		eventsPath, err = paths.Events()
	}
	if err != nil {
		return nil, fmt.Errorf("adapter: %w", err)
	}

	arrowES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "arrow.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "runtime.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "collection.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
		closers:   []io.Closer{arrowES, runtimeES, quiverES},
	}, nil
}
