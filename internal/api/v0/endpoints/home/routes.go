package home

import (
	"github.com/gin-gonic/gin"

	homehandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/home/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.HomeUsecase,
) {
	h := homehandlers.New(svc)
	rg.GET("/home", h.Home)
	rg.POST("/home/refresh", h.Refresh)
}
