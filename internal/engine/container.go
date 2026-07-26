package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// Container holds all engine-layer dependencies.
type Container struct {
	Vault     vault.Vault
	Manifold  manifold.Manifold
	Wizard    wizard.Wizard
	Netbridge netbridge.Netbridge
	DepTree   deptree.DepTree
	// Providers holds one entry per platform that declares a search URL.
	// A fetch-only platform is not a discovery provider and is absent here.
	Providers []provider.Provider
}

const defaultProviderTimeout = 10 * time.Second

type engineOpts struct{ homeDir string }

// Option configures engine.New.
type Option func(*engineOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var.
func WithHomeDir(dir string) Option {
	return func(o *engineOpts) { o.homeDir = dir }
}

func (c *Container) Start(ctx context.Context) {
	c.Vault.Start(ctx)
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

	wiz, err := wizard.New(nil)
	if err != nil {
		return nil, fmt.Errorf("engine container: wizard: %w", err)
	}

	fetchTimeout, err := time.ParseDuration(config.GetManifold().FetchTimeout)
	if err != nil {
		fetchTimeout = 30 * time.Second
	}

	vaultPath := metadata.GetVaultPath()
	namespacesPath := metadata.GetNamespacesPath()
	if cfg.homeDir != "" {
		vaultPath = metadata.GetVaultPathAt(cfg.homeDir)
		namespacesPath = metadata.GetNamespacesPathAt(cfg.homeDir)
	}

	v, err := vault.New(vaultPath, namespacesPath, 0)
	if err != nil {
		return nil, fmt.Errorf("engine container: vault: %w", err)
	}

	providers, err := newProviders(metadata.GetPlatforms(), config.GetSearch())
	if err != nil {
		return nil, fmt.Errorf("engine container: providers: %w", err)
	}

	return &Container{
		Vault:     v,
		Manifold:  manifold.New(fetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
		Providers: providers,
	}, nil
}

func newProviders(
	platforms metadata.Platforms,
	search config.Search,
) ([]provider.Provider, error) {
	timeout, err := time.ParseDuration(search.ProviderTimeout)
	if err != nil {
		timeout = defaultProviderTimeout
	}

	return provider.FromPlatforms(platforms, provider.Config{
		Timeout: timeout,
	})
}
