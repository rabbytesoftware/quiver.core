// internal/api/v0/container.go
package v0

import (
	"fmt"

	api "github.com/rabbytesoftware/quiver/internal/api"
	wshandler "github.com/rabbytesoftware/quiver/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
)

// Container holds V0-scoped dependencies.
type Container struct {
	arrowSvc  arrow.ArrowService
	quiverSvc appquiver.QuiverService
	wsHandler *wshandler.Handler
}

// New builds the V0 container from the app container.
func New(
	appContainer *app.Container,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v0: app container is required")
	}
	return &Container{
		arrowSvc:  appContainer.Arrow,
		quiverSvc: appContainer.Quiver,
		wsHandler: wshandler.NewHandler(),
	}, nil
}

// Prefix returns "/v0".
func (c *Container) Prefix() string { return "/v0" }

// WSHandler returns the v0 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() api.WSVersion {
	return c.wsHandler
}
