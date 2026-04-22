package v0_test

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestNew_NilAppContainer_ReturnsError(t *testing.T) {
	_, err := apiv0.New(nil)
	require.Error(t, err)
}

func TestNew_ValidAppContainer_ReturnsContainer(t *testing.T) {
	c, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNew_WSHandler_NotNil(t *testing.T) {
	c, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	assert.NotNil(t, c.WSHandler())
}

func TestNew_Prefix_IsV0(t *testing.T) {
	c, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	assert.Equal(t, "/v0", c.Prefix())
}
