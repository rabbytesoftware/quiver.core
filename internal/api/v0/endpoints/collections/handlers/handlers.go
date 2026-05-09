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
	svc usecases.CollectionUsecase
}

func New(svc usecases.CollectionUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// @Summary      Follow quiver
// @Description  Follows a quiver collection and caches its arrows locally.
// @Tags         quivers
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      201  {object}  libs.MutationResponse  "Quiver followed"
// @Failure      404  {object}  libs.ErrResponse       "Quiver not found"
// @Failure      409  {object}  libs.ErrResponse       "Already following"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /collection/{ns}/follow [post]
func (h *Handlers) Follow(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Follow(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, string(ns))
}

// @Summary      Unfollow quiver
// @Description  Stops following a quiver collection.
// @Tags         quivers
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {object}  libs.MutationResponse  "Quiver unfollowed"
// @Failure      404  {object}  libs.ErrResponse       "Quiver not followed"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /collection/{ns}/follow [delete]
func (h *Handlers) Unfollow(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	if err := h.svc.Unfollow(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, string(ns))
}

// @Summary      List quivers
// @Description  Returns quiver collections. Use ?followed=true for followed only, ?followed=false for unfollowed cached, or omit for all.
// @Tags         quivers
// @Produce      json
// @Param        followed  query  boolean  false  "Filter by followed status"
// @Success      200  {object}  libs.QueryResponse{data=[]apidto.CollectionListItemDTO}
// @Failure      500  {object}  libs.ErrResponse
// @Router       /collection [get]
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
	dtos := make([]apidto.CollectionListItemDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, apidto.CollectionListItemDTOFrom(item))
	}
	libs.WriteQueryOK(c, dtos)
}

// @Summary      Get quiver
// @Description  Returns detailed information for a quiver including its arrows and follow status.
// @Tags         quivers
// @Produce      json
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {object}  libs.QueryResponse{data=apidto.CollectionDetailDTO}
// @Failure      404  {object}  libs.ErrResponse
// @Failure      500  {object}  libs.ErrResponse
// @Router       /collection/{ns} [get]
func (h *Handlers) Get(c *gin.Context) {
	ns := domain.Namespace(c.Param("ns"))
	detail, err := h.svc.Get(c.Request.Context(), ns)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, string(ns))
		return
	}
	libs.WriteQueryOK(c, apidto.CollectionDetailDTOFrom(detail))
}

// @Summary      Seed quiver manifest
// @Description  Stores a raw quiver manifest (YAML or QUIVER.md) for the given namespace.
// @Tags         quivers
// @Accept       application/octet-stream
// @Param        ns    path  string  true  "Quiver namespace"
// @Param        body  body  string  true  "Raw manifest bytes"
// @Success      201  {object}  libs.MutationResponse  "Manifest stored"
// @Failure      400  {object}  libs.ErrResponse       "Invalid manifest"
// @Failure      500  {object}  libs.ErrResponse       "Internal error"
// @Router       /collection/{ns}/manifest [post]
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

// @Summary      Get quiver manifest
// @Description  Returns the raw cached manifest for a quiver.
// @Tags         quivers
// @Produce      application/json
// @Param        ns   path  string  true  "Quiver namespace"
// @Success      200  {string}  string  "Raw manifest bytes"
// @Failure      404  {object}  libs.ErrResponse
// @Failure      500  {object}  libs.ErrResponse
// @Router       /collection/{ns}/manifest [get]
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

// @Summary      Validate quiver manifest
// @Description  Validates a raw quiver manifest without storing it.
// @Tags         quivers
// @Accept       application/octet-stream
// @Produce      json
// @Param        ns    path  string  true  "Quiver namespace"
// @Param        body  body  string  true  "Raw manifest bytes"
// @Success      200        {object}  libs.QueryResponse{data=apidto.ValidationResultDTO}  "Valid manifest"
// @Failure      422        {object}  libs.QueryResponse{data=apidto.ValidationResultDTO}  "Invalid manifest"
// @Failure      500        {object}  libs.ErrResponse
// @Router       /collection/{ns}/manifest/validate [post]
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
