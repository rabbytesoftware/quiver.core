package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Container holds all engine-layer dependencies.
type Container struct {
	Vault     vault.Vault
	Manifold  manifold.Manifold
	Wizard    wizard.Wizard
	Netbridge netbridge.Netbridge
	DepTree   deptree.DepTree
}

type engineOpts struct{ homeDir string }

// Option configures engine.New.
type Option func(*engineOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var.
func WithHomeDir(dir string) Option {
	return func(o *engineOpts) { o.homeDir = dir }
}

// New constructs all engines and returns a ready-to-use Container.
func New(ctx context.Context, opts ...Option) (*Container, error) {
	cfg := engineOpts{}
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
		return nil, fmt.Errorf("engine container: %w", err)
	}

	es, err := sqlite.NewEventStore(filepath.Join(
		eventsPath,
		"netbridge.db",
	))
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	nb, err := netbridge.New().WithEventStore(es).Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine container: netbridge: %w", err)
	}

	wiz, err := wizard.New()
	if err != nil {
		return nil, fmt.Errorf("engine container: wizard: %w", err)
	}

	fetchTimeout, err := time.ParseDuration(config.GetManifold().FetchTimeout)
	if err != nil {
		fetchTimeout = 30 * time.Second
	}

	return &Container{
		Vault:     vault.New("", "", 0),
		Manifold:  manifold.New(fetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}
