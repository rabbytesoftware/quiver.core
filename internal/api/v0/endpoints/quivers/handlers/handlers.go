package quivers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver/internal/api/libs"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Handlers struct {
	svc usecases.QuiverUsecase
}

func New(svc usecases.QuiverUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// Add registers a quiver by namespace.
//
// @Summary      Register quiver
// @Description  Registers a quiver collection from the registry by its namespace.
// @Tags         quivers
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      201  {object}  libs.MutationResponse  "Quiver registered"
// @Failure      404  {object}  libs.ErrResponse       "Quiver not found"
// @Failure      409  {object}  libs.ErrResponse       "Quiver already registered"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /quiver/{ns} [post]
func (h *Handlers) Add(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Add(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

// Update refreshes a quiver's manifest from the registry.
//
// @Summary      Update quiver
// @Description  Fetches the latest manifest for a quiver and updates its registration.
// @Tags         quivers
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {object}  libs.MutationResponse  "Quiver updated"
// @Failure      404  {object}  libs.ErrResponse       "Quiver not found"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /quiver/{ns} [patch]
func (h *Handlers) Update(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Update(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

// Remove deregisters a quiver.
//
// @Summary      Remove quiver
// @Description  Deregisters a quiver collection.
// @Tags         quivers
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {object}  libs.MutationResponse  "Quiver removed"
// @Failure      404  {object}  libs.ErrResponse       "Quiver not found"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /quiver/{ns} [delete]
func (h *Handlers) Remove(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Remove(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

// List returns all registered quivers.
//
// @Summary      List quivers
// @Description  Returns all registered quiver collections. Use the WebSocket upgrade to stream real-time updates.
// @Tags         quivers
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=[]apidto.QuiverListItemDTO}
// @Failure      500  {object}  libs.ErrResponse
// @Router       /quiver [get]
func (h *Handlers) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}
	dtos := make([]apidto.QuiverListItemDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, apidto.QuiverListItemDTOFrom(item))
	}
	libs.WriteQueryOK(c, dtos)
}

// Get returns the detail view of a single quiver.
//
// @Summary      Get quiver
// @Description  Returns detailed information for a quiver. Use the WebSocket upgrade to stream live updates.
// @Tags         quivers
// @Produce      json
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {object}  libs.QueryResponse{data=apidto.QuiverDetailDTO}
// @Failure      404  {object}  libs.ErrResponse
// @Failure      500  {object}  libs.ErrResponse
// @Router       /quiver/{ns} [get]
func (h *Handlers) Get(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	detail, err := h.svc.Get(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteQueryOK(c, apidto.QuiverDetailDTOFrom(detail))
}
