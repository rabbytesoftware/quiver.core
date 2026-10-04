package versions_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/endpoints/versions"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestGet(t *testing.T) {
	h := versions.New(versions.Build{
		Version: "1.2.3", BuildID: "42", Commit: "abc", BuiltAt: "2026-10-04T13:47:00Z", Channel: "nightly-latest",
		Features: []string{"console.v1"},
	}, []string{"v0"}, "v0")

	r := gin.New()
	r.GET("/versions", h.Get)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/versions", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Version  string   `json:"version"`
			BuildID  string   `json:"build_id"`
			Commit   string   `json:"commit"`
			BuiltAt  string   `json:"built_at"`
			Channel  string   `json:"channel"`
			Features []string `json:"features"`
			API      struct {
				Supported []string `json:"supported"`
				Latest    string   `json:"latest"`
			} `json:"api"`
		} `json:"data"`
	}

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "1.2.3", resp.Data.Version)
	assert.Equal(t, "42", resp.Data.BuildID)
	assert.Equal(t, "abc", resp.Data.Commit)
	assert.Equal(t, "2026-10-04T13:47:00Z", resp.Data.BuiltAt)
	assert.Equal(t, "nightly-latest", resp.Data.Channel)
	assert.Equal(t, []string{"console.v1"}, resp.Data.Features)
	assert.Equal(t, []string{"v0"}, resp.Data.API.Supported)
	assert.Equal(t, "v0", resp.Data.API.Latest)
}
