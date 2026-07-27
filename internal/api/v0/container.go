package v0

import (
	"fmt"

	api "github.com/rabbytesoftware/quiver.core/internal/api"
	wshandler "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

type Container struct {
	arrowSvc      usecases.ArrowUsecase
	runtimeSvc    usecases.RuntimeUsecase
	collectionSvc usecases.CollectionUsecase
	searchSvc     usecases.SearchUsecase
	discoverySvc  usecases.DiscoveryUsecase
	wsHandler     *wshandler.Handler
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

	return &Container{
		arrowSvc:      appContainer.Arrow,
		runtimeSvc:    appContainer.Runtime,
		collectionSvc: appContainer.Collection,
		searchSvc:     appContainer.Search,
		discoverySvc:  appContainer.Discovery,
		wsHandler:     wsHandler,
	}, nil
}

func (c *Container) Prefix() string { return "/v0" }

// WSHandler returns the v0 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() api.WSVersion {
	return c.wsHandler
}
