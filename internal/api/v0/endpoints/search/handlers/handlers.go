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
	// disc is nil when the daemon was built without a vault or a manifold:
	// there is nothing to parse a discovered manifest with or write it to.
	disc usecases.DiscoveryUsecase
}

func New(
	svc usecases.SearchUsecase,
	disc usecases.DiscoveryUsecase,
) *Handlers {
	return &Handlers{svc: svc, disc: disc}
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

// Discover starts a network search across every configured git host.
//
// @Summary      Start a discovery pass
// @Description  Searches the configured git hosts for arrows nobody on this machine has seen yet, and returns immediately with a job id. It does not wait for providers.
// @Description
// @Description  Every candidate is proven before it is reported: its manifest is fetched, parsed and compiled, then written to the vault. That is what makes the results renderable without a second round trip, and what makes a later POST /v0/arrow/{ns} on a discovered namespace serve from cache.
// @Description
// @Description  Verified results stream from GET /v0/search/discover/{job} with an Upgrade header. Counts and per-provider failures never appear on that stream; read them once from the same path without the header, after the socket closes.
// @Tags         search
// @Accept       json
// @Produce      json
// @Param        body  body      apidto.DiscoverRequestDTO  true  "Query. Blank or whitespace-only is rejected."
// @Success      202   {object}  libs.QueryResponse{data=apidto.DiscoveryJobStartedDTO}
// @Failure      400   {object}  libs.ErrResponse  "Missing or blank q, or a body that is not JSON"
// @Failure      503   {object}  libs.ErrResponse  "Discovery is not configured on this daemon"
// @Router       /search/discover [post]
func (h *Handlers) Discover(c *gin.Context) {
	if h.disc == nil {
		libs.WriteErr(c, http.StatusServiceUnavailable, "discovery is not available", "")
		return
	}

	var req apidto.DiscoverRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "body must be a json object with a q field", "")
		return
	}

	text := strings.TrimSpace(req.Q)
	if text == "" {
		libs.WriteErr(c, http.StatusBadRequest, "field q is required", "")
		return
	}

	job, err := h.disc.Start(c.Request.Context(), text)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryWithStatus(c, http.StatusAccepted, apidto.DiscoveryJobStartedDTOFrom(job))
}

// Job reports how a discovery pass went.
//
// @Summary      Read a discovery job
// @Description  Returns the status of a discovery pass and, once it has finished, how many candidates were found, verified and skipped, plus what every host contributed.
// @Description
// @Description  A host that refused is reported with the reason and, for a rate limit, how many seconds to wait. That is the only place a client can tell "GitHub rate-limited you" apart from "nothing matched", because the result stream carries results and nothing else.
// @Description
// @Description  Read this once, when the socket closes. Do not poll it. A finished job stays readable for a short grace period and is then evicted, after which this returns 404 and the search should simply be run again — the verified manifests are already in the vault.
// @Description
// @Description  Sending an Upgrade: websocket header to this same path opens the result stream instead.
// @Tags         search
// @Produce      json
// @Param        job  path  string  true  "Job id returned by POST /search/discover"
// @Success      200  {object}  libs.QueryResponse{data=apidto.DiscoveryJobDTO}
// @Failure      404  {object}  libs.ErrResponse  "Unknown or already evicted job"
// @Failure      503  {object}  libs.ErrResponse  "Discovery is not configured on this daemon"
// @Router       /search/discover/{job} [get]
func (h *Handlers) Job(c *gin.Context) {
	if h.disc == nil {
		libs.WriteErr(c, http.StatusServiceUnavailable, "discovery is not available", "")
		return
	}

	job, err := h.disc.Get(c.Request.Context(), c.Param("job"))
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryOK(c, apidto.DiscoveryJobDTOFrom(*job))
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
