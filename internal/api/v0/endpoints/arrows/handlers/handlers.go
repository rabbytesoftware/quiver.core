package arrows

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/libs"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
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

func (h *Handlers) Execute(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	method := c.Param("method")

	var req apidto.ExecuteMethodRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		req = apidto.ExecuteMethodRequestDTO{}
	}

	var err error
	switch method {
	case "install":
		err = h.svc.Install(c.Request.Context(), ns, req.Variables)
	case "uninstall":
		err = h.svc.Uninstall(c.Request.Context(), ns, req.Variables)
	case "execute":
		err = h.svc.BeginExecution(c.Request.Context(), ns, domain.MethodExecute, req.Variables)
	case "stop":
		err = h.svc.Stop(c.Request.Context(), ns)
	default:
		err = h.svc.BeginExecution(c.Request.Context(), ns, method, req.Variables)
	}

	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusAccepted, string(ns))
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

	result, err := h.svc.ValidateManifest(c.Request.Context(), ns, body)
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
