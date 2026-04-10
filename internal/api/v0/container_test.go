package v0_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/app"
	v0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestInit_NilAppContainer_ReturnsError(t *testing.T) {
	_, err := v0.Init(nil, nil)
	require.Error(t, err)
}

func TestInit_WithNilWSHandler_CreatesHandler(t *testing.T) {
	c, err := v0.Init(&app.Container{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, c.WSHandler())
}

func TestInit_WithWSHandler_UsesProvided(t *testing.T) {
	h := v0.NewWSHandler()
	c, err := v0.Init(&app.Container{}, h)
	require.NoError(t, err)
	assert.Equal(t, h, c.WSHandler())
}

func TestNewWSHandler_ReturnsNonNil(t *testing.T) {
	h := v0.NewWSHandler()
	assert.NotNil(t, h)
}
