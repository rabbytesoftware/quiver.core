package internal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/api"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	App      *app.Container
	API      *api.Container
}

// Start wires engines, app, and API together then blocks until ctx is cancelled.
// host is an optional URI override (e.g. "unix:///custom.sock" or "tcp://0.0.0.0:9000").
// An empty host uses the value from config.
func (c *Container) Start(
	ctx context.Context,
	host string,
) error {
	c.Engines.Start(ctx)
	c.App.Start(ctx)

	cfg := config.GetAPI()
	if host != "" {
		cfg.Host = host
	}

	listener, err := gateway.New(cfg)
	if err != nil {
		return fmt.Errorf("internal: gateway: %w", err)
	}

	runErr := make(chan error, 1)
	go func() { runErr <- c.API.Run(listener) }()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.API.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("internal: shutdown: %w", err)
	}

	if err := <-runErr; err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("internal: api: %w", err)
	}

	return nil
}

// New wires all internal modules together: engine + adapter → app → api.
func New(
	ctx context.Context,
) (*Container, error) {
	engines, err := engine.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New()
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(engines, adapters)
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	v0Container, err := apiv0.New(appContainer)
	if err != nil {
		return nil, fmt.Errorf("internal: api/v0: %w", err)
	}

	apiContainer, err := api.New(appContainer.Hub, v0Container)
	if err != nil {
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	return &Container{
		Engines:  engines,
		Adapters: adapters,
		App:      appContainer,
		API:      apiContainer,
	}, nil
}
