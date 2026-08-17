package system

import (
	"github.com/gin-gonic/gin"

	systemhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/system/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.ConfigUsecase,
) {
	h := systemhandlers.New(svc)
	rg.GET("/config", h.Config)
	rg.PATCH("/config", h.PatchConfig)
}
