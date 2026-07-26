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

// cancelOnLeave stops the pass once the subscriber is gone: the broadcaster's
// handler blocks until the socket closes, and there is no point fetching
// manifests nobody will see. Nothing is wasted — every result verified so far is
// already in the vault, and the job stays readable for its grace period.
func cancelOnLeave(
	disc usecases.DiscoveryUsecase,
	ws gin.HandlerFunc,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ws(c)
		if disc != nil {
			disc.Cancel(c.Request.Context(), c.Param("job"))
		}
	}
}
