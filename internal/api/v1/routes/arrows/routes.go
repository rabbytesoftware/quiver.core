package arrows

import (
	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/arrows"
)

func SetupRoutes(router *gin.RouterGroup, usecases *usecase.ArrowsUsecase) {
	/*WEBSOCKETS (DO NOT MOVE GROUP)*/
	
	ws := router.Group("/ws")

	// Listen arrow channels
	ws.GET("/")

	/*HTTP (DO NOT MOVE GROUP)*/

	http := router.Group("/")

	// Execute method
	http.GET("/:namespace/execute/:method")
	// Add arrow (by namespace or path)
	http.POST("/:namespace")
	// Get single arrow in library
	http.GET("/:namespace")
	// List arrows in library
	http.GET("")
	// Remove arrow
	http.DELETE("/:namespace")
}
