// Package handlers serves an arrow's interface through the daemon.
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Handlers struct{ svc usecases.SurfaceUsecase }

func New(svc usecases.SurfaceUsecase) *Handlers { return &Handlers{svc: svc} }

// Serve proxies the request to the arrow's open surface.
//
// @Summary      Serve arrow interface
// @Description  Reverse-proxies any method and path under the namespace to the arrow's open interface, with the /v0/ui/{ns} prefix stripped and the query string forwarded untouched. WebSocket upgrades ride the same route. Authorization and Cookie headers are never forwarded to the arrow.
// @Tags         ui
// @Param        ns    path  string  true  "Arrow namespace"
// @Param        path  path  string  true  "Path inside the arrow interface"
// @Success      200   "Whatever the arrow answers"
// @Failure      404   {object}  libs.ErrResponse  "Arrow not found"
// @Failure      500   {object}  libs.ErrResponse  "Internal error"
// @Failure      502   {string}  string            "The arrow interface did not answer"
// @Failure      503   {object}  libs.ErrResponse  "Arrow has no open interface"
// @Router       /ui/{ns}/{path} [get]
func (h *Handlers) Serve(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	handler, err := h.svc.Handler(c.Request.Context(), ns)
	switch {
	case errors.Is(err, usecases.ErrNoSurface):
		libs.WriteErr(c, http.StatusServiceUnavailable, "arrow has no open surface", string(ns))
		return
	case errors.Is(err, apperrors.ErrNotFound):
		libs.WriteErr(c, http.StatusNotFound, "arrow not found", string(ns))
		return
	case err != nil:
		libs.WriteErr(c, http.StatusInternalServerError, "could not open the arrow surface", string(ns), err)
		return
	}

	// httputil.ReverseProxy aborts a copy with http.ErrAbortHandler; that is
	// routine (the client went away), not a server error.
	defer func() {
		if r := recover(); r != nil && r != http.ErrAbortHandler { //nolint:errorlint // sentinel panic value
			panic(r)
		}
	}()

	c.Request.URL.Path = c.Param("path")
	c.Request.URL.RawPath = ""
	handler.ServeHTTP(c.Writer, c.Request)
}
