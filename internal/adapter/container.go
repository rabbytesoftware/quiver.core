package adapter

import (
	"fmt"
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

	quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "quiver.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: quiver event store: %w", err)
	}

	return &Container{
		ArrowES:   arrowES,
		RuntimeES: runtimeES,
		QuiverES:  quiverES,
	}, nil
}
