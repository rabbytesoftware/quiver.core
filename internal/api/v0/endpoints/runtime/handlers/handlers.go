package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Handlers struct {
	svc usecases.RuntimeUsecase
}

func New(svc usecases.RuntimeUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// Execute triggers a lifecycle method on an arrow.
//
// @Summary      Execute method
// @Description  Triggers a lifecycle method on an arrow (install, uninstall, execute, stop, update, or any custom method defined in the manifest). Returns 202 Accepted immediately; progress is streamed via WebSocket.
// @Tags         runtime
// @Accept       json
// @Param        ns      path  string                          true   "Arrow namespace"
// @Param        method  path  string                          true   "Method name (install | uninstall | execute | stop | update | <custom>)"
// @Param        body    body  apidto.ExecuteMethodRequestDTO  false  "Optional variables"
// @Success      202     {object}  libs.MutationResponse             "Method accepted"
// @Failure      400     {object}  libs.ErrResponse                  "Invalid request, or a reserved variable was set"
// @Failure      404     {object}  libs.ErrResponse                  "Arrow not found"
// @Failure      409     {object}  libs.ErrResponse                  "Arrow already running"
// @Failure      500     {object}  libs.ErrResponse                  "Internal error"
// @Router       /runtime/{ns}/{method} [post]
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
