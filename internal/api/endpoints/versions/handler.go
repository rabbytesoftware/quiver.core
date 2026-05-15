package versions

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// Handler serves the GET /versions endpoint.
type Handler struct {
	version   string
	buildID   string
	supported []string
	latest    string
}

// New returns a Handler loaded with build-time info and the registered API version list.
func New(
	version string,
	buildID string,
	supported []string,
	latest string,
) *Handler {
	return &Handler{
		version:   version,
		buildID:   buildID,
		supported: supported,
		latest:    latest,
	}
}

// Get returns supported API versions, core build info, and minimum required client version.
func (h *Handler) Get(c *gin.Context) {
	libs.WriteQueryOK(c, versionsResponse{
		Version: h.version,
		BuildID: h.buildID,
		API: apiInfo{
			Supported:        h.supported,
			Latest:           h.latest,
			MinClientVersion: minClientVersion,
		},
	})
}
