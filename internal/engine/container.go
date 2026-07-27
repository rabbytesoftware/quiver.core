package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
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

	netbridgeEvents    io.Closer
	netbridgeSnapshots io.Closer
}

const defaultProviderTimeout = 10 * time.Second

// closeTimeout bounds one handle release. Nothing is in flight by the time the
// releases run, so it guards against a wedged close rather than being a budget
// any close is expected to spend.
const closeTimeout = 5 * time.Second

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

// Shutdown drains netbridge's aggregate, then releases every database handle
// this container opened: netbridge's events and snapshots, and the vault index.
//
// It must run after the app layer has drained: the runtime assembler allocates
// ports through netbridge and the arrow resolver writes manifests into the vault
// index, so an in-flight install still needs both. The vault releases last for
// the same reason within this layer — it is the handle the others might still
// reach through.
//
// The release runs whatever the drain did with ctx, including exhaust it. A
// close is local and immediate; skipping one because a sibling ran long is how a
// database file ends up outliving the process that opened it, which POSIX hides
// and Windows does not.
func (c *Container) Shutdown(ctx context.Context) error {
	drainErr := c.drain(ctx)
	return errors.Join(drainErr, c.release())
}

// drain flushes the netbridge aggregate under the caller's whole budget: it is
// the only step here that waits on anything.
func (c *Container) drain(ctx context.Context) error {
	if err := c.Netbridge.Shutdown(ctx); err != nil {
		return fmt.Errorf("engine container: netbridge shutdown: %w", err)
	}
	return nil
}

// release closes every handle, each under a bound of its own so one that wedges
// loses its turn rather than the handles after it.
func (c *Container) release() error {
	return shutdown.Sequence("engine container", []shutdown.Phase{
		{Name: "netbridge events close", Timeout: closeTimeout, Run: closePhase(c.netbridgeEvents)},
		{Name: "netbridge snapshots close", Timeout: closeTimeout, Run: closePhase(c.netbridgeSnapshots)},
		{Name: "vault close", Timeout: closeTimeout, Run: closePhase(c.Vault)},
	})
}

// closePhase adapts a closer to a shutdown phase, whose Run takes a context that
// releasing a handle has no use for.
func closePhase(cl io.Closer) func(ctx context.Context) error {
	return func(context.Context) error { return cl.Close() }
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

	ss, err := sqlite.NewSnapshotStore(filepath.Join(
		eventsPath,
		"netbridge_snapshots.db",
	))
	if err != nil {
		shutdown.CloseAll(es)
		return nil, fmt.Errorf("engine container: %w", err)
	}

	nb, err := netbridge.New().WithEventStore(es).WithSnapshotStore(ss).Build(ctx)
	if err != nil {
		shutdown.CloseAll(es, ss)
		return nil, fmt.Errorf("engine container: netbridge: %w", err)
	}

	wiz, err := wizard.New(nil)
	if err != nil {
		shutdown.CloseAll(es, ss)
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

	providers, err := newProviders(metadata.GetPlatforms(), config.GetSearch())
	if err != nil {
		shutdown.CloseAll(es, ss)
		return nil, fmt.Errorf("engine container: providers: %w", err)
	}

	// The vault opens last of everything fallible: it owns a database handle, and
	// building it before a step that can still fail would mean every one of those
	// failure paths owes it a close.
	v, err := vault.New(vaultPath, namespacesPath, 0)
	if err != nil {
		shutdown.CloseAll(es, ss)
		return nil, fmt.Errorf("engine container: vault: %w", err)
	}

	return &Container{
		Vault:     v,
		Manifold:  manifold.New(fetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
		Providers: providers,

		netbridgeEvents:    es,
		netbridgeSnapshots: ss,
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
