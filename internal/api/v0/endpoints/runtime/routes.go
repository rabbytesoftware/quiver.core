package runtime

import (
	"github.com/gin-gonic/gin"

	runtimehandlers "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/runtime/handlers"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.RuntimeUsecase,
	runtimeWS gin.HandlerFunc,
) {
	h := runtimehandlers.New(svc)
	rg.POST("/runtime/:ns/:method", h.Execute)
	rg.GET("/runtime", runtimeWS)
	rg.GET("/runtime/:ns", runtimeWS)
}
