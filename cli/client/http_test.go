package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"quiver-cli/client"
)

// apiOK writes a standard success envelope around data.
func apiOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

// apiErr writes a standard error envelope.
func apiErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
}

// httpClient creates a test HTTP server with the given handler and returns an HTTPClient pointing at it.
func httpClient(t *testing.T, handler http.HandlerFunc) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.NewHTTPClient(srv.URL)
}

// --- ArrowList ---

func TestHTTPClient_ArrowList_ReturnsItems(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v0/arrow", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("user_installed"))
		apiOK(w, []client.ArrowListItem{
			{Namespace: "github.com/foo/bar", Name: "bar"},
		})
	})

	items, err := c.ArrowList(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "github.com/foo/bar", items[0].Namespace)
}

func TestHTTPClient_ArrowList_ServerError_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusInternalServerError, "internal error")
	})
	_, err := c.ArrowList(context.Background(), false)
	assert.Error(t, err)
}

// --- ArrowGet ---

func TestHTTPClient_ArrowGet_ReturnsDetail(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.RawPath)
		apiOK(w, client.ArrowDetail{Namespace: "github.com/foo/bar", State: "ready"})
	})

	detail, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "ready", detail.State)
}

func TestHTTPClient_ArrowGet_NotFound_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusNotFound, "not found")
	})
	_, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	assert.Error(t, err)
}

// --- ArrowGetManifest ---

func TestHTTPClient_ArrowGetManifest_ReturnsDataBytes(t *testing.T) {
	manifest := map[string]any{"name": "bar", "version": "1.0.0"}
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.RawPath)
		apiOK(w, manifest)
	})

	data, err := c.ArrowGetManifest(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "bar", decoded["name"])
}

// --- ArrowAdd ---

func TestHTTPClient_ArrowAdd_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.RawPath)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true, "namespace": "github.com/foo/bar"})
	})
	assert.NoError(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

func TestHTTPClient_ArrowAdd_Conflict_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusConflict, "already registered")
	})
	assert.Error(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

// --- ArrowUpdate ---

func TestHTTPClient_ArrowUpdate_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowUpdate(context.Background(), "github.com/foo/bar"))
}

// --- ArrowRemove ---

func TestHTTPClient_ArrowRemove_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar%40v1.0.0", r.URL.RawPath)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowRemove(context.Background(), "github.com/foo/bar@v1.0.0"))
}

// --- ArrowSeed ---

func TestHTTPClient_ArrowSeed_SendsYAMLBody(t *testing.T) {
	manifest := []byte("name: bar\nversion: 1.0.0")
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.RawPath)
		assert.Equal(t, "application/x-yaml", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowSeed(context.Background(), "github.com/foo/bar", manifest))
}

// --- ArrowValidate ---

func TestHTTPClient_ArrowValidate_Valid(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest/validate", r.URL.RawPath)
		apiOK(w, client.ValidationResult{Valid: true, SupportedPlatforms: []string{"linux"}})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("name: bar"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
}

func TestHTTPClient_ArrowValidate_Invalid_ReturnsResult(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Server returns 422 for invalid but same body shape.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data": client.ValidationResult{
				Valid:  false,
				Errors: []client.ValidationError{{Field: "name", Rule: "required", Message: "name is required"}},
			},
		})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("version: 1.0.0"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "name", result.Errors[0].Field)
}
