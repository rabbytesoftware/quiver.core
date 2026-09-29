package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/api"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/core"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
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
	loggerCloseTimeout  = 2 * time.Second
)

type Container struct {
	Engines  *engine.Container
	Adapters *adapter.Container
	App      *app.Container
	API      *api.Container

	// authGate is the same instance the v0 router already built its
	// device-pairing middleware around. Start flips it once the listener
	// scheme is known — see middleware.AuthGate's doc comment for why that
	// reaches routes built before the flip.
	authGate *middleware.AuthGate

	// listener is set only via WithGateway -- PrepareGateway's caller already
	// paid for the bind before New ever touched the adapter layer, so Start
	// must reuse it rather than asking gateway.New for a second one (which,
	// dialing this same process's own listener, would misreport it as
	// ErrDaemonRunning). Nil for every caller that has not adopted
	// PrepareGateway, which is every existing test: Start falls back to
	// resolving and binding from its own host argument for those exactly as
	// it always has.
	listener net.Listener

	// loggerShutdown closes core.New/NewAt's log file handle. Closed last,
	// after every other phase, so logging from their own shutdown still
	// reaches the file.
	loggerShutdown func() error
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
		{Name: "logger close", Timeout: loggerCloseTimeout, Run: c.closeLogger},
	}
}

func (c *Container) closeAdapters(_ context.Context) error {
	return c.Adapters.Close()
}

// closeLogger releases core.New/NewAt's log file handle. Without this, a
// process that constructs more than one Container in its lifetime — every
// test that builds one — leaks a held-open file each time; Windows refuses
// to let a later RemoveAll (e.g. t.TempDir's cleanup) delete a directory
// containing one.
func (c *Container) closeLogger(_ context.Context) error {
	if c.loggerShutdown == nil {
		return nil
	}
	return c.loggerShutdown()
}

// PrepareGateway resolves host (falling back to config exactly as Start does)
// and binds the gateway listener, before internal.New is ever called.
//
// cmd/quiver calls this ahead of New on purpose: New's construction reaches
// adapter.New, which runs the sqlite event stores' migration unconditionally,
// long before Start would otherwise have taken this same bind-first-and-fail
// path. Two daemons racing to start on the same machine (a first boot's wider
// window -- see app.Container.Start's own doc comment on the one-time binary
// promotion -- makes this far likelier than it sounds) both used to reach that
// migration regardless of which of them could ever have bound the socket, and
// the loser crashed racing the winner to CREATE TABLE the same schema instead
// of retiring before it touched anything. Binding here, before New, means the
// loser's gateway.New returns ErrDaemonRunning before New -- and so
// adapter.New -- is ever called at all.
func PrepareGateway(host string) (net.Listener, string, error) {
	cfg := config.GetAPI()
	if host != "" {
		cfg.Host = host
	}

	scheme, _, err := gateway.Scheme(cfg.Host)
	if err != nil {
		return nil, "", fmt.Errorf("internal: gateway: %w", err)
	}

	listener, err := gateway.New(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("internal: gateway: %w", err)
	}

	return listener, scheme, nil
}

// Start wires engines, app, and API together and blocks until ctx is
// cancelled. host is an optional URI override (e.g. "unix:///custom.sock" or
// "tcp://0.0.0.0:9000"); an empty host uses the value from config.
//
// A Container built with WithGateway already holds the listener PrepareGateway
// bound for it -- see that function's doc for why binding has to happen before
// New, not here. Start reuses it as-is. Every other Container (every existing
// caller, since WithGateway is new) falls back to resolving and binding its
// own listener exactly as this always has, from host/config rather than a
// pre-bound listener.
func (c *Container) Start(
	ctx context.Context,
	host string,
) error {
	listener := c.listener
	if listener == nil {
		bound, err := c.bindGateway(host)
		if err != nil {
			return err
		}
		listener = bound
	}

	c.Engines.Start(ctx)
	c.App.Start(ctx)

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

// bindGateway is Start's fallback for a Container that was never given a
// pre-bound listener via WithGateway -- every caller before PrepareGateway
// existed, and every test. It resolves and binds exactly as Start always did
// before WithGateway was added.
func (c *Container) bindGateway(host string) (net.Listener, error) {
	cfg := config.GetAPI()
	if host != "" {
		cfg.Host = host
	}

	scheme, _, err := gateway.Scheme(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}
	c.authGate.SetRequired(scheme == "tcp")

	listener, err := gateway.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("internal: gateway: %w", err)
	}

	return listener, nil
}

type internalOpts struct {
	homeDir           string
	commit            string
	selfUpdateTrigger *selfupdate.Trigger
	listener          net.Listener
	scheme            string
}

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

// WithCommit sets the full hash of the commit the running build came from,
// which quiver.core records against its own catalog row so a rolling release
// tag can be told apart from the build already installed.
func WithCommit(commit string) Option {
	return func(o *internalOpts) { o.commit = commit }
}

// WithSelfUpdateTrigger hands the container the trigger quiver.core's own
// update lifecycle fires when it succeeds. Firing it cancels the context Start
// is blocked on, so the daemon leaves through the same graceful sequence a
// SIGTERM would take it through; cmd/quiver then relaunches instead of exiting.
// Without the option the daemon never succeeds itself.
func WithSelfUpdateTrigger(trig *selfupdate.Trigger) Option {
	return func(o *internalOpts) { o.selfUpdateTrigger = trig }
}

// WithGateway hands New a listener PrepareGateway already bound, and the
// scheme it was resolved from. Start then uses it as-is instead of binding
// its own -- see PrepareGateway's own doc for why cmd/quiver must call it
// before New, not the other way around. Tests that call New without this
// option are unaffected: Start falls back to resolving and binding from its
// own host argument exactly as it always has.
func WithGateway(listener net.Listener, scheme string) Option {
	return func(o *internalOpts) {
		o.listener = listener
		o.scheme = scheme
	}
}

// New wires all internal modules together: engine + adapter → app → api.
func New(
	ctx context.Context,
	version string,
	buildID string,
	opts ...Option,
) (*Container, error) {
	cfg := internalOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	// core.New configures the process-lifetime logger and metadata/config
	// singletons before anything downstream can log or read a config value.
	var loggerShutdown func() error
	if cfg.homeDir != "" {
		_, loggerShutdown = core.NewAt(cfg.homeDir)
	} else {
		_, loggerShutdown = core.New()
	}

	engines, err := engine.New(ctx, engine.WithHomeDir(cfg.homeDir))
	if err != nil {
		_ = loggerShutdown()
		return nil, fmt.Errorf("internal: engine: %w", err)
	}

	adapters, err := adapter.New(adapter.WithHomeDir(cfg.homeDir))
	if err != nil {
		_ = loggerShutdown()
		return nil, fmt.Errorf("internal: adapter: %w", err)
	}

	appContainer, err := app.New(
		engines,
		adapters,
		app.WithHomeDir(cfg.homeDir),
		app.WithVersion(version),
		app.WithCommit(cfg.commit),
		app.WithSelfUpdateTrigger(cfg.selfUpdateTrigger),
	)
	if err != nil {
		_ = loggerShutdown()
		return nil, fmt.Errorf("internal: app: %w", err)
	}

	v0Container, err := apiv0.New(appContainer)
	if err != nil {
		_ = loggerShutdown()
		return nil, fmt.Errorf("internal: api/v0: %w", err)
	}

	apiContainer, err := api.New(appContainer.Hub, api.BuildInfo{Version: version, BuildID: buildID}, v0Container)
	if err != nil {
		_ = loggerShutdown()
		return nil, fmt.Errorf("internal: api: %w", err)
	}

	// Start only flips authGate itself when it resolves its own listener; a
	// listener supplied here via WithGateway means Start skips that
	// resolution entirely, so the flip has to happen here instead, using the
	// scheme PrepareGateway already resolved it from.
	if cfg.listener != nil {
		v0Container.AuthGate.SetRequired(cfg.scheme == "tcp")
	}

	return &Container{
		Engines:        engines,
		Adapters:       adapters,
		App:            appContainer,
		API:            apiContainer,
		authGate:       v0Container.AuthGate,
		listener:       cfg.listener,
		loggerShutdown: loggerShutdown,
	}, nil
}
