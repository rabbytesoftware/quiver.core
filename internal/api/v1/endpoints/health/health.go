package health

import (
	"github.com/gin-gonic/gin"
	healthhandlers "github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/health/handlers"
)

// Register mounts GET /health onto the given router group.
func Register(rg *gin.RouterGroup) {
	rg.GET("/health", healthhandlers.Check)
}
