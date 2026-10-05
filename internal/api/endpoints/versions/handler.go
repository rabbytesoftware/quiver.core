package versions

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// Handler serves the GET /versions endpoint.
type Handler struct {
	build     Build
	supported []string
	latest    string
}

// New returns a Handler loaded with build info and the registered API version list.
func New(
	build Build,
	supported []string,
	latest string,
) *Handler {
	return &Handler{
		build:     build,
		supported: supported,
		latest:    latest,
	}
}

// Get returns supported API versions, core build info and the features served.
func (h *Handler) Get(c *gin.Context) {
	libs.WriteQueryOK(c, versionsResponse{
		Build: h.build,
		API: apiInfo{
			Supported: h.supported,
			Latest:    h.latest,
		},
	})
}
