package quivers

import (
	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/quivers"
)

func SetupRoutes(
	router *gin.RouterGroup,
	usecases *usecase.ApiQuiverUsescases,
) {
}
