package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
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
// @Success      200     {object}  libs.MutationResponse             "No-op: arrow already in the requested state"
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

	var (
		err error
		// started reports whether an execution was actually begun. Only
		// install can short-circuit to an idempotent no-op today; every other
		// method that reaches here has begun work.
		started = true
	)
	switch method {
	case "install":
		started, err = h.svc.Install(c.Request.Context(), ns, req.Variables)
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
		libs.WriteErr(c, status, msg, string(ns), err)
		return
	}

	// 202 when async work started; 200 when the call was an idempotent no-op.
	status := http.StatusAccepted
	if !started {
		status = http.StatusOK
	}
	libs.WriteMutationOK(c, status, string(ns))
}

// Get returns the runtime snapshot for a single arrow.
//
// @Summary      Get runtime
// @Description  Returns the current runtime state for an arrow, including the active execution and the last completed return. Use the WebSocket upgrade on the same route to stream updates instead.
// @Tags         runtime
// @Produce      json
// @Param        ns   path  string  true  "Arrow namespace"
// @Success      200  {object}  libs.QueryResponse{data=apidto.ArrowRuntimeDTO}
// @Failure      404  {object}  libs.ErrResponse  "Arrow not found"
// @Failure      500  {object}  libs.ErrResponse  "Internal error"
// @Router       /runtime/{ns} [get]
func (h *Handlers) Get(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))

	rt, err := h.svc.GetRuntime(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	if rt == nil {
		status, msg := apierr.StatusAndMessage(apperrors.ErrNotFound)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteQueryOK(c, apidto.ArrowRuntimeDTOFrom(*rt))
}

// List returns runtime snapshots for every arrow in the catalog.
//
// @Summary      List runtimes
// @Description  Returns the current runtime state of every arrow in the catalog. Arrows that have never been installed report state "absent". Use the WebSocket upgrade on the same route to stream updates instead.
// @Tags         runtime
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=[]apidto.ArrowRuntimeDTO}
// @Failure      500  {object}  libs.ErrResponse  "Internal error"
// @Router       /runtime [get]
func (h *Handlers) List(c *gin.Context) {
	runtimes, err := h.svc.ListRuntimes(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	dtos := make([]apidto.ArrowRuntimeDTO, 0, len(runtimes))
	for _, rt := range runtimes {
		dtos = append(dtos, apidto.ArrowRuntimeDTOFrom(rt))
	}
	libs.WriteQueryOK(c, dtos)
}
