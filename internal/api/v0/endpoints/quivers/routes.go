package quivers

import (
	"strings"

	"github.com/gin-gonic/gin"

	quiverhandlers "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/quivers/handlers"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.QuiverUsecase,
	quiverWS gin.HandlerFunc,
) {
	h := quiverhandlers.New(svc)
	rg.POST("/quivers/:ns/follow", h.Follow)
	rg.DELETE("/quivers/:ns/follow", h.Unfollow)
	rg.GET("/quiver", dispatch(h.List, quiverWS))
	rg.GET("/quiver/:ns", dispatch(h.Get, quiverWS))
	rg.GET("/quiver/:ns/manifest", h.GetManifest)
	rg.POST("/quiver/:ns/manifest", h.SeedManifest)
	rg.POST("/quiver/:ns/manifest/validate", h.ValidateManifest)
	// backward-compat
	rg.POST("/quiver/:ns", h.Add)
	rg.PATCH("/quiver/:ns", h.Update)
	rg.DELETE("/quiver/:ns", h.Remove)
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
