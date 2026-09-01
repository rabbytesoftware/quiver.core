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
)

type Container struct {
	arrowSvc      usecases.ArrowUsecase
	runtimeSvc    usecases.RuntimeUsecase
	collectionSvc usecases.CollectionUsecase
	searchSvc     usecases.SearchUsecase
	discoverySvc  usecases.DiscoveryUsecase
	configSvc     usecases.ConfigUsecase
	authSvc       usecases.AuthUsecase
	wsHandler     *wshandler.Handler
	rateLimiter   *middleware.RateLimiter

	// AuthGate is exported so internal.Container.Start can flip it once the
	// daemon's listener scheme is resolved — see the type's own doc comment
	// for why a later SetRequired call still reaches every route already
	// built into the router.
	AuthGate *middleware.AuthGate
}

func New(
	appContainer *app.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}
	wsHandler := wshandler.NewHandler()

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
		configSvc:     appContainer.Config,
		authSvc:       appContainer.Auth,
		wsHandler:     wsHandler,
		rateLimiter:   rateLimiter,
		AuthGate:      authGate,
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
