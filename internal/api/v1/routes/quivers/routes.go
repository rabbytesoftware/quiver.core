package quivers

import (
	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/quivers"
)

func SetupRoutes(
	router *gin.RouterGroup,
	usecases *usecase.QuiversUsecase,
) {
}
