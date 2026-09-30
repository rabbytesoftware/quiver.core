package home

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

type Handlers struct {
	// svc is nil when the daemon was built without discovery: there is nothing
	// to recommend from.
	svc usecases.HomeUsecase
}

func New(
	svc usecases.HomeUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// Home lists the recommended arrows.
//
// @Summary      Read the home shelves
// @Description  Returns the shelves the home screen shows, each a named, ranked list of arrows that are installable without any query. Every arrow has the shape GET /v0/search returns.
// @Description
// @Description  The answer comes from a local snapshot and the vault index. It never reaches a git host, so it is instant and works offline. A shelf that has never been filled has a null refreshed_at and no arrows; the snapshot fills in the background, so a client that sees refreshing set should ask again shortly.
// @Description
// @Description  Shelves are titled for people: show title and nothing that names where an arrow was found. When recommendations are disabled the list of shelves is empty.
// @Tags         home
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=apidto.HomeDTO}
// @Failure      500  {object}  libs.ErrResponse  "Internal error"
// @Failure      503  {object}  libs.ErrResponse  "Recommendations are not configured on this daemon"
// @Router       /home [get]
func (h *Handlers) Home(c *gin.Context) {
	if h.svc == nil {
		libs.WriteErr(c, http.StatusServiceUnavailable, "home is not available", "")
		return
	}

	home, err := h.svc.Home(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "", err)
		return
	}

	libs.WriteQueryOK(c, apidto.HomeDTOFrom(home))
}

// Refresh rebuilds the home shelves.
//
// @Summary      Refresh the home shelves
// @Description  Starts a refresh of the home shelves in the background and returns at once with no body. A refresh already running is joined rather than restarted, so calling this repeatedly is harmless. Read GET /v0/home to see the shelves fill; its refreshing field says when the work is done.
// @Description
// @Description  A refresh that fails or is cancelled leaves the previous shelves in place.
// @Tags         home
// @Success      202  "Refresh started, or already running"
// @Failure      503  {object}  libs.ErrResponse  "Recommendations are not configured on this daemon"
// @Router       /home/refresh [post]
func (h *Handlers) Refresh(c *gin.Context) {
	if h.svc == nil {
		libs.WriteErr(c, http.StatusServiceUnavailable, "home is not available", "")
		return
	}

	h.svc.Refresh(c.Request.Context())
	c.Status(http.StatusAccepted)
}
