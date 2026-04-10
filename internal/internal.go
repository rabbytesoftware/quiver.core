package internal

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// Container is the root dependency container for the quiver server process.
type Container struct {
	API *api.Container
}

// Init builds all layers: engine → event stores → api (which internally
// creates the app layer, hub, and v0 routes).
func Init(ctx context.Context) (*Container, error) {
	engines, err := engine.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	home := metadata.GetQuiverHome()

	arrowES, err := sqlite.NewEventStore(filepath.Join(home, "arrow-events.db"))
	if err != nil {
		return nil, fmt.Errorf("internal: arrow event store: %w", err)
	}

	runtimeES, err := sqlite.NewEventStore(filepath.Join(home, "runtime-events.db"))
	if err != nil {
		return nil, fmt.Errorf("internal: runtime event store: %w", err)
	}

	quiverES, err := sqlite.NewEventStore(filepath.Join(home, "quiver-events.db"))
	if err != nil {
		return nil, fmt.Errorf("internal: quiver event store: %w", err)
	}

	apiContainer, err := api.Init(engines, arrowES, runtimeES, quiverES)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{API: apiContainer}, nil
}
