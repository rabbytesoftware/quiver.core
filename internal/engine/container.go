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
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// Container holds all engine-layer dependencies.
type Container struct {
	Vault              vault.Vault
	Manifold           manifold.Manifold
	Wizard             wizard.Wizard
	Netbridge          netbridge.Netbridge
	DepTree            deptree.DepTree
	netbridgeEvents    io.Closer
	netbridgeSnapshots io.Closer
}

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

// Shutdown drains netbridge's aggregate and closes its event and snapshot
// handles.
//
// It must run after the app layer has drained: the runtime assembler allocates
// ports through netbridge, so an in-flight install still needs it. Every phase
// runs even when an earlier one fails, so a drain error cannot leak handles.
func (c *Container) Shutdown(ctx context.Context) error {
	var errs []error

	if err := c.Netbridge.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("engine container: netbridge shutdown: %w", err))
	}

	if err := c.netbridgeEvents.Close(); err != nil {
		errs = append(errs, fmt.Errorf("engine container: netbridge events close: %w", err))
	}

	if err := c.netbridgeSnapshots.Close(); err != nil {
		errs = append(errs, fmt.Errorf("engine container: netbridge snapshots close: %w", err))
	}

	return errors.Join(errs...)
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

	return &Container{
		Vault:              vault.New(vaultPath, namespacesPath, 0),
		Manifold:           manifold.New(fetchTimeout),
		Wizard:             wiz,
		Netbridge:          nb,
		DepTree:            deptree.New(),
		netbridgeEvents:    es,
		netbridgeSnapshots: ss,
	}, nil
}
