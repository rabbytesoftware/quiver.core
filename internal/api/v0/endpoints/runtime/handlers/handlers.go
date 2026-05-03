package handlers

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
	svc usecases.RuntimeUsecase
}

func New(svc usecases.RuntimeUsecase) *Handlers {
	return &Handlers{svc: svc}
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
		err = h.svc.Execute(c.Request.Context(), ns, domain.MethodExecute, req.Variables)
	case "stop":
		err = h.svc.Stop(c.Request.Context(), ns)
	case "_update", "update":
		err = h.svc.Execute(c.Request.Context(), ns, domain.MethodUpdate, req.Variables)
	default:
		err = h.svc.Execute(c.Request.Context(), ns, method, req.Variables)
	}

	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusAccepted, string(ns))
}
