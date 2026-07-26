package internal

import (
	"context"
	"errors"
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

// Per-phase shutdown budgets. Each phase gets its own rather than sharing one:
// with a single budget, one slow in-flight HTTP request consumes all of it and
// the aggregate drain then runs on an already-dead context, returning
// immediately without persisting anything.
const (
	apiDrainTimeout     = 2 * time.Second
	appDrainTimeout     = 10 * time.Second
	engineDrainTimeout  = 5 * time.Second
	adapterCloseTimeout = 5 * time.Second
)

type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	App      *app.Container
	API      *api.Container
}

type shutdownPhase struct {
	name    string
	timeout time.Duration
	run     func(ctx context.Context) error
}

// Shutdown stops accepting requests, drains every aggregate, then releases the
// storage handles.
func (c *Container) Shutdown() error {
	return runShutdown(c.shutdownPhases())
}

// shutdownPhases lists the sequence in order. Requests stop first, every
// aggregate drains next, and the stores close last: a drain running after the
// close would send its in-flight writes to a closed database.
//
// Adapters.Close is synchronous and takes no context; it is listed as a phase
// anyway so the whole sequence lives in one place.
func (c *Container) shutdownPhases() []shutdownPhase {
	return []shutdownPhase{
		{name: "api shutdown", timeout: apiDrainTimeout, run: c.API.Shutdown},
		{name: "app shutdown", timeout: appDrainTimeout, run: c.App.Shutdown},
		{name: "engine shutdown", timeout: engineDrainTimeout, run: c.Engines.Shutdown},
		{name: "adapters close", timeout: adapterCloseTimeout, run: c.closeAdapters},
	}
}

func (c *Container) closeAdapters(_ context.Context) error {
	return c.Adapters.Close()
}

// runShutdown runs every phase in order. A failed phase never skips the ones
// after it: an aborted sequence would leave aggregates accepting writes or
// SQLite handles open with their WAL unchecked.
func runShutdown(phases []shutdownPhase) error {
	var errs []error

	for _, p := range phases {
		if err := runPhase(p.timeout, p.run); err != nil {
			errs = append(errs, fmt.Errorf("internal: %s: %w", p.name, err))
		}
	}

	return errors.Join(errs...)
}

// runPhase runs one shutdown phase under its own deadline, derived from
// context.Background() so no phase can inherit a budget an earlier one spent.
func runPhase(
	timeout time.Duration,
	fn func(ctx context.Context) error,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return fn(ctx)
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

	var errs []error

	if err := c.Shutdown(); err != nil {
		errs = append(errs, err)
	}

	if err := <-runErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, fmt.Errorf("internal: api: %w", err))
	}

	return errors.Join(errs...)
}

// New wires all internal modules together: engine + adapter → app → api.
func New(
	ctx context.Context,
	version string,
	buildID string,
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

	apiContainer, err := api.New(appContainer.Hub, api.BuildInfo{Version: version, BuildID: buildID}, v0Container)
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
