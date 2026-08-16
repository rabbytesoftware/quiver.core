package system

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

// Handlers serves the daemon configuration endpoints.
type Handlers struct {
	svc usecases.ConfigUsecase
}

// New returns Handlers backed by the given configuration usecase.
func New(
	svc usecases.ConfigUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// Config returns the daemon configuration.
//
// @Summary      Read the daemon configuration
// @Description  Returns the configuration three ways at once: running is what this process is using, configured is what the next start will use, and defaults is what ships in the binary.
// @Description
// @Description  Every configuration change takes effect on restart, so running and configured differ whenever a change is still pending. restart_required names exactly those fields, so a client does not have to diff the documents itself.
// @Description
// @Description  running carries no api section: the --host flag can override the configured host at start, so the daemon cannot report a bind address from configuration alone.
// @Tags         system
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=apidto.ConfigDTO}
// @Failure      500  {object}  libs.ErrResponse  "The configuration file could not be read"
// @Router       /config [get]
func (h *Handlers) Config(c *gin.Context) {
	view, err := h.svc.Get(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryOK(c, apidto.ConfigDTOFrom(view))
}

// PatchConfig changes the daemon configuration.
//
// @Summary      Change the daemon configuration
// @Description  Applies a sparse configuration change. A field left out of the body is untouched, a field set to null is restored to its default, and a field carrying a value is set to it.
// @Description
// @Description  Settings are accepted or refused one at a time: a body mixing valid and invalid settings persists the valid ones and reports the rest in rejected, including any key the daemon does not recognise. The response is 422 only when nothing at all could be applied.
// @Description
// @Description  Nothing takes effect until the daemon restarts. Re-read GET /v0/config to see the change reflected in configured and listed in restart_required.
// @Tags         system
// @Accept       json
// @Produce      json
// @Param        body  body      usecases.Config  true  "Sparse configuration document: any subset of the sections and settings returned by GET /v0/config, with null to restore a default"
// @Success      200   {object}  libs.QueryResponse{data=apidto.ConfigPatchResultDTO}
// @Failure      400   {object}  libs.ErrResponse  "Body could not be read"
// @Failure      422   {object}  libs.ErrResponse  "Every field in the body was rejected"
// @Router       /config [patch]
func (h *Handlers) PatchConfig(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "body could not be read", "")
		return
	}

	result, err := h.svc.Patch(c.Request.Context(), body)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryOK(c, apidto.ConfigPatchResultDTOFrom(result))
}
