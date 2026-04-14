package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
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

// Init constructs all engines and returns a ready-to-use Container.
func Init(ctx context.Context) (*Container, error) {
	eventsPath, err := paths.Events()
	if err != nil {
		return nil, fmt.Errorf("engine container: %w", err)
	}

	es, err := sqlite.NewEventStore(filepath.Join(eventsPath, "netbridge.db"))
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
		Vault:     vault.New("", 0, domain.CurrentOS()),
		Manifold:  manifold.New(fetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}
