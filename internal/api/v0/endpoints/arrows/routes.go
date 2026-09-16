package arrows

import (
	"strings"

	"github.com/gin-gonic/gin"

	arrowhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/arrows/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.ArrowUsecase,
	arrowWS gin.HandlerFunc,
) {
	h := arrowhandlers.New(svc)
	rg.POST("/arrow/:ns", h.Add)
	rg.PATCH("/arrow/:ns", h.Update)
	rg.DELETE("/arrow/:ns", h.Remove)
	rg.GET("/arrow", dispatch(h.List, arrowWS))
	rg.GET("/arrow/:ns", dispatch(h.GetDetail, arrowWS))
	rg.GET("/arrow/:ns/manifest", h.GetManifest)
	rg.GET("/arrow/:ns/readme", h.GetReadme)
	rg.GET("/arrow/:ns/dependents", h.GetDependents)
	rg.GET("/arrow/:ns/dependencies", h.GetDependencies)
	rg.POST("/arrow/:ns/manifest", h.Seed)
	rg.POST("/arrow/:ns/manifest/validate", h.Validate)
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
