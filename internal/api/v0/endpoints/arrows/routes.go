package arrows

import (
	"strings"

	"github.com/gin-gonic/gin"
	arrowhandlers "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/arrows/handlers"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
)

func Register(
	rg *gin.RouterGroup,
	svc arrow.ArrowService,
	arrowWS gin.HandlerFunc,
	runtimeWS gin.HandlerFunc,
) {
	h := arrowhandlers.New(svc)
	rg.POST("/arrow/:ns", h.Add)
	rg.PATCH("/arrow/:ns", h.Update)
	rg.DELETE("/arrow/:ns", h.Remove)
	rg.GET("/arrow", dispatch(h.List, arrowWS))
	rg.GET("/arrow/:ns", dispatch(h.GetDetail, arrowWS))
	rg.GET("/arrow/:ns/manifest", h.GetManifest)
	rg.POST("/arrow/:ns/:method", h.Execute)
	rg.Handle("SEED", "/arrow/:ns", h.Seed)
	rg.Handle("SEED", "/arrow/:ns/validate", h.Validate)

	rg.GET("/arrow.runtime", runtimeWS)
	rg.GET("/arrow.runtime/:ns", runtimeWS)
}

// dispatch checks the Upgrade header. WS requests go to wsHandler; plain HTTP goes to rest.
func dispatch(rest, ws gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			ws(c)
			return
		}
		rest(c)
	}
}
