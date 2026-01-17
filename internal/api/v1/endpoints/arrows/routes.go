package arrows

import (
	"github.com/gin-gonic/gin"
	arrowhandler "github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/arrows/handlers"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/arrows"
)

func SetupRoutes(router *gin.RouterGroup, usecases *usecase.ArrowUsecases) {
	h := arrowhandler.NewArrowHandler(usecases)

	/*WEBSOCKETS (DO NOT MOVE GROUP)*/

	ws := router.Group("/ws")

	// Listen arrow channels
	ws.GET("/", h.ListenChannel)

	/*HTTP (DO NOT MOVE GROUP)*/

	http := router.Group("/")

	// Execute method (variables in body)
	http.POST("/:namespace/execute/:method", h.ExecuteMethod)
	// Get single arrow in library
	http.GET("/:namespace", h.GetArrow)
	// List arrows in library
	http.GET("", h.ListArrows)
	// Remove arrow
	http.DELETE("/:namespace", h.RemoveArrow)
	// Stop method execution
	http.DELETE("/:namespace/stop/:method", h.StopMethod)
	// Kill method execution
	http.DELETE("/:namespace/kill/:method", h.KillMethod)
	// Add arrow (by namespace, path or directory)
	http.POST("/add/*value", h.AddArrow)
}
