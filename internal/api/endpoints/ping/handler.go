package ping

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/core/build"
)

// Handler serves the stable readiness contract used by the update bootstrap.
type Handler struct {
	info build.Info
}

// New returns a readiness handler for info.
func New(info build.Info) *Handler {
	return &Handler{info: info}
}

// Get reports the identity of the daemon that owns the active listener.
//
// @Summary      Read daemon readiness identity
// @Description  Returns the build and update-attempt identity of the daemon serving the active listener.
// @Tags         system
// @Produce      json
// @Success      200  {object}  response
// @Router       /ping [get]
func (h *Handler) Get(c *gin.Context) {
	libs.WriteQueryOK(c, response{
		Status:       "ready",
		Version:      h.info.Version,
		BuildID:      h.info.BuildID,
		Digest:       h.info.Digest,
		AttemptToken: h.info.AttemptToken,
	})
}
