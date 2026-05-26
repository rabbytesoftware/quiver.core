package libs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type envelope struct {
	Success   bool    `json:"success"`
	Error     *string `json:"error"`
	Namespace string  `json:"namespace"`
	Data      any     `json:"data"`
}

func newContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, w
}

func TestWriteMutationOK(t *testing.T) {
	c, w := newContext(http.MethodPost, "/")
	libs.WriteMutationOK(c, http.StatusCreated, "github.com/user/repo")

	assert.Equal(t, http.StatusCreated, w.Code)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env.Success)
	assert.Nil(t, env.Error)
	assert.Equal(t, "github.com/user/repo", env.Namespace)
}

func TestWriteQueryOK(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteQueryOK(c, map[string]string{"key": "val"})

	assert.Equal(t, http.StatusOK, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env["success"].(bool))
	_, hasErr := env["error"]
	assert.True(t, hasErr, "error key must always be present in the envelope")
	assert.Nil(t, env["error"])
	assert.NotNil(t, env["data"])
}

func TestWriteQueryWithStatus_Success(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteQueryWithStatus(c, http.StatusOK, map[string]string{"key": "val"})

	assert.Equal(t, http.StatusOK, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env["success"].(bool))
	assert.NotNil(t, env["data"])
}

func TestWriteQueryWithStatus_Failure(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteQueryWithStatus(c, http.StatusUnprocessableEntity, map[string]string{"valid": "false"})

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.False(t, env["success"].(bool))
	assert.NotNil(t, env["data"])
}

func TestWriteErr(t *testing.T) {
	c, w := newContext(http.MethodPost, "/")
	libs.WriteErr(c, http.StatusNotFound, "not found", "github.com/user/repo")

	assert.Equal(t, http.StatusNotFound, w.Code)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.False(t, env.Success)
	require.NotNil(t, env.Error)
	assert.Equal(t, "not found", *env.Error)
	assert.Equal(t, "github.com/user/repo", env.Namespace)
}

func TestWriteErr_NoNamespace(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteErr(c, http.StatusInternalServerError, "internal error", "")

	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	_, hasNs := env["namespace"]
	assert.False(t, hasNs, "namespace should be omitted when empty")
}
