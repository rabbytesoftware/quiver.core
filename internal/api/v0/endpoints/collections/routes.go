package quivers

import (
	"strings"

	"github.com/gin-gonic/gin"

	quiverhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/collections/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.CollectionUsecase,
	quiverWS gin.HandlerFunc,
) {
	h := quiverhandlers.New(svc)
	rg.POST("/collection/:ns/follow", h.Follow)
	rg.DELETE("/collection/:ns/follow", h.Unfollow)
	rg.GET("/collection", dispatch(h.List, quiverWS))
	rg.GET("/collection/:ns", dispatch(h.Get, quiverWS))
	rg.GET("/collection/:ns/manifest", h.GetManifest)
	rg.POST("/collection/:ns/manifest", h.SeedManifest)
	rg.POST("/collection/:ns/manifest/validate", h.ValidateManifest)
}

func dispatch(rest, ws gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			ws(c)
			return
		}
		rest(c)
	}
}
