package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/health"
	"github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/quivers"
	"github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/system"
	"github.com/rabbytesoftware/quiver/internal/usecases"
)

func SetupRoutes(router *gin.Engine, usecases *usecases.Usescases) {
	healthHandler := health.NewHealthHandler(usecases.System)

	// Setup endpoint groups
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
