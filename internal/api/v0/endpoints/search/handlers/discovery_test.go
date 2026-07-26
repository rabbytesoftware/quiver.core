package search_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	search "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func discoveryRouter(disc *mocks.DiscoveryService) *gin.Engine {
	h := search.New(&mocks.SearchService{}, discoveryOrNil(disc))
	r := gin.New()
	r.POST("/v0/search/discover", h.Discover)
	r.GET("/v0/search/discover/:job", h.Job)
	return r
}

// discoveryOrNil keeps a nil *DiscoveryService from becoming a non-nil
// interface, which is the difference between a 503 and a panic.
func discoveryOrNil(
	disc *mocks.DiscoveryService,
) usecases.DiscoveryUsecase {
	if disc == nil {
		return nil
	}
	return disc
}

func post(
	disc *mocks.DiscoveryService,
	body string,
) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/search/discover", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	discoveryRouter(disc).ServeHTTP(w, req)
	return w
}

func getJob(
	disc *mocks.DiscoveryService,
	id string,
) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	discoveryRouter(disc).ServeHTTP(
		w,
		httptest.NewRequest(http.MethodGet, "/v0/search/discover/"+id, nil),
	)
	return w
}

func decodeStarted(
	t *testing.T,
	body []byte,
) apidto.DiscoveryJobStartedDTO {
	t.Helper()

	var env struct {
		Success bool                          `json:"success"`
		Data    apidto.DiscoveryJobStartedDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.True(t, env.Success)
	return env.Data
}

func decodeJob(
	t *testing.T,
	body []byte,
) apidto.DiscoveryJobDTO {
	t.Helper()

	var env struct {
		Success bool                   `json:"success"`
		Data    apidto.DiscoveryJobDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.True(t, env.Success)
	return env.Data
}

func TestDiscoverHandler_BlankQueryReturns400(t *testing.T) {
	disc := &mocks.DiscoveryService{}

	w := post(disc, `{"q":"   "}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, disc.StartCalls())
}

func TestDiscoverHandler_MissingBodyReturns400(t *testing.T) {
	disc := &mocks.DiscoveryService{}

	w := post(disc, "")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, disc.StartCalls())
}

func TestDiscoverHandler_MalformedBodyReturns400(t *testing.T) {
	disc := &mocks.DiscoveryService{}

	w := post(disc, `{"q":`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, disc.StartCalls())
}

func TestDiscoverHandler_ValidReturns202WithJobID(t *testing.T) {
	expires := time.Date(2026, 7, 26, 12, 0, 30, 0, time.UTC)
	disc := &mocks.DiscoveryService{
		StartResult: usecases.Job{
			ID:        "job-1",
			Query:     "chrom",
			Status:    usecases.JobRunning,
			ExpiresAt: expires,
		},
	}

	w := post(disc, `{"q":"  chrom  "}`)

	require.Equal(t, http.StatusAccepted, w.Code)
	got := decodeStarted(t, w.Body.Bytes())
	assert.Equal(t, "job-1", got.JobID)
	assert.Equal(t, "chrom", got.Query)
	assert.True(t, expires.Equal(got.ExpiresAt))
	assert.Equal(t, "chrom", disc.StartQuery(), "the handler trims before starting")
}

func TestDiscoverHandler_StartFailureIsReported(t *testing.T) {
	disc := &mocks.DiscoveryService{StartErr: fmt.Errorf("providers unavailable")}

	w := post(disc, `{"q":"chrom"}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDiscoverHandler_WithoutDiscoveryReturns503(t *testing.T) {
	w := post(nil, `{"q":"chrom"}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "discovery is not available")
}

func TestJobHandler_RunningReturnsStatus(t *testing.T) {
	disc := &mocks.DiscoveryService{
		GetResult: &usecases.Job{ID: "job-1", Query: "chrom", Status: usecases.JobRunning},
	}

	w := getJob(disc, "job-1")

	require.Equal(t, http.StatusOK, w.Code)
	got := decodeJob(t, w.Body.Bytes())
	assert.Equal(t, "job-1", got.JobID)
	assert.Equal(t, "running", got.Status)
	assert.Equal(t, "chrom", got.Query)
	assert.Empty(t, got.Providers)
	assert.NotNil(t, got.Providers, "an empty provider set renders as [], never null")
	assert.Equal(t, "job-1", disc.GetID())
}

func TestJobHandler_CompletedReturnsCounts(t *testing.T) {
	disc := &mocks.DiscoveryService{
		GetResult: &usecases.Job{
			ID:     "job-1",
			Query:  "chrom",
			Status: usecases.JobCompleted,
			Outcome: discovery.Outcome{
				Found:    25,
				Verified: 19,
				Skipped:  6,
				Providers: []discovery.ProviderOutcome{
					{Host: "github.com", OK: true, Returned: 25},
				},
			},
		},
	}

	w := getJob(disc, "job-1")

	require.Equal(t, http.StatusOK, w.Code)
	got := decodeJob(t, w.Body.Bytes())
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 25, got.Found)
	assert.Equal(t, 19, got.Verified)
	assert.Equal(t, 6, got.Skipped)
	require.Len(t, got.Providers, 1)
	assert.Equal(t, "github.com", got.Providers[0].Host)
	assert.True(t, got.Providers[0].OK)
	assert.Equal(t, 25, got.Providers[0].Returned)
}

// TestJobHandler_RateLimitedProviderIsReported is the one that matters most to a
// user: "GitHub rate-limited you, try in 40s" must never render as "no results
// found".
func TestJobHandler_RateLimitedProviderIsReported(t *testing.T) {
	disc := &mocks.DiscoveryService{
		GetResult: &usecases.Job{
			ID:     "job-1",
			Query:  "chrom",
			Status: usecases.JobCompleted,
			Outcome: discovery.Outcome{
				Providers: []discovery.ProviderOutcome{
					{
						Host:       "gitlab.com",
						OK:         false,
						Reason:     discovery.ReasonRateLimited,
						RetryAfter: 40 * time.Second,
					},
				},
			},
		},
	}

	w := getJob(disc, "job-1")

	require.Equal(t, http.StatusOK, w.Code)
	got := decodeJob(t, w.Body.Bytes())
	assert.Zero(t, got.Found, "a rate limit is not a result count")
	require.Len(t, got.Providers, 1)
	assert.Equal(t, "gitlab.com", got.Providers[0].Host)
	assert.False(t, got.Providers[0].OK)
	assert.Equal(t, "rate_limited", got.Providers[0].Reason)
	assert.Equal(t, 40, got.Providers[0].RetryAfter)

	// The wire form is what a client actually reads: a failing host has to be
	// distinguishable from an empty result set in the JSON itself.
	assert.Contains(t, w.Body.String(), `"reason":"rate_limited"`)
	assert.Contains(t, w.Body.String(), `"retry_after":40`)
}

func TestJobHandler_UnknownIDReturns404(t *testing.T) {
	disc := &mocks.DiscoveryService{GetErr: apperrors.ErrNotFound}

	w := getJob(disc, "missing")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestJobHandler_WithoutDiscoveryReturns503(t *testing.T) {
	w := getJob(nil, "job-1")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
