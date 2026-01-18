package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/v1/controllers/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v1/controllers/health"
	"github.com/rabbytesoftware/quiver/internal/api/v1/controllers/quivers"
	"github.com/rabbytesoftware/quiver/internal/api/v1/controllers/system"
	"github.com/rabbytesoftware/quiver/internal/usecases"
)

func SetupRoutes(router *gin.Engine, usecases *usecases.Usecases) {
	healthHandler := health.NewHealthHandler(usecases.System)

	v1 := router.Group("/api/v1")
	{
		arrows.SetupRoutes(
			v1.Group("/arrow"),
			usecases.Arrows,
		)
		quivers.SetupRoutes(
			v1.Group("/quiver"),
			usecases.Quivers,
		)
		system.SetupRoutes(
			v1.Group("/system"),
			usecases.System,
		)
		healthHandler.SetupRoutes(
			v1,
		)
	}
}
