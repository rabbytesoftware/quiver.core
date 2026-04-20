package internal

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/adapter"
	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	WsHub    *api.Hub
	App      *app.Container
	API      *api.Container
}

// New wires all internal modules together: engine + adapter → app → api.
func New(ctx context.Context) (*Container, error) {
	engines, err := engine.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New()
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	wshub := api.NewHub()

	appContainer, err := app.New(engines, adapters, wshub)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	apiContainer, err := api.New(appContainer, wshub)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{
		Engines:  engines,
		Adapters: adapters,
		WsHub:    wshub,
		App:      appContainer,
		API:      apiContainer,
	}, nil
}
