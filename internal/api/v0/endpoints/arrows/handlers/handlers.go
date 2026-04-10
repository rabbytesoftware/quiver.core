package arrows

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/libs"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Handlers struct {
	svc arrow.ArrowService
}

func New(svc arrow.ArrowService) *Handlers {
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
	dtos := make([]ArrowListItemDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, ToArrowListItemDTO(item))
	}
	libs.WriteQueryOK(c, dtos)
}

func (h *Handlers) GetDetail(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	detail, err := h.svc.GetDetail(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteQueryOK(c, ToArrowDetailDTO(detail))
}

func (h *Handlers) Execute(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	method := c.Param("method")

	var req ExecuteMethodRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		req = ExecuteMethodRequestDTO{}
	}

	if err := h.svc.BeginExecution(
		c.Request.Context(),
		ns,
		method,
		req.Variables,
	); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusAccepted, string(ns))
}
