package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/arrows"
	quivers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/collections"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/health"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search"
)

func (c *Container) Register(rg *gin.RouterGroup) {
	arrows.Register(rg, c.arrowSvc, c.wsHandler.Arrow.Handle)
	runtime.Register(rg, c.runtimeSvc, c.wsHandler.Runtime.Handle)
	quivers.Register(rg, c.collectionSvc, c.wsHandler.Collection.Handle)
	search.Register(rg, c.searchSvc)
	health.Register(rg)
}
