package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/system"
)

type HealthHandler struct {
	usecases *usecase.ApiSystemUsescases
}

func NewHealthHandler(
	usecases *usecase.ApiSystemUsescases,
) *HealthHandler {
	return &HealthHandler{
		usecases: usecases,
	}
}

func (h *HealthHandler) SetupRoutes(router *gin.RouterGroup) {
	router.GET("/health", h.Handler())
}

func (h *HealthHandler) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Sector 7C",
		})
	}
}
