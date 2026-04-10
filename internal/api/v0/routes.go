package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/health"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/quivers"
)

// Register mounts all v1 routes onto the given router group.
func (c *Container) Register(rg *gin.RouterGroup) {
	arrows.Register(rg, c.arrowSvc, c.wsHandler)
	quivers.Register(rg, c.quiverSvc, c.wsHandler)
	health.Register(rg)
}
