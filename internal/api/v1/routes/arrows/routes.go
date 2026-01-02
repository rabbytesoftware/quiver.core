package arrows

import (
	"github.com/gin-gonic/gin"
	arrowhandler "github.com/rabbytesoftware/quiver/internal/api/v1/handlers/arrows"
	usecase "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
)

func SetupRoutes(router *gin.RouterGroup, usecases *usecase.ApiArrowUsescases) {
	h := arrowhandler.NewArrowHandler(usecases)

	/*WEBSOCKETS (DO NOT MOVE GROUP)*/

	ws := router.Group("/ws")

	// Listen arrow channels
	ws.GET("/", h.ListenChannel)

	/*HTTP (DO NOT MOVE GROUP)*/

	http := router.Group("/")

	// Add arrow (by namespace or path)
	http.POST("/:namespace", h.AddArrow)
	// Execute method
	http.POST("/:namespace/execute/:method", h.ExecuteMethod)
	// Get single arrow in library
	http.GET("/:namespace", h.GetArrow)
	// List arrows in library
	http.GET("", h.ListArrows)
	// Remove arrow
	http.DELETE("/:namespace", h.RemoveArrow)
}
