package v0_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
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

// TestNew_WithDiscovery_WiresTheResultStream: results reach clients straight
// from the usecase, because a discovery result is not a domain aggregate and
// has no projection behind it to broadcast from.
func TestNew_WithDiscovery_WiresTheResultStream(t *testing.T) {
	disc := &mocks.DiscoveryService{}

	_, err := apiv0.New(&app.Container{Discovery: disc})
	require.NoError(t, err)

	assert.Equal(t, 1, disc.Listeners())
	assert.NotPanics(t, func() { disc.Emit(usecases.StreamItem{JobID: "job-1"}) })
}

func TestNew_WithoutDiscovery_RegistersNoListener(t *testing.T) {
	c, err := apiv0.New(&app.Container{})
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/search/discover", strings.NewReader(`{"q":"chrom"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
