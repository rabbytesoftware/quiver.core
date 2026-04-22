package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/health"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/quivers"
)

// Register mounts all v0 routes onto the given router group.
func (c *Container) Register(rg *gin.RouterGroup) {
	arrows.Register(rg, c.arrowSvc, c.wsHandler.Arrow.Handle, c.wsHandler.Runtime.Handle)
	quivers.Register(rg, c.quiverSvc, c.wsHandler.Quiver.Handle)
	health.Register(rg)
}
