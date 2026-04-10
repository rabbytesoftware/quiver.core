package internal

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

// Container is the root dependency container for the quiver server process.
type Container struct {
	API *api.Container
}

// Init wires all internal modules together: engine + adapter → api.
func Init(ctx context.Context) (*Container, error) {
	engines, err := engine.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.Init(metadata.GetQuiverHome())
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	apiContainer, err := api.Init(engines, adapters.ArrowES, adapters.RuntimeES, adapters.QuiverES)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{API: apiContainer}, nil
}
