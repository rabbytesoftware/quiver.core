package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/system"
)

type HealthHandler struct {
	usecases *usecase.SystemUsescases
}

func NewHealthHandler(
	usecases *usecase.SystemUsescases,
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
