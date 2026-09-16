package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/arrows"
	authendpoint "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/auth"
	quivers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/collections"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/health"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/system"
)

// Register mounts every v0 route. Everything except health and the pairing
// endpoints themselves sits behind AuthGate.RequireBearer — a no-op when the
// daemon is bound to unix://, enforced once it is bound to tcp:// (set by
// internal.Container.Start). The pairing group gates itself per-route
// instead: generate/list/revoke are loopback-only, and redeem carries no
// token at all since the whole point is to mint one.
func (c *Container) Register(rg *gin.RouterGroup) {
	protected := rg.Group("")
	protected.Use(c.AuthGate.RequireBearer())

	arrows.Register(protected, c.arrowSvc, c.wsHandler.Arrow.Handle)
	runtime.Register(protected, c.runtimeSvc, c.wsHandler.Runtime.Handle)
	quivers.Register(protected, c.collectionSvc, c.wsHandler.Collection.Handle)
	search.Register(protected, c.searchSvc, c.discoverySvc, c.wsHandler.Discovery.Handle)
	system.Register(protected, c.configSvc)
	health.Register(rg)
	authendpoint.Register(rg, c.authSvc, c.AuthGate, c.rateLimiter)
}
