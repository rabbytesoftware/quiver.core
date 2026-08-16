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
	"github.com/rabbytesoftware/quiver.core/internal/core/build"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
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

// Shutdown stops accepting requests, drains every aggregate, then releases the
// storage handles.
func (c *Container) Shutdown() error {
	return shutdown.Sequence("internal", c.shutdownPhases())
}

// shutdownPhases lists the sequence in order. Requests stop first, every
// aggregate drains next, and the stores close last, so writes still in flight
// reach an open database.
//
// That ordering is the intent, not a guarantee. A phase that overruns its budget
// is abandoned rather than waited on, so a drain that timed out is still running
// when "adapters close" starts and its remaining writes hit a closed handle.
// The trade is deliberate: a daemon that never exits is worse than a write that
// loses its database, and the aggregate it belonged to replays from the event
// store on the next boot.
//
// Adapters.Close is synchronous and takes no context; it is listed as a phase
// anyway so the whole sequence lives in one place.
func (c *Container) shutdownPhases() []shutdown.Phase {
	return []shutdown.Phase{
		{Name: "api shutdown", Timeout: apiDrainTimeout, Run: c.API.Shutdown},
		{Name: "app shutdown", Timeout: appDrainTimeout, Run: c.App.Shutdown},
		{Name: "engine shutdown", Timeout: engineDrainTimeout, Run: c.Engines.Shutdown},
		{Name: "adapters close", Timeout: adapterCloseTimeout, Run: c.closeAdapters},
	}
}

func (c *Container) closeAdapters(_ context.Context) error {
	return c.Adapters.Close()
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

type internalOpts struct{ homeDir string }

// Option configures internal.New.
type Option func(*internalOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var. The directory is threaded through
// to the engine, adapter, and app containers, which each resolve their own
// paths beneath it. An empty dir is the same as passing no option at all:
// every layer falls back to the process home.
func WithHomeDir(dir string) Option {
	return func(o *internalOpts) { o.homeDir = dir }
}

// New wires all internal modules together: engine + adapter → app → api.
func New(
	ctx context.Context,
	buildInfo build.Info,
	opts ...Option,
) (*Container, error) {
	cfg := internalOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	engines, err := engine.New(ctx, engine.WithHomeDir(cfg.homeDir))
	if err != nil {
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New(adapter.WithHomeDir(cfg.homeDir))
	if err != nil {
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(engines, adapters, app.WithHomeDir(cfg.homeDir))
	if err != nil {
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	v0Container, err := apiv0.New(appContainer)
	if err != nil {
		return nil, fmt.Errorf("internal: api/v0: %w", err)
	}

	apiContainer, err := api.New(appContainer.Hub, buildInfo, v0Container)
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
