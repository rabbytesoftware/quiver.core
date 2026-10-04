package v0

import (
	"fmt"
	"time"

	api "github.com/rabbytesoftware/quiver.core/internal/api"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	wshandler "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/console/logring"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

type Container struct {
	arrowSvc      usecases.ArrowUsecase
	runtimeSvc    usecases.RuntimeUsecase
	collectionSvc usecases.CollectionUsecase
	searchSvc     usecases.SearchUsecase
	discoverySvc  usecases.DiscoveryUsecase
	homeSvc       usecases.HomeUsecase
	configSvc     usecases.ConfigUsecase
	authSvc       usecases.AuthUsecase
	pathSvc       usecases.PathUsecase
	systemSvc     usecases.SystemUsecase
	wsHandler     *wshandler.Handler
	rateLimiter   *middleware.RateLimiter

	// AuthGate is exported so internal.Container.Start can flip it once the
	// daemon's listener scheme is resolved — see the type's own doc comment
	// for why a later SetRequired call still reaches every route already
	// built into the router.
	AuthGate *middleware.AuthGate

	// ConsoleAddress is where internal.Container.Start records the address the
	// daemon's own console commands dial, once its listener is bound. The
	// router is built before that address exists, so the executor reads it
	// lazily.
	ConsoleAddress command.Address

	consoleLogs logring.Ring
	consoleExec command.Executor
}

func New(
	appContainer *app.Container,
	opts ...Option,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}

	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

	var wsOpts []wshandler.Option
	if appContainer.Discovery != nil {
		wsOpts = append(wsOpts, wshandler.WithDiscoveryJobs(appContainer.Discovery))
	}
	wsHandler := wshandler.NewHandler(wsOpts...)

	// Discovery results are not domain aggregates and have no projection behind
	// them, so they reach clients straight from the usecase rather than through
	// the domain hub.
	if appContainer.Discovery != nil {
		appContainer.Discovery.OnResult(wsHandler.PushDiscovery)
	}

	authGate := middleware.NewAuthGate(appContainer.Auth)

	rateLimiter, err := newRedeemRateLimiter()
	if err != nil {
		return nil, fmt.Errorf("v0: %w", err)
	}

	address := command.NewAddress()

	return &Container{
		arrowSvc:      appContainer.Arrow,
		runtimeSvc:    appContainer.Runtime,
		collectionSvc: appContainer.Collection,
		searchSvc:     appContainer.Search,
		discoverySvc:  appContainer.Discovery,
		homeSvc:       appContainer.Home,
		configSvc:     appContainer.Config,
		authSvc:       appContainer.Auth,
		pathSvc:       appContainer.Path,
		systemSvc:     appContainer.System,
		wsHandler:     wsHandler,
		rateLimiter:   rateLimiter,
		AuthGate:      authGate,

		ConsoleAddress: address,
		consoleLogs:    logring.Default(),
		consoleExec:    command.New(command.Options{ServerURI: address.Get, Version: cfg.version}),
	}, nil
}

// newRedeemRateLimiter builds the pairing-redeem rate limiter from config,
// once, so every request shares the same counter.
func newRedeemRateLimiter() (*middleware.RateLimiter, error) {
	cfg := config.GetAuth()

	window, err := time.ParseDuration(cfg.RedeemRateWindow)
	if err != nil {
		return nil, fmt.Errorf("parse auth.redeem_rate_window: %w", err)
	}

	return middleware.NewRateLimiter(cfg.RedeemRateLimit, window), nil
}

func (c *Container) Prefix() string { return "/v0" }

// WSHandler returns the v0 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() api.WSVersion {
	return c.wsHandler
}
