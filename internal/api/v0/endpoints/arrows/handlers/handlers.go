package arrows

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/libs"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/usecases"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Handlers struct {
	svc usecases.ArrowUsecase
}

func New(svc usecases.ArrowUsecase) *Handlers {
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
	opts := models.UpdateOptions{}
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&opts)
	}
	if _, err := h.svc.Update(c.Request.Context(), ns, opts); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

func (h *Handlers) Remove(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if ns.Ref() == "" {
		libs.WriteErr(c, http.StatusBadRequest, "namespace must be versioned (include @ref) for DELETE", string(ns))
		return
	}
	if err := h.svc.Remove(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

func (h *Handlers) List(c *gin.Context) {
	var userInstalled *bool
	if param := c.Query("user_installed"); param != "" {
		v := param == "true"
		userInstalled = &v
	}
	items, err := h.svc.List(c.Request.Context(), userInstalled)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}
	dtos := make([]apidto.ArrowListItemDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, apidto.ArrowListItemDTOFrom(item))
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
	libs.WriteQueryOK(c, apidto.ArrowDetailDTOFrom(detail))
}

func (h *Handlers) GetManifest(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	result, err := h.svc.GetManifest(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteQueryOK(c, apidto.ArrowManifestDTOFrom(result))
}

func (h *Handlers) Seed(c *gin.Context) {
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

func (h *Handlers) Validate(c *gin.Context) {
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

	dto := apidto.ValidationResultDTOFrom(result)
	if result.Valid {
		libs.WriteQueryOK(c, dto)
	} else {
		libs.WriteQueryWithStatus(c, http.StatusUnprocessableEntity, dto)
	}
}
