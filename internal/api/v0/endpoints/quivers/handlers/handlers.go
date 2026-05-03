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

func (h *Handlers) Add(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Add(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

func (h *Handlers) Update(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Update(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

func (h *Handlers) Remove(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Remove(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

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
