package ping_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/endpoints/ping"
	"github.com/rabbytesoftware/quiver.core/internal/core/build"
)

func TestHandler_Get_ReturnsBuildAndAttemptIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := ping.New(build.Info{
		Version:      "25.9.1",
		BuildID:      "123",
		Digest:       "abc123",
		AttemptToken: "attempt-token",
	})
	router := gin.New()
	router.GET("/ping", h.Get)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Status       string `json:"status"`
			Version      string `json:"version"`
			BuildID      string `json:"build_id"`
			Digest       string `json:"digest"`
			AttemptToken string `json:"attempt_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "ready", body.Data.Status)
	assert.Equal(t, "25.9.1", body.Data.Version)
	assert.Equal(t, "123", body.Data.BuildID)
	assert.Equal(t, "abc123", body.Data.Digest)
	assert.Equal(t, "attempt-token", body.Data.AttemptToken)
}

func TestHandler_Get_WithoutAttempt_PreservesStableShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := ping.New(build.Info{Version: "25.9.1", BuildID: "123", Digest: "abc123"})
	router := gin.New()
	router.GET("/ping", h.Get)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	value, exists := data["attempt_token"]
	assert.True(t, exists)
	assert.Equal(t, "", value)
}
