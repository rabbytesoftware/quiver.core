package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/health"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/quivers"
	"github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/runtime"
)

func (c *Container) Register(rg *gin.RouterGroup) {
	arrows.Register(rg, c.arrowSvc, c.wsHandler.Arrow.Handle)
	runtime.Register(rg, c.runtimeSvc, c.wsHandler.Runtime.Handle)
	quivers.Register(rg, c.quiverSvc, c.wsHandler.Quiver.Handle)
	health.Register(rg)
}
