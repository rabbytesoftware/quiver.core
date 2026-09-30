package search

import (
	"strings"

	"github.com/gin-gonic/gin"

	searchhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.SearchUsecase,
	disc usecases.DiscoveryUsecase,
	discoveryWS gin.HandlerFunc,
) {
	h := searchhandlers.New(svc, disc)
	rg.GET("/search", h.Search)
	rg.POST("/search/discover", h.Discover)
	rg.GET("/search/discover/:job", dispatch(h.Job, cancelOnLeave(disc, discoveryWS)))
}

// dispatch checks the Upgrade header. WS requests go to ws; plain HTTP goes to rest.
func dispatch(rest, ws gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			ws(c)
			return
		}
		rest(c)
	}
}

// cancelOnLeave ties the pass to its audience: the broadcaster's handler blocks
// until the socket closes, so the subscriber is attached for exactly that long.
// The pass is cancelled only once the last subscriber has gone and a short
// grace has passed, which lets a client that dropped and reconnected resume it.
// Nothing is wasted either way — every result verified so far is already in the
// vault, and the job stays readable for its grace period.
func cancelOnLeave(
	disc usecases.DiscoveryUsecase,
	ws gin.HandlerFunc,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if disc == nil {
			ws(c)
			return
		}
		id := c.Param("job")
		disc.Attach(id)
		defer disc.Detach(id)
		ws(c)
	}
}
