package versions

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// Handler serves the GET /versions endpoint.
type Handler struct {
	build     Build
	features  []string
	supported []string
	latest    string
}

// New returns a Handler loaded with build-time info, the optional features
// the daemon serves, and the registered API version list.
func New(
	build Build,
	features []string,
	supported []string,
	latest string,
) *Handler {
	return &Handler{
		build:     build,
		features:  features,
		supported: supported,
		latest:    latest,
	}
}

// Get returns supported API versions, core build identity (version, build id,
// commit, build time, channel) and the optional features this daemon serves.
func (h *Handler) Get(c *gin.Context) {
	libs.WriteQueryOK(c, versionsResponse{
		Version:  h.build.Version,
		BuildID:  h.build.BuildID,
		Commit:   h.build.Commit,
		BuiltAt:  h.build.BuiltAt,
		Channel:  h.build.Channel,
		Features: h.features,
		API: apiInfo{
			Supported: h.supported,
			Latest:    h.latest,
		},
	})
}
