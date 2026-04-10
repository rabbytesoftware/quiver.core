// internal/api/v1/v1.go
package v1

import (
	"fmt"

	wshandler "github.com/rabbytesoftware/quiver/internal/api/v1/ws"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
)

// Container holds V1-scoped dependencies.
type Container struct {
	arrowSvc  arrow.ArrowService
	quiverSvc appquiver.QuiverService
	wsHandler *wshandler.Handler
}

// NewWSHandler creates a standalone WS handler. Call this before Init when
// you need to wire the hub before the app container is ready.
func NewWSHandler() *wshandler.Handler {
	return wshandler.NewHandler()
}

// Init builds the V1 container from the app container and a pre-created WS handler.
// If wsHandler is nil, a new one is created internally.
func Init(
	appContainer *app.Container,
	wsHandler *wshandler.Handler,
) (*Container, error) {
	if appContainer == nil {
		return nil, fmt.Errorf("v1: app container is required")
	}
	if wsHandler == nil {
		wsHandler = wshandler.NewHandler()
	}
	return &Container{
		arrowSvc:  appContainer.Arrow,
		quiverSvc: appContainer.Quiver,
		wsHandler: wsHandler,
	}, nil
}

// WSHandler returns the v1 WebSocket handler (implements api.WSVersion).
func (c *Container) WSHandler() *wshandler.Handler {
	return c.wsHandler
}
