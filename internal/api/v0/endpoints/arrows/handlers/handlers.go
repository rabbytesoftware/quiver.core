package arrows

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Handlers struct {
	svc usecases.ArrowUsecase
}

func New(svc usecases.ArrowUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// Add registers an arrow from an existing manifest in the Quiver registry.
//
// @Summary      Register arrow
// @Description  Registers an arrow by its namespace. The manifest must already exist in the registry.
// @Tags         arrows
// @Param        ns   path  string  true  "Arrow namespace (e.g. github.com/user/repo@v1.0.0)"
// @Success      201  {object}  libs.MutationResponse  "Arrow registered"
// @Failure      400  {object}  libs.ErrResponse       "Invalid namespace"
// @Failure      404  {object}  libs.ErrResponse       "Manifest not found"
// @Failure      409  {object}  libs.ErrResponse       "Arrow already registered"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /arrow/{ns} [post]
func (h *Handlers) Add(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Add(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

// Update pulls the latest manifest for an arrow from the registry and re-registers it.
//
// @Summary      Update arrow manifest
// @Description  Fetches the latest manifest for the arrow and updates its registration.
// @Tags         arrows
// @Accept       json
// @Param        ns   path  string  true  "Arrow namespace"
// @Success      200  {object}  libs.MutationResponse  "Arrow updated"
// @Failure      404  {object}  libs.ErrResponse       "Arrow not found"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /arrow/{ns} [patch]
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

// Remove deregisters an arrow.
//
// @Summary      Remove arrow
// @Description  Deregisters an arrow. The namespace must match the one the arrow was registered under, including its @ref qualifier when present.
// @Tags         arrows
// @Param        ns   path  string  true  "Arrow namespace as registered (e.g. github.com/user/repo@v1.0.0)"
// @Success      200  {object}  libs.MutationResponse  "Arrow removed"
// @Failure      404  {object}  libs.ErrResponse       "Arrow not found"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /arrow/{ns} [delete]
func (h *Handlers) Remove(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Remove(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

// List returns all registered arrows.
//
// @Summary      List arrows
// @Description  Returns all registered arrows. Use the WebSocket upgrade to stream real-time updates instead.
// @Tags         arrows
// @Produce      json
// @Param        user_installed  query  boolean  false  "Filter by user-installed flag"
// @Success      200  {object}  libs.QueryResponse{data=[]apidto.ArrowListItemDTO}
// @Failure      500  {object}  libs.ErrResponse
// @Router       /arrow [get]
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

// GetDetail returns the full detail view of a single arrow including runtime state.
//
// @Summary      Get arrow detail
// @Description  Returns detailed information for an arrow including current state, active run, and last return. Use the WebSocket upgrade to stream live updates.
// @Tags         arrows
// @Produce      json
// @Param        ns   path  string  true  "Arrow namespace"
// @Success      200  {object}  libs.QueryResponse{data=apidto.ArrowDetailDTO}
// @Failure      404  {object}  libs.ErrResponse
// @Failure      500  {object}  libs.ErrResponse
// @Router       /arrow/{ns} [get]
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

// GetManifest returns the raw manifest definition for an arrow.
//
// @Summary      Get arrow manifest
// @Description  Returns the full manifest definition including targets, variables, and lifecycle steps.
// @Tags         arrows
// @Produce      json
// @Param        ns   path  string  true  "Arrow namespace"
// @Success      200  {object}  libs.QueryResponse{data=apidto.ArrowManifestDTO}
// @Failure      404  {object}  libs.ErrResponse
// @Failure      500  {object}  libs.ErrResponse
// @Router       /arrow/{ns}/manifest [get]
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

// Seed uploads a raw YAML manifest for an arrow and registers it immediately.
//
// @Summary      Seed arrow manifest
// @Description  Accepts a raw YAML manifest in the request body, stores it in the registry, and registers the arrow. Use this to publish a new arrow from a local manifest file.
// @Tags         arrows
// @Accept       application/x-yaml
// @Param        ns    path  string  true   "Arrow namespace"
// @Param        body  body  string  true   "Raw YAML manifest"
// @Success      201   {object}  libs.MutationResponse  "Manifest seeded and arrow registered"
// @Failure      400   {object}  libs.ErrResponse       "Failed to read body"
// @Failure      422   {object}  libs.ErrResponse       "Invalid manifest"
// @Failure      500   {object}  libs.ErrResponse       "Internal error"
// @Router       /arrow/{ns}/manifest [post]
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

// Validate checks a raw YAML manifest without registering it.
//
// @Summary      Validate arrow manifest
// @Description  Parses and validates a raw YAML manifest. Returns validation errors and platform compatibility without writing anything to the registry.
// @Tags         arrows
// @Accept       application/x-yaml
// @Produce      json
// @Param        ns    path  string  true  "Arrow namespace"
// @Param        body  body  string  true  "Raw YAML manifest"
// @Success      200   {object}  libs.QueryResponse{data=apidto.ValidationResultDTO}  "Manifest is valid"
// @Failure      400   {object}  libs.ErrResponse                                     "Failed to read body"
// @Failure      422   {object}  libs.QueryResponse{data=apidto.ValidationResultDTO}  "Manifest has validation errors"
// @Failure      500   {object}  libs.ErrResponse                                  "Internal error"
// @Router       /arrow/{ns}/manifest/validate [post]
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
