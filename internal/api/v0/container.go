package v0

import (
	"fmt"
	"time"

	api "github.com/rabbytesoftware/quiver.core/internal/api"
	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	wshandler "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
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

	logs    *logring.Ring
	version string
}

// New builds the v0 container. logs feeds the console's log stream (nil serves
// an empty ring) and version is what the console's CLI reports as its own.
func New(
	appContainer *app.Container,
	logs *logring.Ring,
	version string,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}

	var wsOpts []wshandler.Option
	if appContainer.Discovery != nil {
		wsOpts = append(wsOpts, wshandler.WithDiscoveryJobs(appContainer.Discovery))
	}
	wsHandler := wshandler.NewHandler(wsOpts...)

	if logs == nil {
		logs = logring.New()
	}

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
		logs:          logs,
		version:       version,
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
