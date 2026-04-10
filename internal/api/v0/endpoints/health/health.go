package health

import (
	"github.com/gin-gonic/gin"
	healthhandlers "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/health/handlers"
)

func Register(rg *gin.RouterGroup) {
	rg.GET("/health", healthhandlers.Check)
}
