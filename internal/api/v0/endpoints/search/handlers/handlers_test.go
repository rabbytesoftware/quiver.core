package search_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	search "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/search/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func setup(svc *mocks.SearchService) *gin.Engine {
	h := search.New(svc, nil)
	r := gin.New()
	r.GET("/v0/search", h.Search)
	return r
}

func do(
	t *testing.T,
	svc *mocks.SearchService,
	target string,
) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	setup(svc).ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func decodeResults(
	t *testing.T,
	body []byte,
) []apidto.SearchResultDTO {
	t.Helper()
	var env struct {
		Success bool                     `json:"success"`
		Data    []apidto.SearchResultDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.True(t, env.Success)
	return env.Data
}

func TestSearchHandler_MissingQueryReturns400(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, svc.SearchCalls)
}

func TestSearchHandler_BlankQueryReturns400(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search?q=%20%20%20")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, svc.SearchCalls)
}

func TestSearchHandler_ReturnsResults(t *testing.T) {
	svc := &mocks.SearchService{SearchResult: []models.SearchResult{{
		Namespace:    domain.Namespace("github.com/user/repo"),
		Name:         "repo",
		Description:  "a repo",
		Tags:         []string{"db"},
		Media:        domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
		Versions:     []string{"v1.0.0"},
		CompatibleOS: []domain.OS{domain.OSLinuxAMD64},
		Provenance:   models.ProvenanceInstalled,
		Installed:    true,
		Stars:        42,
		Source:       "github",
	}}}

	w := do(t, svc, "/v0/search?q=repo")

	require.Equal(t, http.StatusOK, w.Code)
	results := decodeResults(t, w.Body.Bytes())
	require.Len(t, results, 1)
	assert.Equal(t, "github.com/user/repo", results[0].Namespace)
	assert.Equal(t, "repo", results[0].Name)
	assert.Equal(t, []string{"db"}, results[0].Tags)
	assert.Equal(t, "icon.png", results[0].Media.Icon)
	assert.Equal(t, []string{"v1.0.0"}, results[0].Versions)
	assert.Equal(t, []string{"linux/amd64"}, results[0].CompatibleOS)
	assert.Equal(t, "installed", results[0].Provenance)
	assert.True(t, results[0].Installed)
	assert.Equal(t, 42, results[0].Stars)
	assert.Equal(t, "repo", svc.SearchQuery.Text)
}

func TestSearchHandler_LimitIsCapped(t *testing.T) {
	testCases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "absent falls back to default", query: "/v0/search?q=repo", want: 25},
		{name: "unparseable falls back to default", query: "/v0/search?q=repo&limit=many", want: 25},
		{name: "zero falls back to default", query: "/v0/search?q=repo&limit=0", want: 25},
		{name: "negative falls back to default", query: "/v0/search?q=repo&limit=-3", want: 25},
		{name: "in range is honoured", query: "/v0/search?q=repo&limit=7", want: 7},
		{name: "at the cap is honoured", query: "/v0/search?q=repo&limit=100", want: 100},
		{name: "above the cap is clamped", query: "/v0/search?q=repo&limit=5000", want: 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.SearchService{}
			w := do(t, svc, tc.query)

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.want, svc.SearchQuery.Limit)
		})
	}
}

func TestSearchHandler_InvalidOSReturns400(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search?q=repo&os=plan9%2F386")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, svc.SearchCalls)
}

func TestSearchHandler_ValidOSIsForwarded(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search?q=repo&os=darwin%2Farm64")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, domain.OSDarwinARM64, svc.SearchQuery.OS)
}

func TestSearchHandler_AbsentOSMeansNoFilter(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search?q=repo")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, domain.OS(""), svc.SearchQuery.OS)
}

func TestSearchHandler_UsecaseErrorMapsToStatus(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want int
	}{
		{name: "fetch failed", err: apperrors.ErrFetchFailed, want: http.StatusBadGateway},
		{name: "unmapped", err: errors.New("boom"), want: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.SearchService{SearchErr: tc.err}
			w := do(t, svc, "/v0/search?q=repo")

			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestSearchHandler_NoResultsReturns200WithEmptyList(t *testing.T) {
	svc := &mocks.SearchService{}
	w := do(t, svc, "/v0/search?q=nothing")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
	assert.Empty(t, decodeResults(t, w.Body.Bytes()))
}
