package v0

import (
	"fmt"

	api "github.com/rabbytesoftware/quiver/internal/api"
	wshandler "github.com/rabbytesoftware/quiver/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
)

type Container struct {
	arrowSvc   usecases.ArrowUsecase
	runtimeSvc usecases.RuntimeUsecase
	quiverSvc  usecases.QuiverUsecase
	wsHandler  *wshandler.Handler
}

func New(
	appContainer *app.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}
	return &Container{
		arrowSvc:   appContainer.Arrow,
		runtimeSvc: appContainer.Runtime,
		quiverSvc:  appContainer.Quiver,
		wsHandler:  wshandler.NewHandler(),
	}, nil
}

func (c *Container) Prefix() string { return "/v0" }

// WSHandler returns the v0 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() api.WSVersion {
	return c.wsHandler
}
