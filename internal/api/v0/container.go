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
	wsHandler     *wshandler.Handler
}

func New(
	appContainer *app.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}
	return &Container{
		arrowSvc:      appContainer.Arrow,
		runtimeSvc:    appContainer.Runtime,
		collectionSvc: appContainer.Collection,
		searchSvc:     appContainer.Search,
		wsHandler:     wshandler.NewHandler(),
	}, nil
}

func (c *Container) Prefix() string { return "/v0" }

// WSHandler returns the v0 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() api.WSVersion {
	return c.wsHandler
}
