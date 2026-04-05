package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

const manifoldFetchTimeout = 30 * time.Second

// Container holds all engine-layer dependencies.
type Container struct {
	Vault     vault.Vault
	Manifold  manifold.Manifold
	Wizard    wizard.Wizard
	Netbridge netbridge.Netbridge
	DepTree   deptree.DepTree
}

// Init constructs all engines and returns a ready-to-use Container.
// Each engine that requires event persistence creates its own SQLite store
// under ~/.quiver/events/.
func Init(ctx context.Context) (*Container, error) {
	es, err := openEventStore("netbridge.db")
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

	return &Container{
		Vault:     vault.New("", 0, domain.CurrentOS().String()),
		Manifold:  manifold.New(manifoldFetchTimeout),
		Wizard:    wiz,
		Netbridge: nb,
		DepTree:   deptree.New(),
	}, nil
}

func openEventStore(filename string) (models.Store, error) {
	dir := filepath.Join(metadata.GetQuiverHome(), "events")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create events dir: %w", err)
	}
	return sqlite.NewEventStore(filepath.Join(dir, filename))
}
