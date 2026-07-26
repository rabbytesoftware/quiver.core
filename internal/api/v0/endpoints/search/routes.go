package search

import (
	"github.com/gin-gonic/gin"

	searchhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func Register(
	rg *gin.RouterGroup,
	svc usecases.SearchUsecase,
) {
	h := searchhandlers.New(svc)
	rg.GET("/search", h.Search)
}
