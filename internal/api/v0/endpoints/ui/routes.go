// Package ui mounts the arrow interface proxy.
package ui

import (
	"github.com/gin-gonic/gin"

	uihandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/ui/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.SurfaceUsecase,
) {
	h := uihandlers.New(svc)
	rg.Any("/ui/:ns/*path", h.Serve)
}
