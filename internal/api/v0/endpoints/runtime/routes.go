package runtime

import (
	"strings"

	"github.com/gin-gonic/gin"

	runtimehandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.RuntimeUsecase,
	runtimeWS gin.HandlerFunc,
) {
	h := runtimehandlers.New(svc)
	rg.POST("/runtime/:ns/:method", h.Execute)
	rg.GET("/runtime", dispatch(h.List, runtimeWS))
	rg.GET("/runtime/:ns", dispatch(h.Get, runtimeWS))
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
