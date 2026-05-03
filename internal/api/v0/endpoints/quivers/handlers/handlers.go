package quivers

import (
	"io"
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

func (h *Handlers) Follow(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Follow(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

func (h *Handlers) Unfollow(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Unfollow(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

func (h *Handlers) List(c *gin.Context) {
	var followed *bool
	if param := c.Query("followed"); param != "" {
		v := param == "true"
		followed = &v
	}
	items, err := h.svc.List(c.Request.Context(), followed)
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

func (h *Handlers) SeedManifest(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "failed to read body", string(ns))
		return
	}

	if err := h.svc.Seed(c.Request.Context(), ns, body); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}

	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

func (h *Handlers) GetManifest(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	data, err := h.svc.GetManifest(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

func (h *Handlers) ValidateManifest(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "failed to read body", string(ns))
		return
	}

	result, err := h.svc.ValidateManifest(c.Request.Context(), body)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	d := apidto.ValidationResultDTOFrom(result)
	if result.Valid {
		libs.WriteQueryOK(c, d)
	} else {
		libs.WriteQueryWithStatus(c, http.StatusUnprocessableEntity, d)
	}
}
