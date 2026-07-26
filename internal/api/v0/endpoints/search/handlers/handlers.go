package search

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

type Handlers struct {
	svc usecases.SearchUsecase
}

func New(svc usecases.SearchUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// Search matches arrows known to this machine.
//
// @Summary      Search arrows
// @Description  Searches everything this machine already knows about: arrows installed, arrows pulled in as dependencies, arrows from followed collections, and every arrow Quiver has resolved recently. The search is offline and always answers — it never reaches the network.
// @Description
// @Description  Results are ranked by textual relevance, then boosted for an exact name match, membership of a followed collection, and stars. Installed state, provenance and known versions are reported, never scored.
// @Description
// @Description  The os filter is advisory: the compatible-OS list is a denormalised projection of the last compile, and install-time re-resolution is authoritative. Treat it as a hint, not a gate.
// @Tags         search
// @Produce      json
// @Param        q      query  string  true   "Search text. Empty or whitespace-only is rejected."
// @Param        limit  query  int     false  "Maximum results to return. Defaults to 25, capped at 100."
// @Param        os     query  string  false  "Advisory platform filter (e.g. linux/amd64)"  Enums(linux/amd64, linux/arm64, windows/amd64, windows/arm64, darwin/amd64, darwin/arm64)
// @Success      200    {object}  libs.QueryResponse{data=[]apidto.SearchResultDTO}
// @Failure      400    {object}  libs.ErrResponse  "Missing q, or an unknown os value"
// @Failure      500    {object}  libs.ErrResponse  "Internal error"
// @Router       /search [get]
func (h *Handlers) Search(c *gin.Context) {
	text := strings.TrimSpace(c.Query("q"))
	if text == "" {
		libs.WriteErr(c, http.StatusBadRequest, "query parameter q is required", "")
		return
	}

	os := domain.OS(c.Query("os"))
	if os != "" && !os.IsValid() {
		libs.WriteErr(c, http.StatusBadRequest, "unknown os filter", "")
		return
	}

	results, err := h.svc.Search(c.Request.Context(), models.SearchQuery{
		Text:  text,
		OS:    os,
		Limit: parseLimit(c.Query("limit")),
	})
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	dtos := make([]apidto.SearchResultDTO, 0, len(results))
	for _, result := range results {
		dtos = append(dtos, apidto.SearchResultDTOFrom(result))
	}
	libs.WriteQueryOK(c, dtos)
}

// parseLimit falls back to the default for anything it cannot read as a
// positive number, and caps the rest so one query cannot ask for the whole
// catalog.
func parseLimit(
	raw string,
) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	return min(n, maxLimit)
}
